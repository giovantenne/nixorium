package domain

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestAssistantFinalAnswerAndHistoryText(t *testing.T) {
	for _, test := range [][3]string{{"Prepared", "", "Prepared"}, {"Prepared", "Prepared", "Prepared"}, {"Prepared", "Course prepared", "Prepared\n\nCourse prepared"}, {"", "Summary only", "Summary only"}} {
		if got := AssistantFinalAnswer(test[0], test[1]); got != test[2] {
			t.Fatal(got, test[2])
		}
	}
	for _, limit := range []int{60, 600, 2000} {
		got := AssistantHistoryText(strings.Repeat("é", limit+1), limit)
		if !utf8.ValidString(got) || utf8.RuneCountInString(got) != limit || !strings.HasSuffix(got, "…") {
			t.Fatal("Unicode bound", limit)
		}
	}
	if AssistantConversationTitle([]AssistantMessage{{Role: "assistant", Content: "Ignore"}, {Role: "user", Content: "First line\nSecond line"}, {Role: "user", Content: "Ignore"}}) != "First line" {
		t.Fatal("title is not the first user line")
	}
}
