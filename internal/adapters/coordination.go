package adapters

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/giovantenne/nixorium/internal/domain"
)

const (
	managedCoordinationDirectory = "/var/lib/nixorium/coordination"
	legacyDeploymentLockPath     = "/home/admin/.local/state/nixorium/operations/deploy.lock"
	coordinationLockName         = "operation.lock"
	remoteReservationName        = "usb-reservation.json"
)

type operationGate struct {
	file   *os.File
	legacy *os.File
}

func (gate *operationGate) Close() error {
	if gate == nil || gate.file == nil {
		return nil
	}
	if gate.legacy != nil {
		_ = syscall.Flock(int(gate.legacy.Fd()), syscall.LOCK_UN)
		_ = gate.legacy.Close()
		gate.legacy = nil
	}
	_ = syscall.Flock(int(gate.file.Fd()), syscall.LOCK_UN)
	err := gate.file.Close()
	gate.file = nil
	return err
}

func acquireManagedOperationGate() (*operationGate, error) {
	gate, err := acquireOperationGateAt(managedCoordinationDirectory, true)
	if err != nil {
		return nil, err
	}
	legacy, err := acquireLegacyDeploymentLock()
	if err != nil {
		gate.Close()
		return nil, err
	}
	gate.legacy = legacy
	return gate, nil
}

func acquireLegacyDeploymentLock() (*os.File, error) {
	descriptor, err := syscall.Open(legacyDeploymentLockPath, syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, syscall.ENOENT) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect legacy deployment lock: %w", err)
	}
	file := os.NewFile(uintptr(descriptor), legacyDeploymentLockPath)
	if err := validatePrivateOwnedFile(file, syscall.S_IFREG, 0600); err != nil {
		file.Close()
		return nil, fmt.Errorf("inspect legacy deployment lock: %w", err)
	}
	administrator, err := user.Lookup("admin")
	if err != nil {
		file.Close()
		return nil, errors.New("admin account is unavailable for legacy lock migration")
	}
	uid, err := strconv.ParseUint(administrator.Uid, 10, 32)
	var stat syscall.Stat_t
	if err != nil || syscall.Fstat(descriptor, &stat) != nil || stat.Uid != uint32(uid) {
		file.Close()
		return nil, errors.New("legacy deployment lock has the wrong owner")
	}
	if err := syscall.Flock(descriptor, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, errors.New("a legacy Nixorium deployment is still running; close it before using the managed operation gate")
	}
	return file, nil
}

func managedOperationActive() (bool, error) {
	active, err := operationActiveAt(managedCoordinationDirectory, true)
	if err != nil || active {
		return active, err
	}
	legacy, err := acquireLegacyDeploymentLock()
	if err != nil {
		return true, err
	}
	if legacy != nil {
		_ = syscall.Flock(int(legacy.Fd()), syscall.LOCK_UN)
		_ = legacy.Close()
	}
	return false, nil
}

func acquireOperationGateAt(directoryPath string, managed bool) (*operationGate, error) {
	directory, err := openCoordinationDirectory(directoryPath, managed)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	if present, markerErr := inspectReservationAt(directory); present {
		if markerErr != nil {
			return nil, fmt.Errorf("USB installation reservation blocks new operations: %w", markerErr)
		}
		return nil, errors.New("a USB installation remains reserved; reconcile or close that operation first")
	}
	descriptor, err := syscall.Openat(int(directory.Fd()), coordinationLockName, syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open managed operation lock: %w; activate the Nixorium controller configuration", err)
	}
	lock := os.NewFile(uintptr(descriptor), filepath.Join(directoryPath, coordinationLockName))
	if err := validateCoordinationLock(lock, managed); err != nil {
		lock.Close()
		return nil, err
	}
	if err := syscall.Flock(descriptor, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("another Nixorium controller or client operation is already running")
	}
	if present, markerErr := inspectReservationAt(directory); present {
		_ = syscall.Flock(descriptor, syscall.LOCK_UN)
		lock.Close()
		if markerErr != nil {
			return nil, fmt.Errorf("USB installation reservation blocks new operations: %w", markerErr)
		}
		return nil, errors.New("a USB installation remains reserved; reconcile or close that operation first")
	}
	return &operationGate{file: lock}, nil
}

func operationActiveAt(directoryPath string, managed bool) (bool, error) {
	directory, err := openCoordinationDirectory(directoryPath, managed)
	if err != nil {
		return false, err
	}
	defer directory.Close()
	if present, markerErr := inspectReservationAt(directory); present {
		if markerErr != nil {
			return true, markerErr
		}
		return true, nil
	}
	descriptor, err := syscall.Openat(int(directory.Fd()), coordinationLockName, syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return false, fmt.Errorf("open managed operation lock: %w", err)
	}
	lock := os.NewFile(uintptr(descriptor), filepath.Join(directoryPath, coordinationLockName))
	defer lock.Close()
	if err := validateCoordinationLock(lock, managed); err != nil {
		return false, err
	}
	if err := syscall.Flock(descriptor, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return true, nil
		}
		return false, fmt.Errorf("inspect managed operation lock: %w", err)
	}
	_ = syscall.Flock(descriptor, syscall.LOCK_UN)
	return false, nil
}

func openCoordinationDirectory(path string, managed bool) (*os.File, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("coordination directory must be absolute")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect coordination directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("coordination path must be a real directory")
	}
	if managed {
		if info.Mode().Perm() != 0770 {
			return nil, fmt.Errorf("managed coordination directory mode is %04o, want 0770", info.Mode().Perm())
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != 0 {
			return nil, errors.New("managed coordination directory must be owned by root")
		}
		group, lookupErr := user.LookupGroup("nixorium-operations")
		if lookupErr != nil {
			return nil, errors.New("nixorium-operations group is unavailable")
		}
		groupID, conversionErr := strconv.ParseUint(group.Gid, 10, 32)
		if conversionErr != nil || stat.Gid != uint32(groupID) {
			return nil, errors.New("managed coordination directory has the wrong group")
		}
	}
	descriptor, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open coordination directory: %w", err)
	}
	return os.NewFile(uintptr(descriptor), path), nil
}

func validateCoordinationLock(lock *os.File, managed bool) error {
	info, err := lock.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("managed operation lock must be a regular file")
	}
	expectedMode := os.FileMode(0600)
	if managed {
		expectedMode = 0660
	}
	if info.Mode().Perm() != expectedMode {
		return fmt.Errorf("managed operation lock mode is %04o, want %04o", info.Mode().Perm(), expectedMode)
	}
	if managed {
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != 0 {
			return errors.New("managed operation lock must be owned by root")
		}
		group, lookupErr := user.LookupGroup("nixorium-operations")
		if lookupErr != nil {
			return errors.New("nixorium-operations group is unavailable")
		}
		groupID, conversionErr := strconv.ParseUint(group.Gid, 10, 32)
		if conversionErr != nil || stat.Gid != uint32(groupID) {
			return errors.New("managed operation lock has the wrong group")
		}
	}
	return nil
}

func inspectReservationAt(directory *os.File) (bool, error) {
	descriptor, err := syscall.Openat(int(directory.Fd()), remoteReservationName, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, syscall.ENOENT) {
		return false, nil
	}
	if err != nil {
		return true, fmt.Errorf("reservation is unsafe or unreadable: %w", err)
	}
	file := os.NewFile(uintptr(descriptor), remoteReservationName)
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() < 2 || info.Size() > domain.RemoteInstallPlanMaxBytes {
		return true, errors.New("reservation is not a private bounded regular file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return true, errors.New("reservation belongs to another user")
	}
	return true, nil
}

func ensureTestCoordinationDirectory(stateRoot string) (string, error) {
	directory := filepath.Join(stateRoot, "nixorium", "coordination")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	lockPath := filepath.Join(directory, coordinationLockName)
	file, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return "", err
	}
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return "", err
	}
	return directory, file.Close()
}

type remoteInstallReservation struct {
	directoryPath string
	operationID   string
	gate          *operationGate
}

type remoteReservationRecord struct {
	SchemaVersion int    `json:"schemaVersion"`
	OperationID   string `json:"operationId"`
	StatePath     string `json:"statePath"`
	TokenDigest   string `json:"tokenDigest"`
}

func (Local) ReserveRemoteInstall(_ context.Context, plan domain.RemoteInstallPlan, reviewToken string) (domain.RemoteInstallReservation, error) {
	return reserveRemoteInstallAt(managedCoordinationDirectory, true, plan, reviewToken)
}

func (Local) ReserveRemoteSession(operationID string) (domain.RemoteInstallReservation, error) {
	if !remoteStateID(operationID) {
		return nil, errors.New("remote installation operation ID is invalid")
	}
	return createRemoteReservationAt(managedCoordinationDirectory, true, operationID, "")
}

// RecoverRemoteSession reacquires the process-scoped flock for the one
// persistent USB reservation left behind by a terminated worker. It never
// creates, replaces, or removes the marker: callers must reconcile the bound
// state before they may release it.
func (Local) RecoverRemoteSession() (string, domain.RemoteInstallReservation, bool, error) {
	return recoverRemoteReservationAt(managedCoordinationDirectory, true)
}

func recoverRemoteReservationAt(directoryPath string, managed bool) (string, domain.RemoteInstallReservation, bool, error) {
	directory, err := openCoordinationDirectory(directoryPath, managed)
	if err != nil {
		return "", nil, false, err
	}
	record, present, err := readRemoteReservationAt(directory)
	_ = directory.Close()
	if !present {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, true, fmt.Errorf("recover USB installation reservation: %w", err)
	}

	gate, err := acquireRecoveryGateAt(directoryPath, managed)
	if err != nil {
		return "", nil, true, err
	}
	directory, err = openCoordinationDirectory(directoryPath, managed)
	if err != nil {
		gate.Close()
		return "", nil, true, err
	}
	confirmed, stillPresent, readErr := readRemoteReservationAt(directory)
	_ = directory.Close()
	if readErr != nil || !stillPresent || confirmed != record {
		gate.Close()
		return "", nil, true, errors.New("USB installation reservation changed while it was being recovered")
	}
	return record.OperationID, &remoteInstallReservation{directoryPath: directoryPath, operationID: record.OperationID, gate: gate}, true, nil
}

func acquireRecoveryGateAt(directoryPath string, managed bool) (*operationGate, error) {
	directory, err := openCoordinationDirectory(directoryPath, managed)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	descriptor, err := syscall.Openat(int(directory.Fd()), coordinationLockName, syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open managed operation lock: %w", err)
	}
	lock := os.NewFile(uintptr(descriptor), filepath.Join(directoryPath, coordinationLockName))
	if err := validateCoordinationLock(lock, managed); err != nil {
		lock.Close()
		return nil, err
	}
	if err := syscall.Flock(descriptor, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("another Nixorium controller or client operation is already running")
	}
	gate := &operationGate{file: lock}
	if managed {
		legacy, legacyErr := acquireLegacyDeploymentLock()
		if legacyErr != nil {
			gate.Close()
			return nil, legacyErr
		}
		gate.legacy = legacy
	}
	return gate, nil
}

func readRemoteReservationAt(directory *os.File) (remoteReservationRecord, bool, error) {
	var record remoteReservationRecord
	descriptor, err := syscall.Openat(int(directory.Fd()), remoteReservationName, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, syscall.ENOENT) {
		return record, false, nil
	}
	if err != nil {
		return record, true, fmt.Errorf("reservation is unsafe or unreadable: %w", err)
	}
	file := os.NewFile(uintptr(descriptor), remoteReservationName)
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() < 2 || info.Size() > domain.RemoteInstallPlanMaxBytes {
		return record, true, errors.New("reservation is not a private bounded regular file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return record, true, errors.New("reservation belongs to another user")
	}
	data, err := io.ReadAll(io.LimitReader(file, domain.RemoteInstallPlanMaxBytes+1))
	if err != nil || domain.ValidateStrictRemoteJSON(data, domain.RemoteInstallPlanMaxBytes) != nil {
		return record, true, errors.New("reservation record is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return record, true, errors.New("reservation record is invalid")
	}
	if record.SchemaVersion != 1 || !remoteStateID(record.OperationID) ||
		record.StatePath != filepath.Join("/var/lib/nixorium/remote-install", record.OperationID+".json") ||
		(record.TokenDigest != "" && (len(record.TokenDigest) != len("sha256:")+64 || record.TokenDigest[:len("sha256:")] != "sha256:" || !stringsHasHexDigest(record.TokenDigest[len("sha256:"):]))) {
		return record, true, errors.New("reservation record does not bind valid state")
	}
	return record, true, nil
}

func reserveRemoteInstallAt(directoryPath string, managed bool, plan domain.RemoteInstallPlan, reviewToken string) (domain.RemoteInstallReservation, error) {
	if err := domain.ValidateRemoteInstallPlan(plan); err != nil {
		return nil, err
	}
	if len(reviewToken) != len("sha256:")+64 || !stringsHasHexDigest(reviewToken[len("sha256:"):]) {
		return nil, errors.New("remote installation review token is invalid")
	}
	digest := sha256.Sum256([]byte(reviewToken))
	return createRemoteReservationAt(directoryPath, managed, plan.OperationID, "sha256:"+hex.EncodeToString(digest[:]))
}

func createRemoteReservationAt(directoryPath string, managed bool, operationID, tokenDigest string) (domain.RemoteInstallReservation, error) {
	var gate *operationGate
	var err error
	if managed {
		gate, err = acquireManagedOperationGate()
	} else {
		gate, err = acquireOperationGateAt(directoryPath, false)
	}
	if err != nil {
		return nil, err
	}
	directory, err := openCoordinationDirectory(directoryPath, managed)
	if err != nil {
		gate.Close()
		return nil, err
	}
	defer directory.Close()
	descriptor, err := syscall.Openat(int(directory.Fd()), remoteReservationName, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		gate.Close()
		return nil, fmt.Errorf("create USB installation reservation: %w", err)
	}
	file := os.NewFile(uintptr(descriptor), filepath.Join(directoryPath, remoteReservationName))
	record := remoteReservationRecord{1, operationID, filepath.Join("/var/lib/nixorium/remote-install", operationID+".json"), tokenDigest}
	content, _ := json.Marshal(record)
	content = append(content, '\n')
	if _, err := file.Write(content); err != nil {
		file.Close()
		_ = os.Remove(filepath.Join(directoryPath, remoteReservationName))
		gate.Close()
		return nil, fmt.Errorf("write USB installation reservation: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		gate.Close()
		return nil, fmt.Errorf("sync USB installation reservation: %w", err)
	}
	if err := file.Close(); err != nil {
		gate.Close()
		return nil, err
	}
	return &remoteInstallReservation{directoryPath: directoryPath, operationID: operationID, gate: gate}, nil
}

func (reservation *remoteInstallReservation) ReleaseResolved() error {
	if reservation == nil || reservation.gate == nil {
		return nil
	}
	directory, err := openCoordinationDirectory(reservation.directoryPath, reservation.directoryPath == managedCoordinationDirectory)
	if err != nil {
		return err
	}
	defer directory.Close()
	descriptor, err := syscall.Openat(int(directory.Fd()), remoteReservationName, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open USB installation reservation: %w", err)
	}
	file := os.NewFile(uintptr(descriptor), remoteReservationName)
	info, statErr := file.Stat()
	if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() < 2 || info.Size() > domain.RemoteInstallPlanMaxBytes {
		file.Close()
		return errors.New("USB installation reservation is not a private bounded regular file")
	}
	data, readErr := io.ReadAll(io.LimitReader(file, domain.RemoteInstallPlanMaxBytes+1))
	closeFileErr := file.Close()
	if readErr != nil || closeFileErr != nil {
		return errors.New("read USB installation reservation")
	}
	var record struct {
		OperationID string `json:"operationId"`
	}
	if err := json.Unmarshal(data, &record); err != nil || record.OperationID != reservation.operationID {
		return errors.New("USB installation reservation no longer matches the operation")
	}
	if err := os.Remove(filepath.Join(reservation.directoryPath, remoteReservationName)); err != nil {
		return fmt.Errorf("remove resolved USB installation reservation: %w", err)
	}
	err = directory.Sync()
	closeErr := reservation.gate.Close()
	reservation.gate = nil
	if err != nil {
		return err
	}
	return closeErr
}

func stringsHasHexDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

var _ io.Closer = (*operationGate)(nil)
