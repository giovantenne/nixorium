package adapters

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

const shutdownSessionCommand = "nixorium-session-state"

type clientOperationLease struct{ file *os.File }

func (lease *clientOperationLease) Close() error {
	if lease == nil || lease.file == nil {
		return nil
	}
	_ = syscall.Flock(int(lease.file.Fd()), syscall.LOCK_UN)
	err := lease.file.Close()
	lease.file = nil
	return err
}

func (Local) ClientOperationActive() (bool, error) {
	stateRoot, err := userStateRoot()
	if err != nil {
		return false, err
	}
	directory, err := openOperationLogDirectory(stateRoot)
	if errors.Is(err, syscall.ENOENT) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer directory.Close()
	descriptor, err := syscall.Openat(int(directory.Fd()), "deploy.lock", syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, syscall.ENOENT) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open client operation lock: %w", err)
	}
	lock := os.NewFile(uintptr(descriptor), "deploy.lock")
	defer lock.Close()
	if err := validatePrivateOwnedFile(lock, syscall.S_IFREG, 0600); err != nil {
		return false, fmt.Errorf("inspect client operation lock: %w", err)
	}
	if err := syscall.Flock(descriptor, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return true, nil
		}
		return false, fmt.Errorf("inspect client operation lock: %w", err)
	}
	_ = syscall.Flock(descriptor, syscall.LOCK_UN)
	return false, nil
}

func (Local) AcquireClientOperation() (io.Closer, error) {
	stateRoot, err := userStateRoot()
	if err != nil {
		return nil, err
	}
	productDirectory := filepath.Join(stateRoot, "nixorium")
	if err := os.MkdirAll(productDirectory, 0700); err != nil {
		return nil, fmt.Errorf("create operation state directory: %w", err)
	}
	if err := requirePrivateDirectory(productDirectory); err != nil {
		return nil, err
	}
	directoryPath := filepath.Join(productDirectory, "operations")
	if err := os.Mkdir(directoryPath, 0700); err != nil && !os.IsExist(err) {
		return nil, fmt.Errorf("create operation log directory: %w", err)
	}
	directory, err := openOperationLogDirectory(stateRoot)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	descriptor, err := syscall.Openat(int(directory.Fd()), "deploy.lock", syscall.O_RDWR|syscall.O_CREAT|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, fmt.Errorf("open client operation lock: %w", err)
	}
	lock := os.NewFile(uintptr(descriptor), "deploy.lock")
	if err := lock.Chmod(0600); err != nil {
		lock.Close()
		return nil, fmt.Errorf("secure client operation lock: %w", err)
	}
	if err := validatePrivateOwnedFile(lock, syscall.S_IFREG, 0600); err != nil {
		lock.Close()
		return nil, fmt.Errorf("inspect client operation lock: %w", err)
	}
	if err := syscall.Flock(descriptor, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("another Nixorium client operation is already running")
	}
	return &clientOperationLease{file: lock}, nil
}

func (Local) ShutdownObservations(ctx context.Context, hosts []domain.HostMeta, timeout time.Duration) map[string]domain.ShutdownObservation {
	tcp := probeSSHStatuses(ctx, hosts, timeout, probeHostSSH)
	type result struct {
		name        string
		observation domain.ShutdownObservation
	}
	results := make(chan result, len(hosts))
	jobs := make(chan domain.HostMeta)
	workers := min(len(hosts), maximumConcurrentSSHProbes)
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			for host := range jobs {
				probe := tcp[host.Name]
				observation := domain.ShutdownObservation{Reachability: probe.Reachability, SSH: probe.SSH, Session: domain.ShutdownSessionUnknown, Detail: probe.Detail}
				if probe.SSH == domain.SSHAvailable {
					observation = observeShutdownSession(ctx, host, timeout)
				}
				results <- result{name: host.Name, observation: observation}
			}
		}()
	}
	go func() {
		for _, host := range hosts {
			jobs <- host
		}
		close(jobs)
		group.Wait()
		close(results)
	}()
	observations := make(map[string]domain.ShutdownObservation, len(hosts))
	for range hosts {
		entry := <-results
		observations[entry.name] = entry.observation
	}
	return observations
}

func observeShutdownSession(ctx context.Context, host domain.HostMeta, timeout time.Duration) domain.ShutdownObservation {
	commandContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(commandContext, "ssh", shutdownSSHArguments(host, timeout, shutdownSessionCommand)...)
	output := &boundedCommandBuffer{limit: 8 * 1024}
	command.Stdout = output
	command.Stderr = output
	err := command.Run()
	detail := strings.TrimSpace(sanitizeOperationLog(output.buffer.Bytes()))
	if output.truncated {
		detail += " (output truncated)"
	}
	if err != nil {
		sshState := domain.SSHAvailable
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) || exitError.ExitCode() == 255 {
			sshState = domain.SSHUnavailable
		}
		if detail == "" {
			detail = err.Error()
		}
		return domain.ShutdownObservation{Reachability: domain.ReachabilityReachable, SSH: sshState, Session: domain.ShutdownSessionUnknown, Detail: detail}
	}
	switch detail {
	case string(domain.ShutdownSessionIdle):
		return domain.ShutdownObservation{Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Session: domain.ShutdownSessionIdle}
	case string(domain.ShutdownSessionActive):
		return domain.ShutdownObservation{Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Session: domain.ShutdownSessionActive, Detail: "an interactive user session is active"}
	default:
		return domain.ShutdownObservation{Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Session: domain.ShutdownSessionUnknown, Detail: "session helper returned an invalid response"}
	}
}

func (Local) DispatchShutdowns(ctx context.Context, hosts []domain.HostMeta, timeout time.Duration) map[string]domain.ShutdownDispatchResult {
	type result struct {
		name     string
		dispatch domain.ShutdownDispatchResult
	}
	results := make(chan result, len(hosts))
	jobs := make(chan domain.HostMeta)
	workers := min(len(hosts), maximumConcurrentSSHProbes)
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			for host := range jobs {
				results <- result{name: host.Name, dispatch: dispatchShutdown(ctx, host, timeout)}
			}
		}()
	}
	go func() {
		for _, host := range hosts {
			jobs <- host
		}
		close(jobs)
		group.Wait()
		close(results)
	}()
	dispatched := make(map[string]domain.ShutdownDispatchResult, len(hosts))
	for range hosts {
		entry := <-results
		dispatched[entry.name] = entry.dispatch
	}
	return dispatched
}

func dispatchShutdown(ctx context.Context, host domain.HostMeta, timeout time.Duration) domain.ShutdownDispatchResult {
	commandContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(commandContext, "ssh", shutdownSSHArguments(host, timeout, "systemctl", "poweroff", "--no-block")...)
	output := &boundedCommandBuffer{limit: 8 * 1024}
	command.Stdout = output
	command.Stderr = output
	if err := command.Run(); err != nil {
		detail := strings.TrimSpace(sanitizeOperationLog(output.buffer.Bytes()))
		if detail == "" {
			detail = err.Error()
		}
		if output.truncated {
			detail += " (output truncated)"
		}
		return domain.ShutdownDispatchResult{Detail: detail}
	}
	return domain.ShutdownDispatchResult{Accepted: true}
}

func shutdownSSHArguments(host domain.HostMeta, timeout time.Duration, remote ...string) []string {
	seconds := max(1, int(timeout.Round(time.Second)/time.Second))
	arguments := []string{
		"-T",
		"-o", "BatchMode=yes",
		"-o", fmt.Sprintf("ConnectTimeout=%d", seconds),
		"-o", "ConnectionAttempts=1",
		"-o", "PasswordAuthentication=no",
		"-o", "KbdInteractiveAuthentication=no",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ClearAllForwardings=yes",
		"-o", "ForwardAgent=no",
		"-o", "PermitLocalCommand=no",
		"-o", "LogLevel=ERROR",
		"root@" + host.IP,
	}
	return append(arguments, remote...)
}
