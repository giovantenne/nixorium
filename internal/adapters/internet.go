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

func internetSSH(ctx context.Context, host domain.HostMeta, arguments ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	args := shutdownSSHArguments(host, 3*time.Second, append([]string{"nixorium-internet"}, arguments...)...)
	// Keep the controller's managed host-key file stable during runtime actions.
	args = append([]string{"-o", "UpdateHostKeys=no"}, args...)
	command := exec.CommandContext(ctx, "ssh", args...)
	stdout := &boundedCommandBuffer{limit: 4096}
	stderr := &boundedCommandBuffer{limit: 4096}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("client helper: %s (%w)", strings.TrimSpace(sanitizeOperationLog(stderr.buffer.Bytes())), err)
	}
	if stdout.truncated {
		return nil, errors.New("client helper response exceeded limit")
	}
	return stdout.buffer.Bytes(), nil
}
func decodeInternetObservation(data []byte) domain.InternetObservation {
	var state domain.InternetObservation
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&state)
	if err != nil || decoder.Decode(new(any)) != io.EOF || !state.Valid() {
		return domain.InternetObservation{State: "unknown", Detail: "Invalid client response; update the client before using Internet control."}
	}
	return state
}
func (Local) ObserveInternet(ctx context.Context, host domain.HostMeta) domain.InternetObservation {
	data, err := internetSSH(ctx, host, "status")
	if err != nil {
		return domain.InternetObservation{State: "unknown", Detail: err.Error()}
	}
	return decodeInternetObservation(data)
}
func (Local) ChangeInternet(ctx context.Context, host domain.HostMeta, action domain.InternetAction, bootID string) error {
	if !action.Valid() || !(domain.InternetObservation{SchemaVersion: 1, BootID: bootID, State: "enabled"}).Valid() {
		return errors.New("invalid Internet action or boot identity")
	}
	_, err := internetSSH(ctx, host, string(action), bootID)
	return err
}
