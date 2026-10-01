package adapters

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

const cleanupHelperCommand = "nixorium-clean-generations"
const cleanupControllerHelper = "/run/current-system/sw/bin/" + cleanupHelperCommand
const cleanupConnectTimeout = 5 * time.Second

var cleanupUnitPattern = regexp.MustCompile(`^nixorium-clean-generations@[0-9a-f]{16}\.service$`)
var cleanupDigestPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// ObserveControllerGenerations runs the read-only plan mode locally.
func (Local) ObserveControllerGenerations(ctx context.Context) domain.CleanupObservation {
	observation := domain.CleanupObservation{Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable}
	commandContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	command := exec.CommandContext(commandContext, cleanupControllerHelper, "--plan")
	configureCommandCancellation(command)
	output := &boundedCommandBuffer{limit: 64 * 1024}
	command.Stdout = output
	if err := command.Run(); err != nil {
		observation.Detail = "the cleanup helper is not available; apply the saved configuration to this controller first"
		return observation
	}
	return cleanupObservationFrom(observation, output.buffer.String(), output.truncated)
}

func (Local) CleanControllerGenerations(ctx context.Context, expect string) error {
	if !cleanupDigestPattern.MatchString(expect) {
		return errors.New("reviewed removal digest is invalid")
	}
	return (Local{}).ControlSystemUnit(ctx, "start", "nixorium-clean-generations@"+expect+".service")
}

func (Local) ObserveClientGenerations(ctx context.Context, hosts []domain.HostMeta, timeout time.Duration) map[string]domain.CleanupObservation {
	tcp := probeSSHStatuses(ctx, hosts, timeout, probeHostSSH)
	return forEachHost(hosts, func(host domain.HostMeta) domain.CleanupObservation {
		probe := tcp[host.Name]
		observation := domain.CleanupObservation{Reachability: probe.Reachability, SSH: probe.SSH, Detail: probe.Detail}
		if probe.SSH != domain.SSHAvailable {
			if observation.Detail == "" {
				observation.Detail = "off or not reachable"
			}
			return observation
		}
		output, truncated, err := runCleanupSSH(ctx, host, timeout, "--plan")
		if err != nil {
			observation.Detail = cleanupSSHDetail(output, err)
			return observation
		}
		return cleanupObservationFrom(observation, output, truncated)
	})
}

func (Local) CleanClientGenerations(ctx context.Context, expect map[string]string, hosts []domain.HostMeta, timeout time.Duration) map[string]domain.CleanupDispatchResult {
	return forEachHost(hosts, func(host domain.HostMeta) domain.CleanupDispatchResult {
		digest := expect[host.Name]
		if !cleanupDigestPattern.MatchString(digest) {
			return domain.CleanupDispatchResult{State: "failed", Detail: "reviewed removal digest is invalid"}
		}
		output, truncated, err := runCleanupSSH(ctx, host, timeout, "--apply", digest)
		result, ok := domain.ParseCleanupApply(output)
		if ok && !truncated && (err == nil || result.State == "changed") {
			return result
		}
		detail := cleanupSSHDetail(output, err)
		if err == nil {
			detail = "the cleanup helper returned an invalid result"
		}
		return domain.CleanupDispatchResult{State: "failed", Detail: detail}
	})
}

func cleanupObservationFrom(observation domain.CleanupObservation, output string, truncated bool) domain.CleanupObservation {
	parsed, ok := domain.ParseCleanupPlan(output)
	if !ok || truncated {
		observation.Detail = "the cleanup helper returned an invalid answer; update this computer first"
		return observation
	}
	parsed.Reachability, parsed.SSH = observation.Reachability, observation.SSH
	return parsed
}

// runCleanupSSH runs the fixed helper as root with a fixed argument array.
func runCleanupSSH(ctx context.Context, host domain.HostMeta, timeout time.Duration, arguments ...string) (string, bool, error) {
	commandContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	remote := append([]string{cleanupHelperCommand}, arguments...)
	command := exec.CommandContext(commandContext, "ssh", shutdownSSHArguments(host, cleanupConnectTimeout, remote...)...)
	configureCommandCancellation(command)
	output := &boundedCommandBuffer{limit: 64 * 1024}
	command.Stdout = output
	err := command.Run()
	return output.buffer.String(), output.truncated, err
}

func cleanupSSHDetail(output string, err error) string {
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		switch exitError.ExitCode() {
		case 255:
			return "management access failed"
		case 127:
			return "the cleanup helper is not installed; update this computer first"
		}
		return fmt.Sprintf("the cleanup helper failed (exit %d)", exitError.ExitCode())
	}
	if detail := strings.TrimSpace(sanitizeOperationLog([]byte(output))); detail != "" {
		return detail
	}
	return err.Error()
}

func forEachHost[T any](hosts []domain.HostMeta, observe func(domain.HostMeta) T) map[string]T {
	results := make(map[string]T, len(hosts))
	var mutex sync.Mutex
	jobs := make(chan domain.HostMeta)
	var group sync.WaitGroup
	workers := min(len(hosts), maximumConcurrentSSHProbes)
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			for host := range jobs {
				value := observe(host)
				mutex.Lock()
				results[host.Name] = value
				mutex.Unlock()
			}
		}()
	}
	for _, host := range hosts {
		jobs <- host
	}
	close(jobs)
	group.Wait()
	return results
}
