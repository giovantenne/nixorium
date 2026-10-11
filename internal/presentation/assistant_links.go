package presentation

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

var assistantURLPattern = regexp.MustCompile(`https?://[^\s<>"'\x00-\x20\x7f-\x9f]+`)

// Linkify rendered text before wrapping; ANSI sequences have no visible width.
// Existing terminal styles and links are preserved, including wrapped OSC 8 links.
func assistantLinkify(text string) string {
	var result strings.Builder
	linked := false
	for len(text) > 0 {
		if text[0] == '\x1b' {
			sequence, _, n, _ := ansi.DecodeSequence(text, 0, nil)
			if n == 0 {
				n, sequence = 1, text[:1]
			}
			if strings.HasPrefix(sequence, "\x1b]8;") {
				payload := strings.TrimSuffix(strings.TrimSuffix(sequence[4:], "\x1b\\"), "\x07")
				if _, url, ok := strings.Cut(payload, ";"); ok {
					linked = url != ""
				}
			}
			result.WriteString(sequence)
			text = text[n:]
			continue
		}
		end := strings.IndexByte(text, '\x1b')
		if end < 0 {
			end = len(text)
		}
		plain := text[:end]
		if !linked {
			plain = assistantURLPattern.ReplaceAllStringFunc(plain, func(match string) string {
				url := strings.TrimRight(match, ".,;!?")
				for _, pair := range [][2]string{{"(", ")"}, {"[", "]"}, {"{", "}"}} {
					for strings.HasSuffix(url, pair[1]) && strings.Count(url, pair[1]) > strings.Count(url, pair[0]) {
						url = strings.TrimSuffix(url, pair[1])
					}
				}
				return "\x1b]8;;" + url + "\x1b\\" + url + "\x1b]8;;\x1b\\" + match[len(url):]
			})
		}
		result.WriteString(plain)
		text = text[end:]
	}
	return result.String()
}
