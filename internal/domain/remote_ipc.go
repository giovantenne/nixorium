package domain

import (
	"errors"
	"fmt"
)

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
	SchemaVersion int                    `json:"schemaVersion"`
	RequestID     string                 `json:"requestId"`
	Operation     RemoteInstallOperation `json:"operation"`
	OperationID   string                 `json:"operationId,omitempty"`
	Host          string                 `json:"host,omitempty"`
}

type RemoteInstallResponse struct {
	SchemaVersion int                   `json:"schemaVersion"`
	RequestID     string                `json:"requestId"`
	State         string                `json:"state"`
	OperationID   string                `json:"operationId,omitempty"`
	Message       string                `json:"message,omitempty"`
	Session       *RemoteInstallSession `json:"session,omitempty"`
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
	if session.Receipt != nil && session.Receipt.OperationID != session.OperationID {
		return session, errors.New("remote installation session receipt identity differs")
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
	return response, nil
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
	case RemoteInstallStatusOperation, RemoteInstallCancelOperation, RemoteInstallRebootOperation, RemoteInstallVerifyOperation,
		RemoteInstallCloseOperation, RemoteInstallReconcileOperation, RemoteInstallPlanOperation, RemoteInstallApplyOperation:
		return true
	default:
		return false
	}
}
