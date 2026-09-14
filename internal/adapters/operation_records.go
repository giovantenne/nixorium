package adapters

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/giovantenne/nixorium/internal/domain"
)

const (
	maximumOperationRecords     = 1000
	maximumOperationRecordsFile = int64(1024 * 1024)
	maximumOperationFieldBytes  = 256
)

var (
	operationRecordToken = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	operationRecordID    = regexp.MustCompile(`^record-(\d{8}T\d{6}\.\d{9}Z)-\d+(?:-\d+)?$`)
)

type operationRecordFile struct {
	SchemaVersion int                      `json:"schemaVersion"`
	Records       []domain.OperationRecord `json:"records"`
}

func (Local) OperationRecords(limit int) ([]domain.OperationRecord, error) {
	if limit < 1 || limit > maximumOperationRecords {
		return nil, fmt.Errorf("operation record limit must be between 1 and %d", maximumOperationRecords)
	}
	stateRoot, err := userStateRoot()
	if err != nil {
		return nil, err
	}
	directory, err := openOperationLogDirectory(stateRoot)
	if errors.Is(err, syscall.ENOENT) {
		return []domain.OperationRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	store, err := readOperationRecordFile(directory)
	if errors.Is(err, syscall.ENOENT) {
		return []domain.OperationRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	start := len(store.Records) - limit
	if start < 0 {
		start = 0
	}
	result := make([]domain.OperationRecord, 0, len(store.Records)-start)
	for index := len(store.Records) - 1; index >= start; index-- {
		result = append(result, store.Records[index])
	}
	return result, nil
}

func (Local) RecordOperation(record domain.OperationRecord) error {
	stateRoot, err := userStateRoot()
	if err != nil {
		return err
	}
	return recordOperation(stateRoot, time.Now().UTC(), os.Getpid(), record)
}

func recordOperation(stateRoot string, now time.Time, pid int, record domain.OperationRecord) error {
	if now.IsZero() || pid < 1 {
		return errors.New("operation record timestamp and process ID must be valid")
	}
	if !operationRecordToken.MatchString(record.Operation) || !operationRecordToken.MatchString(record.State) {
		return errors.New("operation record has an invalid operation or state")
	}
	if !safeOperationRecordText(record.Subject, true) || !safeOperationRecordText(record.Summary, false) {
		return errors.New("operation record contains unsafe or oversized text")
	}
	directory, err := ensureOperationRecordDirectory(stateRoot)
	if err != nil {
		return err
	}
	defer directory.Close()

	lockDescriptor, err := syscall.Openat(int(directory.Fd()), "records.lock", syscall.O_RDWR|syscall.O_CREAT|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return fmt.Errorf("open operation record lock: %w", err)
	}
	lock := os.NewFile(uintptr(lockDescriptor), "records.lock")
	defer lock.Close()
	if err := validatePrivateOwnedFile(lock, syscall.S_IFREG, 0600); err != nil {
		return fmt.Errorf("inspect operation record lock: %w", err)
	}
	if err := syscall.Flock(lockDescriptor, syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock operation records: %w", err)
	}
	defer syscall.Flock(lockDescriptor, syscall.LOCK_UN)

	store, err := readOperationRecordFile(directory)
	if errors.Is(err, syscall.ENOENT) {
		store = operationRecordFile{SchemaVersion: domain.SchemaVersion, Records: []domain.OperationRecord{}}
	} else if err != nil {
		return err
	}
	record.RecordedAt = now.UTC()
	record.ID = nextOperationRecordID(store.Records, record.RecordedAt, pid)
	store.Records = append(store.Records, record)
	sort.SliceStable(store.Records, func(left, right int) bool {
		if store.Records[left].RecordedAt.Equal(store.Records[right].RecordedAt) {
			return store.Records[left].ID < store.Records[right].ID
		}
		return store.Records[left].RecordedAt.Before(store.Records[right].RecordedAt)
	})
	store.Records = retainNewestOperationRecords(store.Records, maximumOperationRecords)
	return writeOperationRecordFile(directory, store)
}

func ensureOperationRecordDirectory(stateRoot string) (*os.File, error) {
	if !filepath.IsAbs(stateRoot) {
		return nil, errors.New("operation state directory must be absolute")
	}
	productDirectory := filepath.Join(stateRoot, "nixorium")
	if err := os.MkdirAll(productDirectory, 0700); err != nil {
		return nil, fmt.Errorf("create operation state directory: %w", err)
	}
	if err := requirePrivateDirectory(productDirectory); err != nil {
		return nil, err
	}
	directory := filepath.Join(productDirectory, "operations")
	if err := os.Mkdir(directory, 0700); err != nil && !os.IsExist(err) {
		return nil, fmt.Errorf("create operation record directory: %w", err)
	}
	if err := requirePrivateDirectory(directory); err != nil {
		return nil, err
	}
	return openOperationLogDirectory(stateRoot)
}

func readOperationRecordFile(directory *os.File) (operationRecordFile, error) {
	descriptor, err := syscall.Openat(int(directory.Fd()), "records.json", syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return operationRecordFile{}, fmt.Errorf("open operation records: %w", err)
	}
	file := os.NewFile(uintptr(descriptor), "records.json")
	defer file.Close()
	if err := validatePrivateOwnedFile(file, syscall.S_IFREG, 0600); err != nil {
		return operationRecordFile{}, fmt.Errorf("inspect operation records: %w", err)
	}
	content, err := io.ReadAll(io.LimitReader(file, maximumOperationRecordsFile+1))
	if err != nil {
		return operationRecordFile{}, fmt.Errorf("read operation records: %w", err)
	}
	if int64(len(content)) > maximumOperationRecordsFile {
		return operationRecordFile{}, errors.New("operation record file is unexpectedly large")
	}
	store := operationRecordFile{}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&store); err != nil {
		return operationRecordFile{}, fmt.Errorf("decode operation records: %w", err)
	}
	if err := rejectTrailingJSON(decoder); err != nil {
		return operationRecordFile{}, fmt.Errorf("decode operation records: %w", err)
	}
	if store.SchemaVersion != domain.SchemaVersion || store.Records == nil || len(store.Records) > maximumOperationRecords {
		return operationRecordFile{}, errors.New("operation record file has an invalid schema or record count")
	}
	seen := map[string]bool{}
	for _, record := range store.Records {
		if !validStoredOperationRecord(record) || seen[record.ID] {
			return operationRecordFile{}, errors.New("operation record file contains an invalid or duplicate record")
		}
		seen[record.ID] = true
	}
	sort.SliceStable(store.Records, func(left, right int) bool {
		if store.Records[left].RecordedAt.Equal(store.Records[right].RecordedAt) {
			return store.Records[left].ID < store.Records[right].ID
		}
		return store.Records[left].RecordedAt.Before(store.Records[right].RecordedAt)
	})
	return store, nil
}

func writeOperationRecordFile(directory *os.File, store operationRecordFile) error {
	content, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("encode operation records: %w", err)
	}
	content = append(content, '\n')
	if int64(len(content)) > maximumOperationRecordsFile {
		return errors.New("operation record file would exceed its 1 MiB bound")
	}
	temporary, err := os.CreateTemp(directory.Name(), ".operation-records.*")
	if err != nil {
		return fmt.Errorf("create operation record draft: %w", err)
	}
	temporaryPath := temporary.Name()
	keep := true
	defer func() {
		if keep {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure operation record draft: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return fmt.Errorf("write operation record draft: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync operation record draft: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close operation record draft: %w", err)
	}
	target := filepath.Join(directory.Name(), "records.json")
	if err := os.Rename(temporaryPath, target); err != nil {
		return fmt.Errorf("replace operation records: %w", err)
	}
	keep = false
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync operation record directory: %w", err)
	}
	return nil
}

func safeOperationRecordText(value string, optional bool) bool {
	if optional && value == "" {
		return true
	}
	if value == "" || value != strings.TrimSpace(value) || len(value) > maximumOperationFieldBytes {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.In(character, unicode.Cf) {
			return false
		}
	}
	return true
}

func validStoredOperationRecord(record domain.OperationRecord) bool {
	match := operationRecordID.FindStringSubmatch(record.ID)
	if match == nil || record.RecordedAt.IsZero() {
		return false
	}
	idTime, err := time.Parse(operationLogTimestampLayout, match[1])
	return err == nil && idTime.Equal(record.RecordedAt) && operationRecordToken.MatchString(record.Operation) &&
		operationRecordToken.MatchString(record.State) && safeOperationRecordText(record.Subject, true) &&
		safeOperationRecordText(record.Summary, false)
}

func nextOperationRecordID(records []domain.OperationRecord, now time.Time, pid int) string {
	base := fmt.Sprintf("record-%s-%d", now.UTC().Format(operationLogTimestampLayout), pid)
	used := map[string]bool{}
	for _, record := range records {
		used[record.ID] = true
	}
	if !used[base] {
		return base
	}
	for suffix := 1; ; suffix++ {
		candidate := fmt.Sprintf("%s-%d", base, suffix)
		if !used[candidate] {
			return candidate
		}
	}
}

func retainNewestOperationRecords(records []domain.OperationRecord, limit int) []domain.OperationRecord {
	if len(records) <= limit {
		return records
	}
	return append([]domain.OperationRecord{}, records[len(records)-limit:]...)
}
