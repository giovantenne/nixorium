package classroomview

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Showing a message on a student's screen: message.show carries Text and how
// long it stays (Seconds, or UntilUnlock) and is answered by message.shown;
// message.hide removes it and is answered by message.hidden. A computer has
// at most one message: a new one replaces the previous. Text is plain text,
// never markup, and the agent bounds it again whatever the controller sent.
const (
	TypeMessageShow   = "message.show"
	TypeMessageShown  = "message.shown"
	TypeMessageHide   = "message.hide"
	TypeMessageHidden = "message.hidden"
	// CodeMessageUnavailable: the session has no classroom extension that
	// can show messages (it is not running, or it predates messages).
	CodeMessageUnavailable = "message-unavailable"
)

// A short notice tells the student what the teacher just did: notice.show
// carries Notice, one of the kinds below (never text, so the wording stays in
// the student's own extension), and is answered by notice.shown. A notice
// disappears by itself after a few seconds. Agents that predate notices
// answer CodeUnsupported.
const (
	TypeNotice      = "notice.show"
	TypeNoticeShown = "notice.shown"
	// NoticeScreenshot: the teacher saved a picture of the screen.
	NoticeScreenshot = "screenshot"
	// NoticeCollect: the teacher collected the files on the desktop.
	NoticeCollect = "collect"
)

// ValidNotice reports whether kind is a notice the agent can show.
func ValidNotice(kind string) bool {
	return kind == NoticeScreenshot || kind == NoticeCollect
}

// Limits of a message and its duration. A message stays until the student
// closes it when Seconds is 0 (the Close button appears after
// MessageCloseSeconds), disappears after Seconds, or, with UntilUnlock,
// when the computer is unlocked.
const (
	MaxMessageRunes   = 300
	MaxMessageLines   = 10
	MaxMessageSeconds = 600
	// MessageCloseSeconds is how long the extension waits before it offers
	// Close on a message that stays until the student closes it.
	MessageCloseSeconds = 10
)

// Messenger shows messages on the screen of a session's user. Sessions of
// agents that predate messages answer CodeUnsupported.
type Messenger interface {
	ShowMessage(text string, seconds int, untilUnlock bool) error
	HideMessage() error
}

// TrimMessage removes the blank space around a message the teacher typed.
func TrimMessage(text string) string {
	return strings.TrimSpace(text)
}

// ValidateMessage refuses a message the extension must not show. The
// wording is for the teacher.
func ValidateMessage(text string, seconds int, untilUnlock bool) error {
	switch {
	case strings.TrimSpace(text) == "":
		return errors.New("Write the message first.")
	case !utf8.ValidString(text):
		return errors.New("The message contains characters that cannot be shown.")
	case utf8.RuneCountInString(text) > MaxMessageRunes:
		return errors.New("The message is longer than 300 characters.")
	case strings.Count(text, "\n")+1 > MaxMessageLines:
		return errors.New("The message has more than 10 lines.")
	}
	for _, character := range text {
		// Lines are the only layout: no tabs, returns or other control
		// characters, and no Unicode line or paragraph separators.
		if character != '\n' && (unicode.IsControl(character) || character == '\u2028' || character == '\u2029') {
			return errors.New("The message cannot contain tabs or other control characters.")
		}
	}
	if seconds < 0 || seconds > MaxMessageSeconds {
		return errors.New("The duration of the message is out of range.")
	}
	if untilUnlock && seconds != 0 {
		return errors.New("Choose one duration for the message.")
	}
	return nil
}
