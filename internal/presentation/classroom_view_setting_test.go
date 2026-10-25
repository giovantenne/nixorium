package presentation

import (
	"context"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestClassroomViewSettingRoundTripsAndStaysOutOfTheFileWhenOn(t *testing.T) {
	settings := wizardSettings()
	if got := settingFieldValue(settings, "lab.classroomView"); got != "on" {
		t.Fatalf("default value = %q", got)
	}
	off, err := setSettingField(settings, "lab.classroomView", "off")
	if err != nil || off.Lab.ClassroomView == nil || *off.Lab.ClassroomView || settingFieldValue(off, "lab.classroomView") != "off" {
		t.Fatalf("off = %+v, %v", off.Lab.ClassroomView, err)
	}
	data, err := domain.MarshalLabSettings(off)
	if err != nil || !strings.Contains(string(data), `"classroomView": false`) {
		t.Fatalf("saved off: %s, %v", data, err)
	}
	back, err := setSettingField(off, "lab.classroomView", "on")
	if err != nil || back.Lab.ClassroomView != nil || settingFieldValue(back, "lab.classroomView") != "on" {
		t.Fatalf("on = %+v, %v", back.Lab.ClassroomView, err)
	}
	data, err = domain.MarshalLabSettings(back)
	if err != nil || strings.Contains(string(data), "classroomView") {
		t.Fatalf("saved on still has the key: %s, %v", data, err)
	}
	if _, err := setSettingField(settings, "lab.classroomView", "maybe"); err == nil {
		t.Fatal("an unknown value was accepted")
	}
}

func TestClassroomViewFieldOffersOnlyOnAndOff(t *testing.T) {
	var group routineSettingsGroup
	for _, candidate := range routineSettingsGroups {
		if candidate.id == "classroom" {
			group = candidate
		}
	}
	if len(group.fields) != 1 || settingsHelp[group.fields[0].id].description == "" {
		t.Fatalf("group = %+v", group)
	}
	editor := newSettingsEditorModel(wizardSettings(), group.fields, "Classroom view")
	if got := len(editor.selector.Items()); got != 2 {
		t.Fatalf("choices = %d, want only On and Off", got)
	}
	if view := editor.View().Content; strings.Contains(view, "Custom") {
		t.Fatalf("a closed field offers a typed value:\n%s", view)
	}
}

func TestClassroomViewReviewShowsOnOffAndImpact(t *testing.T) {
	base := wizardSettings()
	candidate, _ := setSettingField(base, "lab.classroomView", "off")
	changes := domain.DiffLabSettings(base, candidate)
	model := dashboardModel{screen: dashboardSettingsReview, width: 140, height: 50,
		settings: settingsModel{plan: domain.ConfigPlanReport{State: "valid", Changes: changes, Impacts: domain.SettingsImpacts(base, candidate, changes)}},
	}
	view := model.View().Content
	for _, want := range []string{"Classroom view: on → off", "Internet pause", "dock icon", "Update computers"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in review:\n%s", want, view)
		}
	}
}

func TestComputersMenuHidesTheClassroomViewWhenItIsOff(t *testing.T) {
	model := dashboardModel{}
	model.actions.ClassroomMode = true
	found := map[string]bool{}
	for _, task := range model.availableComputerTasks() {
		found[task.id] = true
	}
	for _, id := range []string{"view", "lock", "share"} {
		if found[id] {
			t.Fatalf("the menu offers %q with the classroom view off", id)
		}
	}
	if len(found) == 0 {
		t.Fatal("the remaining functions are missing")
	}
	model.actions.OpenClassroomView = func(context.Context) (string, error) { return "", nil }
	if tasks := model.availableComputerTasks(); tasks[0].id != "view" {
		t.Fatalf("tasks = %+v", tasks)
	}
}
