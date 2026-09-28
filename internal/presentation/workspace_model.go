package presentation

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type workspaceStage int

const (
	workspaceOverview workspaceStage = iota
	workspaceFieldList
	workspaceFieldEdit
	workspaceReview
	workspaceResult
)

type workspaceModel struct {
	stage                         workspaceStage
	loaded                        domain.WorkspacePlanReport
	candidate                     domain.WorkspaceProfile
	plan                          domain.WorkspacePlanReport
	result                        domain.WorkspaceApplyReport
	group, cursor, choice, scroll int
	field                         workspaceField
	choices                       []string
	selected                      []string
	inherit                       bool
	number                        string
	confirmation                  string
	requestID                     uint64
	cancel                        context.CancelFunc
	saving                        bool
}

func (model *workspaceModel) cancelRead() {
	if model.cancel != nil {
		model.cancel()
		model.cancel = nil
	}
	model.requestID++
}

func (model workspaceModel) textEntry() bool {
	return model.stage == workspaceReview || (model.stage == workspaceFieldEdit && model.field.kind == "number")
}

func (model *workspaceModel) startField() {
	model.field = workspaceGroupFields(model.group)[model.cursor]
	model.choice = 0
	model.choices = nil
	model.selected = []string{}
	model.number = ""
	value := workspaceValue(model.candidate, model.field)
	model.inherit = value == nil
	if model.field.kind == "number" {
		if value != nil {
			model.number = fmt.Sprint(value)
		}
	} else if model.field.kind == "favorites" || model.field.kind == "extensions" {
		if value == nil {
			value = workspaceValue(model.loaded.Inspection.Resolution.Catalog.Baseline, model.field)
		}
		if entries, ok := value.([]any); ok {
			for _, entry := range entries {
				model.selected = append(model.selected, fmt.Sprint(entry))
			}
		}
		if model.field.kind == "favorites" {
			for _, item := range model.loaded.Inspection.Resolution.Catalog.Applications {
				model.choices = append(model.choices, item.ID)
			}
		} else {
			for _, item := range model.loaded.Inspection.Resolution.Catalog.Extensions {
				model.choices = append(model.choices, item.ID)
			}
		}
	} else {
		model.choices = []string{"Inherit"}
		switch model.field.kind {
		case "boolean":
			model.choices = append(model.choices, "true", "false")
		case "browser":
			for _, item := range model.loaded.Inspection.Resolution.Catalog.Applications {
				if item.Browser {
					model.choices = append(model.choices, item.ID)
				}
			}
		default:
			model.choices = append(model.choices, model.field.choices...)
		}
		if value != nil {
			if index := slices.Index(model.choices, fmt.Sprint(value)); index >= 0 {
				model.choice = index
			}
		}
	}
	model.stage = workspaceFieldEdit
}

func (model *workspaceModel) acceptField() error {
	var value any
	switch model.field.kind {
	case "number":
		if model.number != "" {
			number, err := strconv.Atoi(model.number)
			if err != nil {
				return errors.New("enter a whole number, or leave empty to inherit")
			}
			value = number
		}
	case "favorites", "extensions":
		if !model.inherit {
			value = append([]string{}, model.selected...)
		}
	default:
		if model.choice > 0 {
			value = model.choices[model.choice]
			if model.field.kind == "boolean" {
				value = model.choices[model.choice] == "true"
			}
		}
	}
	updated, err := workspaceSetValue(model.candidate, model.field, value)
	if err != nil {
		return err
	}
	model.candidate = updated
	model.plan = domain.WorkspacePlanReport{}
	model.stage = workspaceFieldList
	return nil
}

func (model *workspaceModel) editField(key tea.KeyPressMsg) string {
	if key.String() == "esc" {
		model.stage = workspaceFieldList
		return "Field edit cancelled; the draft is unchanged."
	}
	if key.String() == "enter" {
		if err := model.acceptField(); err != nil {
			return err.Error()
		}
		return "Draft updated; nothing has been saved."
	}
	if model.field.kind == "number" {
		switch key.String() {
		case "backspace":
			if len(model.number) > 0 {
				model.number = model.number[:len(model.number)-1]
			}
		default:
			for _, r := range key.Text {
				if r >= '0' && r <= '9' && len(model.number) < 3 {
					model.number += string(r)
				}
			}
		}
		return ""
	}
	switch key.String() {
	case "up", "k":
		model.choice = max(0, model.choice-1)
	case "down", "j":
		model.choice = min(max(0, len(model.choices)-1), model.choice+1)
	}
	if model.field.kind != "favorites" && model.field.kind != "extensions" {
		return ""
	}
	switch key.String() {
	case "i":
		model.inherit = true
		model.selected = []string{}
		if entries, ok := workspaceValue(model.loaded.Inspection.Resolution.Catalog.Baseline, model.field).([]any); ok {
			for _, entry := range entries {
				model.selected = append(model.selected, fmt.Sprint(entry))
			}
		}
	case "c":
		model.inherit = false
		model.selected = []string{}
	case "space":
		if len(model.choices) == 0 {
			return "The deployment catalog has no entries for this field."
		}
		model.inherit = false
		id := model.choices[model.choice]
		if index := slices.Index(model.selected, id); index >= 0 {
			model.selected = slices.Delete(model.selected, index, index+1)
		} else {
			model.selected = append(model.selected, id)
		}
	case "shift+up", "shift+down":
		if model.field.kind != "favorites" || len(model.choices) == 0 || model.inherit {
			return ""
		}
		index := slices.Index(model.selected, model.choices[model.choice])
		if index < 0 {
			return "Select the favorite before changing its order."
		}
		next := index - 1
		if key.String() == "shift+down" {
			next = index + 1
		}
		if next >= 0 && next < len(model.selected) {
			model.selected[index], model.selected[next] = model.selected[next], model.selected[index]
		}
	}
	return ""
}

func workspaceIssueText(plan domain.WorkspacePlanReport) string {
	items := []string{plan.Message}
	for _, issue := range plan.Issues {
		items = append(items, issue.Message)
	}
	message := strings.TrimSpace(safeWorkspaceText(strings.Join(items, " ")))
	if message == "" {
		return "The workspace review returned incomplete metadata; no file changed."
	}
	return message
}
