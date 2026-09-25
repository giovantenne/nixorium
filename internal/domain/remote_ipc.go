package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var remoteReviewTokenPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type RemoteInstallOperation string

const (
	RemoteInstallBootstrapOperation   RemoteInstallOperation = "bootstrap"
	RemoteInstallPrepareOperation     RemoteInstallOperation = "prepare"
	RemoteInstallPlanOperation        RemoteInstallOperation = "plan"
	RemoteInstallApplyOperation       RemoteInstallOperation = "apply"
	RemoteInstallStatusOperation      RemoteInstallOperation = "status"
	RemoteInstallCancelOperation      RemoteInstallOperation = "cancel-before-apply"
	RemoteInstallRebootOperation      RemoteInstallOperation = "reboot"
	RemoteInstallVerifyOperation      RemoteInstallOperation = "verify"
	RemoteInstallCloseOperation       RemoteInstallOperation = "close"
	RemoteInstallReconcileOperation   RemoteInstallOperation = "reconcile"
	RemoteInstallWorkerProbeOperation RemoteInstallOperation = "worker-probe"
)

type RemoteInstallRequest struct {
	SchemaVersion   int                    `json:"schemaVersion"`
	RequestID       string                 `json:"requestId"`
	Operation       RemoteInstallOperation `json:"operation"`
	OperationID     string                 `json:"operationId,omitempty"`
	Host            string                 `json:"host,omitempty"`
	Address         string                 `json:"address,omitempty"`
	Fingerprint     string                 `json:"fingerprint,omitempty"`
	Disk            string                 `json:"disk,omitempty"`
	HostKeyRotation bool                   `json:"hostKeyRotation,omitempty"`
	ReviewToken     string                 `json:"reviewToken,omitempty"`
	Confirmation    string                 `json:"confirmation,omitempty"`
}

type RemoteInstallResponse struct {
	SchemaVersion int                           `json:"schemaVersion"`
	RequestID     string                        `json:"requestId"`
	State         string                        `json:"state"`
	OperationID   string                        `json:"operationId,omitempty"`
	Message       string                        `json:"message,omitempty"`
	Session       *RemoteInstallSession         `json:"session,omitempty"`
	Plan          *RemoteInstallPlanReport      `json:"plan,omitempty"`
	Execution     *RemoteInstallExecutionReport `json:"execution,omitempty"`
}

// BootstrapVerified is an explicit success gate, not the absence of a failure
// state: artifacts-ready can also be returned after an unsuccessful bootstrap.
func (response RemoteInstallResponse) BootstrapVerified() bool {
	return remoteOperationIDPattern.MatchString(response.OperationID) &&
		(response.State == "bootstrapped" || response.State == "bootstrapped-artifacts")
}

func DecodeRemoteInstallRequest(data []byte) (RemoteInstallRequest, error) {
	var request RemoteInstallRequest
	if err := decodeRemoteJSON(data, RemoteInstallPlanMaxBytes, &request); err != nil {
		return request, fmt.Errorf("decode remote installation request: %w", err)
	}
	if request.SchemaVersion != RemoteInstallSchemaVersion || !remoteOperationIDPattern.MatchString(request.RequestID) {
		return request, errors.New("remote installation request identity is invalid")
	}
	if !validRemoteInstallOperation(request.Operation) {
		return request, errors.New("remote installation request operation is invalid")
	}
	if request.OperationID != "" && !remoteOperationIDPattern.MatchString(request.OperationID) {
		return request, errors.New("remote installation operation ID is invalid")
	}
	if request.Host != "" && !remoteHostPattern.MatchString(request.Host) {
		return request, errors.New("remote installation host is invalid")
	}
	if request.Operation == RemoteInstallPrepareOperation && request.Host == "" {
		return request, errors.New("remote installation prepare requires an inventory host")
	}
	if request.Operation != RemoteInstallPrepareOperation && request.Operation != RemoteInstallBootstrapOperation && request.Host != "" {
		return request, errors.New("remote installation host is only valid for bootstrap or prepare")
	}
	if request.Operation == RemoteInstallBootstrapOperation {
		if request.Host == "" || request.OperationID != "" || validateRemoteIPv4(request.Address) != nil || !remoteFingerprintPattern.MatchString(request.Fingerprint) {
			return request, errors.New("remote installation bootstrap identity is invalid")
		}
	} else if request.Address != "" || request.Fingerprint != "" {
		return request, errors.New("remote installation endpoint is only valid for bootstrap")
	}
	if request.Operation == RemoteInstallPlanOperation {
		if !remoteDevicePattern.MatchString(request.Disk) || request.ReviewToken != "" || request.Confirmation != "" {
			return request, errors.New("remote installation plan request is invalid")
		}
	} else if request.Operation == RemoteInstallApplyOperation {
		if request.Disk != "" || request.HostKeyRotation || !remoteReviewTokenPattern.MatchString(request.ReviewToken) || request.Confirmation == "" || len(request.Confirmation) > 512 {
			return request, errors.New("remote installation apply review is invalid")
		}
	} else if request.Disk != "" || request.HostKeyRotation || request.ReviewToken != "" || request.Confirmation != "" {
		return request, errors.New("remote installation review fields are only valid for plan or apply")
	}
	if remoteOperationNeedsID(request.Operation) && request.OperationID == "" {
		return request, errors.New("remote installation operation requires an operation ID")
	}
	return request, nil
}

func DecodeRemoteInstallSession(data []byte) (RemoteInstallSession, error) {
	var session RemoteInstallSession
	if err := decodeRemoteJSON(data, RemoteInstallFactsMaxBytes, &session); err != nil {
		return session, fmt.Errorf("decode remote installation session: %w", err)
	}
	if session.SchemaVersion != RemoteInstallSchemaVersion || !remoteOperationIDPattern.MatchString(session.OperationID) {
		return session, errors.New("remote installation session identity is invalid")
	}
	if session.LogID != "" && session.LogID != "usb-install-"+session.OperationID+".log" {
		return session, errors.New("remote installation session log identity is invalid")
	}
	if len(session.Events) > 4096 {
		return session, errors.New("remote installation session exceeds the event limit")
	}
	if session.Plan.OperationID != "" {
		if session.Plan.OperationID != session.OperationID {
			return session, errors.New("remote installation session plan identity differs")
		}
		if err := ValidateRemoteInstallPlan(session.Plan); err != nil {
			return session, err
		}
	}
	if session.Artifacts != nil {
		if session.Artifacts.OperationID != session.OperationID {
			return session, errors.New("remote installation session artifact identity differs")
		}
		if err := ValidateRemoteInstallArtifacts(*session.Artifacts); err != nil {
			return session, err
		}
	}
	if session.Preparation != nil {
		if session.Preparation.OperationID != session.OperationID {
			return session, errors.New("remote installation session preparation identity differs")
		}
		if err := ValidateRemoteInstallPreparation(*session.Preparation); err != nil {
			return session, err
		}
	}
	if session.Receipt != nil {
		if session.Receipt.OperationID != session.OperationID {
			return session, errors.New("remote installation session receipt identity differs")
		}
		receiptData, err := json.Marshal(session.Receipt)
		if err != nil {
			return session, errors.New("remote installation session receipt is invalid")
		}
		if _, err := DecodeRemoteInstallReceipt(receiptData); err != nil {
			return session, err
		}
	}
	if session.ReviewTokenDigest != "" && (!stringsHasHexTokenDigest(session.ReviewTokenDigest) || session.ReviewExpiresAt.IsZero()) {
		return session, errors.New("remote installation session review state is invalid")
	}
	if session.Bootstrap != nil {
		if !remoteHostPattern.MatchString(session.Bootstrap.Host) || validateRemoteIPv4(session.Bootstrap.Address) != nil ||
			!remotePublicKeyPattern.MatchString(session.Bootstrap.HostPublicKey) || !remoteFingerprintPattern.MatchString(session.Bootstrap.HostFingerprint) ||
			!strings.HasPrefix(session.Bootstrap.AuthorizedKeyLine, "restrict ") || !remotePublicKeyPattern.MatchString(strings.TrimPrefix(session.Bootstrap.AuthorizedKeyLine, "restrict ")) {
			return session, errors.New("remote installation bootstrap record is invalid")
		}
		if err := ValidateRemoteMachineFacts(session.Bootstrap.Facts); err != nil {
			return session, err
		}
	}
	return session, nil
}

func DecodeRemoteInstallResponse(data []byte) (RemoteInstallResponse, error) {
	var response RemoteInstallResponse
	if err := decodeRemoteJSON(data, RemoteInstallPlanMaxBytes, &response); err != nil {
		return response, fmt.Errorf("decode remote installation response: %w", err)
	}
	if response.SchemaVersion != RemoteInstallSchemaVersion || !remoteOperationIDPattern.MatchString(response.RequestID) {
		return response, errors.New("remote installation response identity is invalid")
	}
	if response.OperationID != "" && !remoteOperationIDPattern.MatchString(response.OperationID) {
		return response, errors.New("remote installation response operation ID is invalid")
	}
	if response.State == "" || len(response.State) > 64 || len(response.Message) > 4096 {
		return response, errors.New("remote installation response content is invalid")
	}
	if response.Session != nil && response.Session.OperationID != response.OperationID {
		return response, errors.New("remote installation response session identity differs")
	}
	if response.Session != nil {
		sessionData, err := json.Marshal(response.Session)
		if err != nil {
			return response, errors.New("remote installation response session is invalid")
		}
		if _, err := DecodeRemoteInstallSession(sessionData); err != nil {
			return response, err
		}
	}
	if response.Plan != nil {
		if response.Plan.OperationID != response.OperationID || response.Plan.Method != RemoteInstallUSBSSH ||
			(response.Plan.ReviewToken != "" && !remoteReviewTokenPattern.MatchString(response.Plan.ReviewToken)) {
			return response, errors.New("remote installation response plan is invalid")
		}
	}
	if response.Execution != nil && response.Execution.OperationID != response.OperationID {
		return response, errors.New("remote installation response execution identity differs")
	}
	return response, nil
}

func stringsHasHexTokenDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validRemoteInstallOperation(operation RemoteInstallOperation) bool {
	switch operation {
	case RemoteInstallBootstrapOperation, RemoteInstallPrepareOperation, RemoteInstallPlanOperation, RemoteInstallApplyOperation,
		RemoteInstallStatusOperation, RemoteInstallCancelOperation, RemoteInstallRebootOperation, RemoteInstallVerifyOperation,
		RemoteInstallCloseOperation, RemoteInstallReconcileOperation, RemoteInstallWorkerProbeOperation:
		return true
	default:
		return false
	}
}

func remoteOperationNeedsID(operation RemoteInstallOperation) bool {
	switch operation {
	case RemoteInstallStatusOperation, RemoteInstallCancelOperation, RemoteInstallRebootOperation,
		RemoteInstallVerifyOperation, RemoteInstallCloseOperation, RemoteInstallReconcileOperation, RemoteInstallPlanOperation,
		RemoteInstallApplyOperation:
		return true
	default:
		return false
	}
}
