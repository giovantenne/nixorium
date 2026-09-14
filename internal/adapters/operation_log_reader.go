package adapters

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/giovantenne/nixorium/internal/domain"
)

const (
	operationLogTimestampLayout = "20060102T150405.000000000Z"
	operationLogSummaryBytes    = int64(4096)
	maximumOperationLogScan     = 10000
)

var deploymentLogName = regexp.MustCompile(`^deploy-(\d{8}T\d{6}\.\d{9}Z)-(\d+)\.log$`)

func (Local) OperationLogs(limit int) ([]domain.OperationLogEntry, error) {
	if limit < 1 || limit > 1000 {
		return nil, errors.New("operation log limit must be between 1 and 1000")
	}
	stateRoot, err := userStateRoot()
	if err != nil {
		return nil, err
	}
	directory, err := openOperationLogDirectory(stateRoot)
	if os.IsNotExist(err) {
		return []domain.OperationLogEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer directory.Close()

	candidates := make([]domain.OperationLogEntry, 0, limit)
	matched := 0
	scanned := 0
	for {
		entries, readErr := directory.ReadDir(256)
		scanned += len(entries)
		if scanned > maximumOperationLogScan {
			return nil, fmt.Errorf("operation log directory contains more than %d entries", maximumOperationLogScan)
		}
		for _, entry := range entries {
			parsed, ok := parseOperationLogName(entry.Name())
			if !ok {
				continue
			}
			matched++
			if matched > maximumOperationLogScan {
				return nil, fmt.Errorf("operation log directory contains more than %d recognized logs", maximumOperationLogScan)
			}
			candidates = append(candidates, parsed)
		}
		sortOperationLogs(candidates)
		if len(candidates) > limit {
			candidates = candidates[:limit]
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("read operation log directory: %w", readErr)
		}
	}

	for index := range candidates {
		content, size, _, readErr := readOperationLogAt(directory, candidates[index].ID, operationLogSummaryBytes)
		candidates[index].SizeBytes = size
		if readErr != nil {
			candidates[index].State = "unavailable"
			candidates[index].Detail = readErr.Error()
			continue
		}
		candidates[index].Available = true
		candidates[index].State = operationResultState(content)
	}
	return candidates, nil
}

func (Local) OperationLog(id string, maximumBytes int64) (domain.OperationLogReport, error) {
	if maximumBytes < 1 || maximumBytes > 1024*1024 {
		return domain.OperationLogReport{}, errors.New("operation log read limit must be between 1 byte and 1 MiB")
	}
	entry, ok := parseOperationLogName(id)
	if !ok {
		return domain.OperationLogReport{}, errors.New("operation log ID is invalid")
	}
	stateRoot, err := userStateRoot()
	if err != nil {
		return domain.OperationLogReport{}, err
	}
	directory, err := openOperationLogDirectory(stateRoot)
	if err != nil {
		return domain.OperationLogReport{}, err
	}
	defer directory.Close()
	content, size, truncated, err := readOperationLogAt(directory, id, maximumBytes)
	if err != nil {
		return domain.OperationLogReport{}, err
	}
	entry.SizeBytes = size
	entry.State = operationResultState(content)
	entry.Available = true
	return domain.OperationLogReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "logs-show",
		State:         "available",
		Log:           &entry,
		Content:       sanitizeOperationLog(content),
		Truncated:     truncated,
		Issues:        []domain.ValidationIssue{},
	}, nil
}

func parseOperationLogName(name string) (domain.OperationLogEntry, bool) {
	match := deploymentLogName.FindStringSubmatch(name)
	if match == nil {
		return domain.OperationLogEntry{}, false
	}
	startedAt, err := time.Parse(operationLogTimestampLayout, match[1])
	if err != nil {
		return domain.OperationLogEntry{}, false
	}
	return domain.OperationLogEntry{ID: name, Kind: "deployment", StartedAt: startedAt.UTC(), State: "unknown"}, true
}

func sortOperationLogs(logs []domain.OperationLogEntry) {
	sort.Slice(logs, func(left, right int) bool {
		if logs[left].StartedAt.Equal(logs[right].StartedAt) {
			return logs[left].ID > logs[right].ID
		}
		return logs[left].StartedAt.After(logs[right].StartedAt)
	})
}

func openOperationLogDirectory(stateRoot string) (*os.File, error) {
	productDirectory := stateRoot + string(os.PathSeparator) + "nixorium"
	if err := validatePrivateOwnedDirectory(productDirectory); err != nil {
		return nil, fmt.Errorf("inspect Nixorium state directory: %w", err)
	}
	path := productDirectory + string(os.PathSeparator) + "operations"
	descriptor, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open operation log directory: %w", err)
	}
	directory := os.NewFile(uintptr(descriptor), path)
	if err := validatePrivateOwnedFile(directory, syscall.S_IFDIR, 0700); err != nil {
		directory.Close()
		return nil, fmt.Errorf("inspect operation log directory: %w", err)
	}
	return directory, nil
}

func validatePrivateOwnedDirectory(path string) error {
	descriptor, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(descriptor), path)
	defer file.Close()
	return validatePrivateOwnedFile(file, syscall.S_IFDIR, 0700)
}

func validatePrivateOwnedFile(file *os.File, kind uint32, mode uint32) error {
	var stat syscall.Stat_t
	if err := syscall.Fstat(int(file.Fd()), &stat); err != nil {
		return err
	}
	if stat.Mode&syscall.S_IFMT != kind {
		return errors.New("must have the expected regular file or directory type")
	}
	if stat.Mode&0777 != mode {
		return fmt.Errorf("mode is %04o, want %04o", stat.Mode&0777, mode)
	}
	if stat.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("owner UID is %d, want %d", stat.Uid, os.Geteuid())
	}
	return nil
}

func readOperationLogAt(directory *os.File, id string, maximumBytes int64) ([]byte, int64, bool, error) {
	descriptor, err := syscall.Openat(int(directory.Fd()), id, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, 0, false, fmt.Errorf("open operation log: %w", err)
	}
	file := os.NewFile(uintptr(descriptor), id)
	defer file.Close()
	if err := validatePrivateOwnedFile(file, syscall.S_IFREG, 0600); err != nil {
		return nil, 0, false, fmt.Errorf("inspect operation log: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		return nil, 0, false, fmt.Errorf("inspect operation log size: %w", err)
	}
	size := info.Size()
	truncated := size > maximumBytes
	if truncated {
		if _, err := file.Seek(size-maximumBytes, io.SeekStart); err != nil {
			return nil, size, false, fmt.Errorf("seek operation log tail: %w", err)
		}
	}
	content, err := io.ReadAll(io.LimitReader(file, maximumBytes))
	if err != nil {
		return nil, size, truncated, fmt.Errorf("read operation log: %w", err)
	}
	return content, size, truncated, nil
}

func operationResultState(content []byte) string {
	state := "incomplete"
	for _, line := range strings.Split(string(content), "\n") {
		if !strings.HasPrefix(line, "Result: ") {
			continue
		}
		candidate := strings.TrimSpace(strings.TrimPrefix(line, "Result: "))
		switch candidate {
		case "completed", "failed", "partial", "blocked":
			state = candidate
		}
	}
	return state
}

func sanitizeOperationLog(content []byte) string {
	text := strings.ToValidUTF8(string(content), "�")
	return strings.Map(func(character rune) rune {
		if character == '\n' || character == '\t' {
			return character
		}
		if unicode.IsControl(character) || unicode.In(character, unicode.Cf) {
			return '�'
		}
		return character
	}, text)
}
