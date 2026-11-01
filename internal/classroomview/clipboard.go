package classroomview

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// MaxClipboardBytes bounds the text shared in either direction at once.
const MaxClipboardBytes = 64 << 10

// ErrClipboardTooLarge means the text is over MaxClipboardBytes.
var ErrClipboardTooLarge = errors.New("the text is too long to share")

// ValidateClipboardText returns the text to share: plain UTF-8 text without
// NUL characters, at most MaxClipboardBytes long. Nothing here ever logs it.
func ValidateClipboardText(text string) (string, error) {
	if len(text) > MaxClipboardBytes {
		return "", ErrClipboardTooLarge
	}
	if !utf8.ValidString(text) {
		return "", errors.New("the text is not valid UTF-8")
	}
	text = strings.ReplaceAll(text, "\x00", "")
	if len(text) > MaxClipboardBytes {
		return "", ErrClipboardTooLarge
	}
	return text, nil
}
