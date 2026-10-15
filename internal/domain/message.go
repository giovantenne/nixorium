package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// A message to the students: shown over their screens until it ends, or
// removed. Like a lock it changes only the students' sessions.

// Classroom operations for messages. Requests carry MessageAction and, to
// show one, MessageContent, or the reviewed MessagePlan; answers carry
// MessagePlan or MessageReport.
const (
	ClassroomMessagePlanOperation  ClassroomOperation = "message-plan"
	ClassroomMessageApplyOperation ClassroomOperation = "message-apply"
)

// messageOperations are the classroom operations of this file.
var messageOperations = map[ClassroomOperation]bool{
	ClassroomMessagePlanOperation:  true,
	ClassroomMessageApplyOperation: true,
}

// MessageAction shows or removes the message.
type MessageAction string

const (
	MessageShow MessageAction = "show"
	MessageHide MessageAction = "hide"
)

func (a MessageAction) Valid() bool { return a == MessageShow || a == MessageHide }

// MessageContent is what a message says and how long it stays: until the
// student closes it (Seconds 0), for Seconds, or until the computer is
// unlocked.
type MessageContent struct {
	Text        string `json:"text"`
	Seconds     int    `json:"seconds,omitempty"`
	UntilUnlock bool   `json:"untilUnlock,omitempty"`
}

// MessagePlan reviews showing or removing a message on the selected
// computers. Message holds the controller's own sentences, never the text.
type MessagePlan struct {
	SchemaVersion int               `json:"schemaVersion"`
	Operation     string            `json:"operation"`
	State         string            `json:"state"`
	Repository    string            `json:"repository"`
	Requested     string            `json:"requested"`
	Action        MessageAction     `json:"action"`
	Content       MessageContent    `json:"content"`
	Targets       []LockTarget      `json:"targets"`
	ExpiresAt     time.Time         `json:"expiresAt"`
	ReviewToken   string            `json:"reviewToken,omitempty"`
	Message       string            `json:"message"`
	Issues        []ValidationIssue `json:"issues"`
}

func (p MessagePlan) HasErrors() bool { return p.State != "ready" || len(p.Issues) > 0 }

// MessageReviewToken binds a review to its action, text, duration, targets
// and expiry.
func MessageReviewToken(p MessagePlan) string {
	p.ReviewToken = ""
	p.Message = ""
	p.Targets = append([]LockTarget(nil), p.Targets...)
	for i := range p.Targets {
		p.Targets[i].Detail = ""
	}
	content, _ := json.Marshal(p)
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// States of a computer in a message report.
const (
	MessageDone    = "done"
	MessageNotSent = "not-sent"
	MessageFailed  = "failed"
)

type MessageOutcome struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Detail string `json:"detail"`
}

type MessageReport struct {
	SchemaVersion int              `json:"schemaVersion"`
	Operation     string           `json:"operation"`
	State         string           `json:"state"`
	Action        MessageAction    `json:"action"`
	Targets       []MessageOutcome `json:"targets"`
	Message       string           `json:"message"`
}

func (r MessageReport) HasErrors() bool { return r.State != "completed" }
