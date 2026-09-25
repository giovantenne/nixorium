package adapters

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func PublishRemoteOperationLog(operationID string, content []byte, result string) (string, error) {
	if !remoteStateID(operationID) || len(content) > 1024*1024 {
		return "", errors.New("remote operation log identity or size is invalid")
	}
	switch result {
	case "completed", "failed", "partial", "blocked":
	default:
		return "", errors.New("remote operation log result is invalid")
	}
	stateRoot, err := userStateRoot()
	if err != nil {
		return "", err
	}
	productDirectory := filepath.Join(stateRoot, "nixorium")
	operationsDirectory := filepath.Join(productDirectory, "operations")
	for _, directory := range []string{stateRoot, productDirectory, operationsDirectory} {
		if err := os.Mkdir(directory, 0700); err != nil && !os.IsExist(err) {
			return "", err
		}
		if err := requirePrivateDirectory(directory); err != nil {
			return "", err
		}
	}
	name := "usb-install-" + operationID + ".log"
	path := filepath.Join(operationsDirectory, name)
	descriptor, err := syscall.Open(path, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if errors.Is(err, syscall.EEXIST) {
		existingDescriptor, openErr := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
		if openErr != nil {
			return "", fmt.Errorf("inspect existing remote operation log: %w", openErr)
		}
		existing := os.NewFile(uintptr(existingDescriptor), path)
		defer existing.Close()
		var stat syscall.Stat_t
		if err := syscall.Fstat(existingDescriptor, &stat); err != nil {
			return "", fmt.Errorf("inspect existing remote operation log: %w", err)
		}
		if stat.Mode&syscall.S_IFMT != syscall.S_IFREG || stat.Mode&0777 != 0600 || stat.Uid != uint32(os.Geteuid()) {
			return "", errors.New("existing remote operation log is unsafe")
		}
		return name, nil
	}
	if err != nil {
		return "", err
	}
	file := os.NewFile(uintptr(descriptor), path)
	defer file.Close()
	sanitized := []byte(sanitizeOperationLog(content))
	var output bytes.Buffer
	fmt.Fprintf(&output, "Operation: USB SSH client installation\nID: %s\n", operationID)
	output.Write(sanitized)
	if output.Len() > 0 && output.Bytes()[output.Len()-1] != '\n' {
		output.WriteByte('\n')
	}
	fmt.Fprintf(&output, "Result: %s\n", result)
	if _, err := file.Write(output.Bytes()); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	return name, nil
}

type DeploymentOperation struct {
	Path         string
	file         *os.File
	lock         *os.File
	coordination *operationGate
}

func OpenDeploymentOperation() (*DeploymentOperation, error) {
	stateRoot, err := userStateRoot()
	if err != nil {
		return nil, err
	}
	gate, err := acquireOperationGateAt(managedCoordinationDirectory, true)
	if err != nil {
		return nil, err
	}
	return openDeploymentOperationWithGate(stateRoot, time.Now(), gate)
}

func openDeploymentOperation(stateRoot string, now time.Time) (*DeploymentOperation, error) {
	coordinationDirectory, err := ensureTestCoordinationDirectory(stateRoot)
	if err != nil {
		return nil, err
	}
	gate, err := acquireOperationGateAt(coordinationDirectory, false)
	if err != nil {
		return nil, err
	}
	return openDeploymentOperationWithGate(stateRoot, now, gate)
}

func openDeploymentOperationWithGate(stateRoot string, now time.Time, gate *operationGate) (*DeploymentOperation, error) {
	fail := func(err error) (*DeploymentOperation, error) {
		_ = gate.Close()
		return nil, err
	}
	if !filepath.IsAbs(stateRoot) {
		return fail(errors.New("operation state directory must be absolute"))
	}
	productDirectory := filepath.Join(stateRoot, "nixorium")
	if err := os.MkdirAll(productDirectory, 0700); err != nil {
		return fail(fmt.Errorf("create operation state directory: %w", err))
	}
	if err := requirePrivateDirectory(productDirectory); err != nil {
		return fail(err)
	}
	directory := filepath.Join(productDirectory, "operations")
	if err := os.Mkdir(directory, 0700); err != nil && !os.IsExist(err) {
		return fail(fmt.Errorf("create operation log directory: %w", err))
	}
	if err := requirePrivateDirectory(directory); err != nil {
		return fail(err)
	}

	lockPath := filepath.Join(directory, "deploy.lock")
	lockDescriptor, err := syscall.Open(lockPath, syscall.O_RDWR|syscall.O_CREAT|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return fail(fmt.Errorf("open deployment lock: %w", err))
	}
	lock := os.NewFile(uintptr(lockDescriptor), lockPath)
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() {
		lock.Close()
		return fail(errors.New("deployment lock parent is not a directory"))
	}
	lockInfo, err := lock.Stat()
	if err != nil || !lockInfo.Mode().IsRegular() {
		lock.Close()
		return fail(errors.New("deployment lock must be a regular file and not a symlink"))
	}
	if err := lock.Chmod(0600); err != nil {
		lock.Close()
		return fail(fmt.Errorf("secure deployment lock: %w", err))
	}
	if err := syscall.Flock(lockDescriptor, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return fail(errors.New("another Nixorium deployment is already running"))
	}

	name := fmt.Sprintf("deploy-%s-%d.log", now.UTC().Format("20060102T150405.000000000Z"), os.Getpid())
	path := filepath.Join(directory, name)
	fileDescriptor, err := syscall.Open(path, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		syscall.Flock(lockDescriptor, syscall.LOCK_UN)
		lock.Close()
		return fail(fmt.Errorf("create deployment log: %w", err))
	}
	return &DeploymentOperation{Path: path, file: os.NewFile(uintptr(fileDescriptor), path), lock: lock, coordination: gate}, nil
}

func requirePrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect operation directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("operation path must be a directory and not a symlink")
	}
	if err := os.Chmod(path, 0700); err != nil {
		return fmt.Errorf("secure operation directory: %w", err)
	}
	return nil
}

func (o *DeploymentOperation) Writer() io.Writer {
	return o.file
}

func (o *DeploymentOperation) Close() error {
	var result error
	if o.file != nil {
		if err := o.file.Sync(); err != nil {
			result = err
		}
		if err := o.file.Close(); err != nil && result == nil {
			result = err
		}
		o.file = nil
	}
	if o.lock != nil {
		_ = syscall.Flock(int(o.lock.Fd()), syscall.LOCK_UN)
		if err := o.lock.Close(); err != nil && result == nil {
			result = err
		}
		o.lock = nil
	}
	if o.coordination != nil {
		if err := o.coordination.Close(); err != nil && result == nil {
			result = err
		}
		o.coordination = nil
	}
	return result
}
