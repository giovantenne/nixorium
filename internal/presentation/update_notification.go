package presentation

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type updateNotificationMsg struct {
	notice domain.UpdateNotification
	err    error
}
type updateNotificationTickMsg struct{}
type updateNotificationDismissedMsg struct {
	key string
	err error
}

func (model dashboardModel) checkUpdateNotification() tea.Cmd {
	if model.actions.ClassroomMode || model.actions.CheckUpdateNotification == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		notice, err := model.actions.CheckUpdateNotification(ctx)
		return updateNotificationMsg{notice: notice, err: err}
	}
}
func (model dashboardModel) finishUpdateNotification(msg updateNotificationMsg) (tea.Model, tea.Cmd) {
	if model.actions.ClassroomMode {
		return model, nil
	}
	if msg.err == nil {
		model.updateNotification = msg.notice
	}
	return model, tea.Tick(time.Hour, func(time.Time) tea.Msg { return updateNotificationTickMsg{} })
}
func (model dashboardModel) dismissUpdateNotification() (tea.Model, tea.Cmd) {
	notice := model.updateNotification
	if notice.Key == "" || model.actions.DismissUpdateNotification == nil {
		return model, nil
	}
	return model, func() tea.Msg {
		return updateNotificationDismissedMsg{key: notice.Key, err: model.actions.DismissUpdateNotification(notice)}
	}
}
