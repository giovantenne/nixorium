package presentation

import (
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestMessageNoticeShowsVerifiedControllerApplyAsSuccess(t *testing.T) {
	model := dashboardModel{message: "The controller now uses the saved configuration."}
	model.controller.result = domain.ControllerRebuildExecutionReport{Operation: "controller-apply", Applied: true, Verified: true, Message: model.message}
	if model.messageNotice().kind != tuiStatusSuccess {
		t.Fatal("verified apply is not a success")
	}
	model.controller.result.Verified = false
	if model.messageNotice().kind != tuiStatusAttention {
		t.Fatal("unverified apply must stay an attention notice")
	}
}
