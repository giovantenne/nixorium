package domain

import (
	"reflect"
	"strings"
	"testing"
)

func TestDiffLabSettingsIsOrderedAndRedactsPasswordHashes(t *testing.T) {
	before := validSettings()
	after := before
	after.Lab.NetworkBase = "10.1.0.0"
	after.Lab.TeacherPassword = "$6$new$privatehash"
	changes := DiffLabSettings(before, after)
	if len(changes) != 2 || changes[0].Field != "lab.networkBase" || changes[1].Field != "lab.teacherPassword" || !changes[1].Sensitive {
		t.Fatalf("changes = %+v", changes)
	}
	for _, change := range changes {
		if strings.Contains(change.Before.(string), "$6$") || strings.Contains(change.After.(string), "$6$") {
			t.Fatalf("change leaked password hash: %+v", change)
		}
	}
}

func TestSettingsFingerprintIsStableAndContentSensitive(t *testing.T) {
	first := SettingsFingerprint([]byte("one"))
	if first != SettingsFingerprint([]byte("one")) || first == SettingsFingerprint([]byte("two")) || !strings.HasPrefix(first, "sha256:") {
		t.Fatalf("unexpected fingerprints: %q", first)
	}
}

// A field DiffLabSettings does not list would be saved without a review and
// reported as no change; this fails when a LabSettings field is missing.
func TestDiffLabSettingsCoversEveryLabSettingsField(t *testing.T) {
	labType := reflect.TypeOf(LabSettings{})
	for index := 0; index < labType.NumField(); index++ {
		field := labType.Field(index)
		t.Run(field.Name, func(t *testing.T) {
			before := validSettings()
			after := before
			target := reflect.ValueOf(&after.Lab).Elem().Field(index)
			switch field.Type.Kind() {
			case reflect.String:
				target.SetString(target.String() + "-changed")
			case reflect.Int:
				target.SetInt(target.Int() + 7)
			case reflect.Bool:
				target.SetBool(!target.Bool())
			case reflect.Map:
				changed := reflect.MakeMap(field.Type)
				changed.SetMapIndex(reflect.ValueOf("pc01-changed"), reflect.ValueOf("enp9s0-changed"))
				target.Set(changed)
			case reflect.Pointer:
				switch field.Type.Elem().Kind() {
				case reflect.Bool:
					// nil means on; the opposite of the starting value is off.
					off := false
					target.Set(reflect.ValueOf(&off))
				default:
					target.Set(reflect.New(field.Type.Elem()))
					target.Elem().Field(0).SetString("changed")
				}
			default:
				t.Fatalf("test cannot change a %s field; teach it how, and add the field to DiffLabSettings", field.Type)
			}
			if reflect.DeepEqual(before.Lab, after.Lab) {
				t.Fatalf("the test did not change %s", field.Name)
			}
			if len(DiffLabSettings(before, after)) == 0 {
				t.Fatalf("DiffLabSettings does not report a change of %s; add it, or the field cannot be saved", field.Name)
			}
		})
	}
}
