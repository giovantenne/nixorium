package classroomview

import (
	"bytes"
	"strings"
	"testing"
)

func TestMessageShowAndHideRoundTrip(t *testing.T) {
	var buffer bytes.Buffer
	for _, message := range []Message{
		{Type: TypeMessageShow, Text: "Exam in progress.\nWait for the teacher.", Seconds: 30},
		{Type: TypeMessageShow, Text: "Back soon", UntilUnlock: true},
		{Type: TypeMessageHide},
	} {
		if err := Write(&buffer, message); err != nil {
			t.Fatal(err)
		}
		got, err := Read(&buffer)
		if err != nil || got.Type != message.Type || got.Text != message.Text || got.Seconds != message.Seconds || got.UntilUnlock != message.UntilUnlock {
			t.Fatalf("round trip of %+v = %+v, %v", message, got, err)
		}
	}
}

func TestValidateMessageAcceptsTheLimits(t *testing.T) {
	for name, test := range map[string]struct {
		text        string
		seconds     int
		untilUnlock bool
	}{
		"one line":            {"Exam in progress. Wait for the teacher.", 0, false},
		"300 characters":      {strings.Repeat("a", MaxMessageRunes), 0, false},
		"300 wide letters":    {strings.Repeat("é", MaxMessageRunes), 0, false},
		"ten lines":           {strings.Repeat("line\n", MaxMessageLines-1) + "line", 30, false},
		"markup is just text": {"<b>Exam</b> &amp; <script>x</script>", 120, false},
		"until unlocked":      {"Wait", 0, true},
		"longest duration":    {"Wait", MaxMessageSeconds, false},
	} {
		if err := ValidateMessage(test.text, test.seconds, test.untilUnlock); err != nil {
			t.Errorf("%s was refused: %v", name, err)
		}
	}
}

func TestValidateMessageRefusesWhatTheExtensionMustNotShow(t *testing.T) {
	for name, test := range map[string]struct {
		text        string
		seconds     int
		untilUnlock bool
		want        string
	}{
		"empty":               {"", 0, false, "Write the message"},
		"only blank space":    {" \n  ", 0, false, "Write the message"},
		"301 characters":      {strings.Repeat("a", MaxMessageRunes+1), 0, false, "300 characters"},
		"301 wide letters":    {strings.Repeat("é", MaxMessageRunes+1), 0, false, "300 characters"},
		"eleven lines":        {strings.Repeat("line\n", MaxMessageLines) + "line", 0, false, "10 lines"},
		"tab":                 {"a\tb", 0, false, "control characters"},
		"carriage return":     {"a\r\nb", 0, false, "control characters"},
		"escape":              {"\x1b[31mred", 0, false, "control characters"},
		"null":                {"a\x00b", 0, false, "control characters"},
		"delete":              {"a\x7fb", 0, false, "control characters"},
		"next line":           {"a\u0085b", 0, false, "control characters"},
		"line separator":      {"a\u2028b", 0, false, "control characters"},
		"paragraph separator": {"a\u2029b", 0, false, "control characters"},
		"invalid UTF-8":       {"a\xffb", 0, false, "cannot be shown"},
		"negative duration":   {"Wait", -1, false, "duration"},
		"too long a time":     {"Wait", MaxMessageSeconds + 1, false, "duration"},
		"two durations":       {"Wait", 30, true, "one duration"},
	} {
		err := ValidateMessage(test.text, test.seconds, test.untilUnlock)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("%s: error = %v, want %q", name, err, test.want)
		}
	}
}

func TestTrimMessageRemovesBlankSpaceAroundIt(t *testing.T) {
	if got := TrimMessage("\n  Exam in progress.\nWait.  \n\n"); got != "Exam in progress.\nWait." {
		t.Fatalf("trimmed = %q", got)
	}
}
