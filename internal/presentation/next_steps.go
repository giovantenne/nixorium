package presentation

import (
	"reflect"

	"github.com/giovantenne/nixorium/internal/domain"
)

var validationIssueType = reflect.TypeOf(domain.ValidationIssue{})

// withNextSteps returns a copy of a report whose validation issues carry the
// recognized next steps. The caller's value is never modified.
func withNextSteps(value any) any {
	if value == nil {
		return value
	}
	original := reflect.ValueOf(value)
	if original.Kind() != reflect.Struct {
		return value
	}
	copied := reflect.New(original.Type()).Elem()
	copied.Set(original)
	attachNextSteps(copied, 0)
	return copied.Interface()
}

func attachNextSteps(value reflect.Value, depth int) {
	if depth > 4 {
		return
	}
	switch value.Kind() {
	case reflect.Struct:
		for index := 0; index < value.NumField(); index++ {
			field := value.Field(index)
			if field.CanSet() {
				attachNextSteps(field, depth+1)
			}
		}
	case reflect.Slice:
		if value.IsNil() {
			return
		}
		if value.Type().Elem() == validationIssueType {
			issues := make([]domain.ValidationIssue, value.Len())
			reflect.Copy(reflect.ValueOf(issues), value)
			value.Set(reflect.ValueOf(domain.WithNextSteps(issues)))
		}
	case reflect.Pointer:
		if !value.IsNil() && value.Elem().Kind() == reflect.Struct {
			copied := reflect.New(value.Elem().Type())
			copied.Elem().Set(value.Elem())
			attachNextSteps(copied.Elem(), depth+1)
			value.Set(copied)
		}
	}
}
