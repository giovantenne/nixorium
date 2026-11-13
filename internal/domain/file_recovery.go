package domain

import (
	"errors"
	"path"
	"strings"
	"time"
	"unicode/utf8"
)

// Recovering a student's files from the read-only snapshots a student
// computer keeps of past sessions. The teacher or the administrator chooses
// one computer, one saved session and the files or folders; the computer
// copies them into a "Recovered files" folder on the student's desktop,
// written as the student and never replacing anything.

// Classroom operations for recovering files. Requests carry Requested (the
// computer), FileRecoverySession and FileRecoveryPath or FileRecoveryPaths,
// or the reviewed FileRecoveryPlan; answers carry FileRecoverySessions,
// FileRecoveryListing, FileRecoveryPlan or FileRecoveryReport.
const (
	ClassroomFileRecoverySessionsOperation ClassroomOperation = "file-recovery-sessions"
	ClassroomFileRecoveryListOperation     ClassroomOperation = "file-recovery-list"
	ClassroomFileRecoveryPlanOperation     ClassroomOperation = "file-recovery-plan"
	ClassroomFileRecoveryApplyOperation    ClassroomOperation = "file-recovery-apply"
)

// fileRecoveryOperations are the classroom operations of this file.
var fileRecoveryOperations = map[ClassroomOperation]bool{
	ClassroomFileRecoverySessionsOperation: true,
	ClassroomFileRecoveryListOperation:     true,
	ClassroomFileRecoveryPlanOperation:     true,
	ClassroomFileRecoveryApplyOperation:    true,
}

// Limits of the client helper's answers and of one recovery.
const (
	FileRecoveryProtocolVersion = 1
	// FileRecoveryMaxListEntries bounds one folder listing.
	FileRecoveryMaxListEntries = 1000
	// FileRecoveryMaxPaths bounds the files and folders chosen for one recovery.
	FileRecoveryMaxPaths = 100
	// FileRecoveryMaxAnswerBytes bounds the helper's JSON answer.
	FileRecoveryMaxAnswerBytes = 512 * 1024
	recoveryMaxPathBytes       = 4096
	recoveryMaxElements        = 32
	recoveryMaxNameBytes       = 255
)

// FileRecoveryFolder is the folder created on the student's desktop.
const FileRecoveryFolder = "Recovered files"

// Operations of the client helper, read from its standard input.
const (
	FileRecoveryHelperSessions = "sessions"
	FileRecoveryHelperList     = "list"
	FileRecoveryHelperRestore  = "restore"
)

// FileRecoveryHelperRequest is the one request the controller writes to the
// client helper's standard input. Paths are relative to the student's home
// as it was saved in the session.
type FileRecoveryHelperRequest struct {
	SchemaVersion int      `json:"schemaVersion"`
	Operation     string   `json:"operation"`
	Session       int      `json:"session,omitempty"`
	Path          string   `json:"path,omitempty"`
	Paths         []string `json:"paths,omitempty"`
}

// FileRecoverySession is one saved session: Number 1 is the most recent.
// SavedAt is when the computer started and saved the session before it.
type FileRecoverySession struct {
	Number  int       `json:"number"`
	SavedAt time.Time `json:"savedAt"`
}

// FileRecoveryEntry is a file or folder of a listed folder.
type FileRecoveryEntry struct {
	Name     string    `json:"name"`
	Dir      bool      `json:"dir,omitempty"`
	Size     int64     `json:"size,omitempty"`
	Modified time.Time `json:"modified"`
}

// FileRecoveryListing is one folder of a saved session. Hidden names, links and
// special files are left out; Truncated says that more entries exist.
type FileRecoveryListing struct {
	Session   int                 `json:"session"`
	Path      string              `json:"path"`
	Entries   []FileRecoveryEntry `json:"entries"`
	Truncated bool                `json:"truncated,omitempty"`
}

// FileRecoveryRestored is what a recovery wrote on the student's desktop.
type FileRecoveryRestored struct {
	Folder string   `json:"folder"`
	Placed []string `json:"placed"`
	Files  int      `json:"files"`
	Bytes  int64    `json:"bytes"`
}

// FileRecoveryHelperAnswer is the client helper's one answer. Exactly one of
// the results is set when Error is empty. Error is a sentence for the
// teacher; Code names the situation.
type FileRecoveryHelperAnswer struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Sessions      []FileRecoverySession `json:"sessions,omitempty"`
	Listing       *FileRecoveryListing  `json:"listing,omitempty"`
	Restored      *FileRecoveryRestored `json:"restored,omitempty"`
	Error         string                `json:"error,omitempty"`
	Code          string                `json:"code,omitempty"`
}

// Codes of the helper's refusals.
const (
	FileRecoveryCodeNoSession = "RECOVERY-NO-SESSION"
	FileRecoveryCodeNotFound  = "RECOVERY-NOT-FOUND"
	FileRecoveryCodeTooLarge  = "RECOVERY-TOO-LARGE"
	FileRecoveryCodeInvalid   = "RECOVERY-INVALID"
	FileRecoveryCodeFailed    = "RECOVERY-FAILED"
)

// ValidFileRecoveryPath accepts "" (the home itself) or a plain relative path
// below it: no leading slash, no "." or ".." elements, no empty elements,
// no backslashes or control characters, within length limits.
func ValidFileRecoveryPath(name string) error {
	if name == "" {
		return nil
	}
	invalid := errors.New("the path is not a plain relative path")
	if len(name) > recoveryMaxPathBytes || !utf8.ValidString(name) || path.Clean(name) != name || path.IsAbs(name) || strings.ContainsRune(name, '\\') {
		return invalid
	}
	elements := strings.Split(name, "/")
	if len(elements) > recoveryMaxElements {
		return errors.New("folders are nested too deeply")
	}
	for _, element := range elements {
		if element == "" || element == "." || element == ".." || len(element) > recoveryMaxNameBytes {
			return invalid
		}
		for _, r := range element {
			if r < 0x20 || r == 0x7f {
				return invalid
			}
		}
	}
	return nil
}

// ValidFileRecoveryPaths checks the files and folders chosen for one recovery:
// at least one, at most FileRecoveryMaxPaths, each a valid non-empty path, none
// repeated and none inside another chosen folder.
func ValidFileRecoveryPaths(paths []string) error {
	if len(paths) == 0 {
		return errors.New("choose at least one file or folder")
	}
	if len(paths) > FileRecoveryMaxPaths {
		return errors.New("too many files and folders are chosen")
	}
	seen := map[string]bool{}
	for _, name := range paths {
		if name == "" {
			return errors.New("the whole home cannot be recovered at once; choose its files or folders")
		}
		if err := ValidFileRecoveryPath(name); err != nil {
			return err
		}
		if seen[name] {
			return errors.New("a file or folder is chosen twice")
		}
		seen[name] = true
	}
	for _, name := range paths {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if seen[parent] {
				return errors.New("a chosen file is inside a chosen folder")
			}
		}
	}
	return nil
}

// FileRecoveryTarget is the computer of a recovery review.
type FileRecoveryTarget struct {
	HostMeta
}

// FileRecoveryPlan reviews copying chosen files of one saved session back to
// the student's desktop. The review token binds the computer, the session,
// its saving time and the paths; the computer reads them again when the
// review is applied.
type FileRecoveryPlan struct {
	SchemaVersion int                `json:"schemaVersion"`
	Operation     string             `json:"operation"`
	State         string             `json:"state"`
	Repository    string             `json:"repository"`
	Requested     string             `json:"requested"`
	Target        FileRecoveryTarget `json:"target"`
	Session       int                `json:"session"`
	SavedAt       time.Time          `json:"savedAt"`
	Paths         []string           `json:"paths"`
	ExpiresAt     time.Time          `json:"expiresAt"`
	ReviewToken   string             `json:"reviewToken,omitempty"`
	Message       string             `json:"message"`
	Issues        []ValidationIssue  `json:"issues"`
}

func (p FileRecoveryPlan) HasErrors() bool { return p.State != "ready" || len(p.Issues) > 0 }

// FileRecoveryReviewToken binds a review to its computer, session, paths and
// expiry.
func FileRecoveryReviewToken(p FileRecoveryPlan) string {
	p.ReviewToken = ""
	p.Message = ""
	p.Paths = append([]string(nil), p.Paths...)
	return reviewTokenDigest(p)
}

// FileRecoveryReport is the outcome of a recovery.
type FileRecoveryReport struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Operation     string                `json:"operation"`
	State         string                `json:"state"`
	Computer      string                `json:"computer"`
	Restored      *FileRecoveryRestored `json:"restored,omitempty"`
	Message       string                `json:"message"`
}

func (r FileRecoveryReport) HasErrors() bool { return r.State != "completed" }
