package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// fileRecoveryHelper is the student computer's helper, started as root
// over SSH with its one request on standard input.
const fileRecoveryHelper = "nixorium-home-recover"

// Reading sessions and folders takes seconds; writing up to 500 MB onto
// the student's desktop takes longer.
const (
	fileRecoveryReadTimeout    = 30 * time.Second
	fileRecoveryRestoreTimeout = 16 * time.Minute
)

// FileRecovery sends one request to a student computer's recovery helper
// and returns its answer. A refusal the helper explains comes back in the
// answer's Error; an error means the computer could not be asked.
func (Local) FileRecovery(ctx context.Context, host domain.HostMeta, request domain.FileRecoveryHelperRequest) (domain.FileRecoveryHelperAnswer, error) {
	request.SchemaVersion = domain.FileRecoveryProtocolVersion
	timeout := fileRecoveryReadTimeout
	if request.Operation == domain.FileRecoveryHelperRestore {
		timeout = fileRecoveryRestoreTimeout
	}
	input, err := json.Marshal(request)
	if err != nil {
		return domain.FileRecoveryHelperAnswer{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := shutdownSSHArguments(host, 5*time.Second, fileRecoveryHelper)
	// Keep the controller's managed host-key file stable during runtime actions.
	args = append([]string{"-o", "UpdateHostKeys=no"}, args...)
	command := exec.CommandContext(ctx, "ssh", args...)
	command.Stdin = bytes.NewReader(input)
	stdout := &boundedCommandBuffer{limit: domain.FileRecoveryMaxAnswerBytes}
	stderr := &boundedCommandBuffer{limit: 4096}
	command.Stdout, command.Stderr = stdout, stderr
	command.WaitDelay = 5 * time.Second
	runErr := command.Run()
	if stdout.truncated {
		return domain.FileRecoveryHelperAnswer{}, errors.New("the student computer's answer is too large")
	}
	answer, decodeErr := decodeFileRecoveryAnswer(stdout.buffer.Bytes())
	if decodeErr == nil && answer.Error != "" {
		return answer, nil
	}
	if runErr != nil {
		detail := strings.TrimSpace(sanitizeOperationLog(stderr.buffer.Bytes()))
		if strings.Contains(detail, "command not found") || strings.Contains(detail, "No such file") {
			return domain.FileRecoveryHelperAnswer{}, errors.New("the student computer does not have file recovery yet; run Update computers first")
		}
		if detail == "" {
			detail = "the computer did not answer; check that it is on"
		}
		return domain.FileRecoveryHelperAnswer{}, fmt.Errorf("%s (%w)", detail, runErr)
	}
	if decodeErr != nil {
		return domain.FileRecoveryHelperAnswer{}, errors.New("the student computer gave an answer that is not understood; run Update computers first")
	}
	return answer, nil
}

func decodeFileRecoveryAnswer(data []byte) (domain.FileRecoveryHelperAnswer, error) {
	var answer domain.FileRecoveryHelperAnswer
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&answer); err != nil {
		return answer, err
	}
	if !errors.Is(decoder.Decode(new(any)), io.EOF) || answer.SchemaVersion != domain.FileRecoveryProtocolVersion {
		return answer, errors.New("unexpected answer")
	}
	return answer, nil
}
