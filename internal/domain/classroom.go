package domain

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
)

const (
	ClassroomProtocolVersion = 1
	ClassroomMessageMaxBytes = 1024 * 1024
)

type ClassroomOperation string

const (
	ClassroomOverviewOperation      ClassroomOperation = "overview"
	ClassroomStatusOperation        ClassroomOperation = "status"
	ClassroomHostsOperation         ClassroomOperation = "hosts"
	ClassroomPowerPlanOperation     ClassroomOperation = "power-plan"
	ClassroomPowerApplyOperation    ClassroomOperation = "power-apply"
	ClassroomInternetPlanOperation  ClassroomOperation = "internet-plan"
	ClassroomInternetApplyOperation ClassroomOperation = "internet-apply"
	ClassroomLockPlanOperation      ClassroomOperation = "lock-plan"
	ClassroomLockApplyOperation     ClassroomOperation = "lock-apply"
	// Sending files to students' desktops: prepare a transfer, upload each
	// file in pieces, then review and apply like any classroom action.
	ClassroomShareBeginOperation ClassroomOperation = "share-begin"
	ClassroomShareChunkOperation ClassroomOperation = "share-chunk"
	ClassroomSharePlanOperation  ClassroomOperation = "share-plan"
	ClassroomShareApplyOperation ClassroomOperation = "share-apply"
	// The desktop helper of a classroom view page waits for a job and
	// reports the transfer it prepared from its user's desktop.
	ClassroomDesktopWaitOperation  ClassroomOperation = "desktop-wait"
	ClassroomDesktopReadyOperation ClassroomOperation = "desktop-ready"
	// ClassroomViewOpenOperation returns a one-time address of the
	// experimental classroom view page.
	ClassroomViewOpenOperation ClassroomOperation = "view-open"
)

func (o ClassroomOperation) Valid() bool {
	switch o {
	case ClassroomOverviewOperation, ClassroomStatusOperation, ClassroomHostsOperation,
		ClassroomPowerPlanOperation, ClassroomPowerApplyOperation,
		ClassroomInternetPlanOperation, ClassroomInternetApplyOperation,
		ClassroomLockPlanOperation, ClassroomLockApplyOperation,
		ClassroomShareBeginOperation, ClassroomShareChunkOperation,
		ClassroomSharePlanOperation, ClassroomShareApplyOperation,
		ClassroomDesktopWaitOperation, ClassroomDesktopReadyOperation,
		ClassroomViewOpenOperation:
		return true
	default:
		return false
	}
}

type ClassroomRequest struct {
	SchemaVersion  int                   `json:"schemaVersion"`
	RequestID      string                `json:"requestId"`
	Operation      ClassroomOperation    `json:"operation"`
	Requested      string                `json:"requested,omitempty"`
	PowerAction    ClientPowerAction     `json:"powerAction,omitempty"`
	SessionPolicy  ShutdownSessionPolicy `json:"sessionPolicy,omitempty"`
	InternetAction InternetAction        `json:"internetAction,omitempty"`
	PowerPlan      *ShutdownPlanReport   `json:"powerPlan,omitempty"`
	InternetPlan   *InternetPlan         `json:"internetPlan,omitempty"`
	LockAction     LockAction            `json:"lockAction,omitempty"`
	LockPlan       *LockPlan             `json:"lockPlan,omitempty"`
	ShareFiles     []ShareFile           `json:"shareFiles,omitempty"`
	ShareTransfer  string                `json:"shareTransfer,omitempty"`
	ShareIndex     int                   `json:"shareIndex,omitempty"`
	ShareOffset    int64                 `json:"shareOffset,omitempty"`
	ShareData      []byte                `json:"shareData,omitempty"`
	SharePlan      *SharePlan            `json:"sharePlan,omitempty"`
	DesktopJob     string                `json:"desktopJob,omitempty"`
	DesktopError   string                `json:"desktopError,omitempty"`
}

// ClassroomShareChunkBytes keeps one uploaded piece, encoded, well below the
// classroom message limit.
const ClassroomShareChunkBytes = 512 << 10

type ClassroomResponse struct {
	SchemaVersion  int                  `json:"schemaVersion"`
	RequestID      string               `json:"requestId"`
	State          string               `json:"state"`
	Message        string               `json:"message,omitempty"`
	Status         *StatusReport        `json:"status,omitempty"`
	Hosts          *HostsReport         `json:"hosts,omitempty"`
	PowerPlan      *ShutdownPlanReport  `json:"powerPlan,omitempty"`
	PowerReport    *ShutdownApplyReport `json:"powerReport,omitempty"`
	ViewURL        string               `json:"viewUrl,omitempty"`
	InternetPlan   *InternetPlan        `json:"internetPlan,omitempty"`
	InternetReport *InternetReport      `json:"internetReport,omitempty"`
	LockPlan       *LockPlan            `json:"lockPlan,omitempty"`
	LockReport     *LockReport          `json:"lockReport,omitempty"`
	ShareTransfer  string               `json:"shareTransfer,omitempty"`
	SharePlan      *SharePlan           `json:"sharePlan,omitempty"`
	ShareReport    *ShareReport         `json:"shareReport,omitempty"`
	DesktopJob     string               `json:"desktopJob,omitempty"`
}

func NewClassroomRequest(operation ClassroomOperation) (ClassroomRequest, error) {
	identifier := make([]byte, 16)
	if _, err := rand.Read(identifier); err != nil {
		return ClassroomRequest{}, err
	}
	return ClassroomRequest{
		SchemaVersion: ClassroomProtocolVersion,
		RequestID:     hex.EncodeToString(identifier),
		Operation:     operation,
	}, nil
}

func DecodeClassroomRequest(content []byte) (ClassroomRequest, error) {
	var request ClassroomRequest
	if err := decodeClassroomMessage(content, &request); err != nil {
		return request, err
	}
	if request.SchemaVersion != ClassroomProtocolVersion || !validClassroomRequestID(request.RequestID) || !request.Operation.Valid() {
		return ClassroomRequest{}, errors.New("invalid classroom request identity")
	}
	return request, nil
}

func DecodeClassroomResponse(content []byte) (ClassroomResponse, error) {
	var response ClassroomResponse
	if err := decodeClassroomMessage(content, &response); err != nil {
		return response, err
	}
	if response.SchemaVersion != ClassroomProtocolVersion || !validClassroomRequestID(response.RequestID) {
		return ClassroomResponse{}, errors.New("invalid classroom response identity")
	}
	return response, nil
}

func validClassroomRequestID(value string) bool {
	if len(value) != 32 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func decodeClassroomMessage(content []byte, value any) error {
	if len(content) == 0 || len(content) > ClassroomMessageMaxBytes {
		return errors.New("classroom message has an invalid size")
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("classroom message contains trailing data")
	}
	return nil
}
