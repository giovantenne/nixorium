package adapters

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
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
	return managedOperationActive()
}

func (Local) AcquireClientOperation() (io.Closer, error) {
	return acquireManagedOperationGate()
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
