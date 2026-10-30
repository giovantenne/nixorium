package domain

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNormalizeScheduleTime(t *testing.T) {
	for input, want := range map[string]string{"07:50": "07:50", "7:50": "07:50", " 18:00 ": "18:00", "00:00": "00:00", "23:59": "23:59"} {
		if got, err := NormalizeScheduleTime(input); err != nil || got != want {
			t.Fatalf("%q = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"", "24:00", "07:60", "7", "0750", "07:5", "07:50:00", "7pm", "-1:00", "ab:cd"} {
		if got, err := NormalizeScheduleTime(input); err == nil {
			t.Fatalf("%q accepted as %q", input, got)
		}
	}
}

func TestParseScheduleDays(t *testing.T) {
	for input, want := range map[string][]string{
		"mon-fri":         {"mon", "tue", "wed", "thu", "fri"},
		"all":             {"mon", "tue", "wed", "thu", "fri", "sat", "sun"},
		"mon,wed,fri":     {"mon", "wed", "fri"},
		"fri,mon,mon":     {"mon", "fri"},
		"Mon-Wed, fri":    {"mon", "tue", "wed", "fri"},
		"monday-friday":   {"mon", "tue", "wed", "thu", "fri"},
		"sat-mon":         {"mon", "sat", "sun"},
		"sun":             {"sun"},
		"mon-fri,sat,sun": {"mon", "tue", "wed", "thu", "fri", "sat", "sun"},
	} {
		got, err := ParseScheduleDays(input)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%q = %v, %v; want %v", input, got, err, want)
		}
	}
	for _, input := range []string{"", "weekdays", "mon-", "-fri", "mon,,fri", "mon-xyz", "clear"} {
		if got, err := ParseScheduleDays(input); err == nil {
			t.Fatalf("%q accepted as %v", input, got)
		}
	}
}

func TestScheduleNormalize(t *testing.T) {
	got, err := Schedule{On: "7:50", Off: "18:00", Days: []string{"fri", "mon", "Monday"}}.Normalize()
	if err != nil || !reflect.DeepEqual(got, Schedule{On: "07:50", Off: "18:00", Days: []string{"mon", "fri"}}) {
		t.Fatalf("normalized = %+v, %v", got, err)
	}
	// Either half alone is a schedule.
	if got, err := (Schedule{Off: "18:00", Days: []string{"sat"}}).Normalize(); err != nil || got.On != "" || got.Off != "18:00" {
		t.Fatalf("off only = %+v, %v", got, err)
	}
	// Without times it is the empty schedule, whatever its days.
	if got, err := (Schedule{Days: []string{"mon"}}).Normalize(); err != nil || !got.Empty() || got.Days == nil || len(got.Days) != 0 {
		t.Fatalf("empty = %+v, %v", got, err)
	}
	for name, schedule := range map[string]Schedule{
		"bad on":     {On: "25:00", Days: []string{"mon"}},
		"bad off":    {Off: "x", Days: []string{"mon"}},
		"no days":    {On: "07:50"},
		"bad day":    {On: "07:50", Days: []string{"someday"}},
		"same times": {On: "07:50", Off: "07:50", Days: []string{"mon"}},
	} {
		if got, err := schedule.Normalize(); err == nil {
			t.Fatalf("%s accepted as %+v", name, got)
		}
	}
}

// The minute is read in the time zone of the time given: the controller's.
// An action that cannot start is tried again for ten minutes, so the two
// times must be further apart than that, the shorter way round midnight.
func TestScheduleTimesMustBeFurtherApartThanTheRetryWindow(t *testing.T) {
	for _, times := range [][2]string{{"07:50", "07:50"}, {"07:50", "07:55"}, {"07:50", "08:00"}, {"08:00", "07:50"}, {"23:55", "00:05"}, {"00:05", "23:55"}, {"00:00", "23:59"}} {
		if got, err := (Schedule{On: times[0], Off: times[1], Days: []string{"mon"}}).Normalize(); err == nil || !strings.Contains(err.Error(), "more than 10 minutes apart") {
			t.Fatalf("%v accepted as %+v, %v", times, got, err)
		}
	}
	for _, times := range [][2]string{{"07:50", "08:01"}, {"08:01", "07:50"}, {"23:55", "00:06"}, {"07:50", "18:00"}} {
		if _, err := (Schedule{On: times[0], Off: times[1], Days: []string{"mon"}}).Normalize(); err != nil {
			t.Fatalf("%v refused: %v", times, err)
		}
	}
	// One time alone has nothing to be apart from.
	if _, err := (Schedule{On: "07:50", Days: []string{"mon"}}).Normalize(); err != nil {
		t.Fatal(err)
	}
}

func TestScheduleActionAtNamesTheHalfSetForAMinute(t *testing.T) {
	cest := time.FixedZone("CEST", 2*3600)
	schedule := Schedule{On: "07:50", Off: "18:00", Days: []string{"mon", "tue"}}
	for want, at := range map[string]time.Time{
		ScheduleActionOn:  time.Date(2026, 10, 12, 7, 50, 59, 0, cest),
		ScheduleActionOff: time.Date(2026, 10, 13, 18, 0, 0, 0, cest),
		"":                time.Date(2026, 10, 14, 18, 0, 0, 0, cest),
	} {
		if got := schedule.ActionAt(at); got != want {
			t.Fatalf("%s: action = %q, want %q", at, got, want)
		}
	}
	moment, err := ParseScheduleMinute("2026-10-12T07:50", cest)
	if err != nil || !moment.Equal(time.Date(2026, 10, 12, 7, 50, 0, 0, cest)) {
		t.Fatalf("parsed = %v, %v", moment, err)
	}
	if _, err := ParseScheduleMinute("yesterday", cest); err == nil {
		t.Fatal("a word parsed as a minute")
	}
}

func TestScheduleDueReadsTheLocalMinuteAndWeekday(t *testing.T) {
	cest := time.FixedZone("CEST", 2*3600)
	schedule := Schedule{On: "07:50", Off: "18:00", Days: []string{"mon", "tue", "wed", "thu", "fri"}}
	monday := time.Date(2026, 10, 12, 7, 50, 30, 0, cest)
	if on, off := schedule.Due(monday); !on || off {
		t.Fatalf("Monday 07:50:30 = %v %v", on, off)
	}
	if on, off := schedule.Due(time.Date(2026, 10, 12, 18, 0, 0, 0, cest)); on || !off {
		t.Fatalf("Monday 18:00 = %v %v", on, off)
	}
	for name, now := range map[string]time.Time{
		"minute before": time.Date(2026, 10, 12, 7, 49, 59, 0, cest),
		"minute after":  time.Date(2026, 10, 12, 7, 51, 0, 0, cest),
		"saturday":      time.Date(2026, 10, 17, 7, 50, 0, 0, cest),
		"sunday":        time.Date(2026, 10, 18, 18, 0, 0, 0, cest),
		// The same instant on the clock of another zone is another minute.
		"utc clock": monday.UTC(),
	} {
		if on, off := schedule.Due(now); on || off {
			t.Fatalf("%s is due: %v %v", name, on, off)
		}
	}
	// A weekend schedule, and Sunday for Go's weekday zero.
	weekend := Schedule{On: "09:00", Days: []string{"sat", "sun"}}
	if on, _ := weekend.Due(time.Date(2026, 10, 18, 9, 0, 0, 0, cest)); !on {
		t.Fatal("Sunday 09:00 is not due")
	}
	if on, _ := weekend.Due(time.Date(2026, 10, 19, 9, 0, 0, 0, cest)); on {
		t.Fatal("Monday 09:00 is due")
	}
	if got := ScheduleMinute(monday); got != "2026-10-12T07:50" {
		t.Fatalf("minute = %q", got)
	}
}

func TestScheduleSummary(t *testing.T) {
	for want, schedule := range map[string]Schedule{
		"Switch on at 07:50 and shut down at 18:00, Monday to Friday.": {On: "07:50", Off: "18:00", Days: []string{"mon", "tue", "wed", "thu", "fri"}},
		"Switch on at 07:50, every day.":                               {On: "07:50", Days: ScheduleWeekdays},
		"Shut down at 18:00, Monday, Wednesday and Friday.":            {Off: "18:00", Days: []string{"mon", "wed", "fri"}},
		"Shut down at 12:00, Saturday and Sunday.":                     {Off: "12:00", Days: []string{"sat", "sun"}},
		"Switch on at 08:00, Tuesday.":                                 {On: "08:00", Days: []string{"tue"}},
		"No schedule.":                                                 {Days: []string{}},
	} {
		if got := schedule.Summary(); got != want {
			t.Fatalf("summary = %q, want %q", got, want)
		}
	}
}

func TestScheduleRecordValidity(t *testing.T) {
	valid := ScheduleRecord{SchemaVersion: 1, Schedule: Schedule{On: "07:50", Days: []string{"mon", "fri"}}, LastRun: "2026-10-12T07:50", Retry: "2026-10-12T07:50"}
	if !valid.Valid() {
		t.Fatal("a canonical record is not valid")
	}
	for name, record := range map[string]ScheduleRecord{
		"days out of order": {SchemaVersion: 1, Schedule: Schedule{On: "07:50", Days: []string{"fri", "mon"}}},
		"short time":        {SchemaVersion: 1, Schedule: Schedule{On: "7:50", Days: []string{"mon"}}},
		"no days":           {SchemaVersion: 1, Schedule: Schedule{On: "07:50"}},
		"bad minute":        {SchemaVersion: 1, Schedule: Schedule{On: "07:50", Days: []string{"mon"}}, LastRun: "yesterday"},
		"bad retry":         {SchemaVersion: 1, Schedule: Schedule{On: "07:50", Days: []string{"mon"}}, Retry: "07:50"},
	} {
		if record.Valid() {
			t.Fatalf("%s is valid", name)
		}
	}
}

func TestScheduleRecordJSONIsFlat(t *testing.T) {
	content, err := json.Marshal(ScheduleRecord{SchemaVersion: 1, Schedule: Schedule{On: "07:50", Off: "18:00", Days: []string{"mon", "tue"}}, LastRun: "2026-10-12T07:50", Retry: "2026-10-12T07:50"})
	want := `{"schemaVersion":1,"on":"07:50","off":"18:00","days":["mon","tue"],"lastRun":"2026-10-12T07:50","retry":"2026-10-12T07:50"}`
	if err != nil || string(content) != want {
		t.Fatalf("json = %s, %v", content, err)
	}
}

func TestScheduleOperationsAreClassroomOperations(t *testing.T) {
	for _, operation := range []ClassroomOperation{ClassroomScheduleGetOperation, ClassroomScheduleSetOperation} {
		request, err := NewClassroomRequest(operation)
		if err != nil {
			t.Fatal(err)
		}
		request.Schedule = &Schedule{On: "07:50", Days: []string{"mon"}}
		content, _ := json.Marshal(request)
		decoded, err := DecodeClassroomRequest(content)
		if err != nil || decoded.Operation != operation || decoded.Schedule == nil || decoded.Schedule.On != "07:50" {
			t.Fatalf("%s: %+v, %v", operation, decoded, err)
		}
	}
	if _, err := DecodeClassroomRequest([]byte(`{"schemaVersion":1,"requestId":"00000000000000000000000000000000","operation":"schedule-run"}`)); err == nil {
		t.Fatal("schedule-run is not a classroom operation: the timer runs it, not the service")
	}
}

func TestScheduleDueOnTheDaylightSavingNights(t *testing.T) {
	rome, err := time.LoadLocation("Europe/Rome")
	if err != nil {
		t.Skipf("no time zone database: %v", err)
	}
	at := func(month time.Month, day, hour, minute int) time.Time {
		return time.Date(2026, month, day, hour, minute, 0, 0, rome)
	}
	// Spring: on Sunday 29 March 2026 the clock goes from 02:00 to 03:00.
	schedule := Schedule{On: "02:30", Off: "02:59", Days: []string{"sun"}}
	if at(time.March, 29, 3, 0).Hour() != 3 || at(time.March, 29, 1, 59).Add(time.Minute).Hour() != 3 {
		t.Fatal("the zone has no spring jump on 29 March 2026")
	}
	for name, test := range map[string]struct {
		now     time.Time
		on, off bool
	}{
		"minute before the jump":       {at(time.March, 29, 1, 59), false, false},
		"first minute after the jump":  {at(time.March, 29, 3, 0), true, true},
		"second minute after the jump": {at(time.March, 29, 3, 1), false, false},
		"the next day":                 {at(time.March, 30, 3, 0), false, false},
		"a week earlier at 03:00":      {at(time.March, 22, 3, 0), false, false},
		"a week earlier at 02:30":      {at(time.March, 22, 2, 30), true, false},
	} {
		if on, off := schedule.Due(test.now); on != test.on || off != test.off {
			t.Errorf("%s: due = %v %v, want %v %v", name, on, off, test.on, test.off)
		}
	}
	// A time just before the jump, or right at the first minute after it,
	// keeps its own minute and does not fire a second time.
	around := Schedule{On: "01:59", Off: "03:00", Days: []string{"sun"}}
	if on, off := around.Due(at(time.March, 29, 3, 0)); on || !off {
		t.Errorf("03:00 after the jump = %v %v", on, off)
	}
	if on, _ := around.Due(at(time.March, 29, 1, 59)); !on {
		t.Error("01:59 before the jump is not due")
	}

	// Autumn: on Sunday 25 October 2026 02:00-02:59 happens twice. Both are
	// due; the record of the last run makes it run once.
	autumn := Schedule{On: "02:30", Days: []string{"sun"}}
	first := time.Date(2026, time.October, 25, 0, 30, 0, 0, time.UTC).In(rome)
	second := first.Add(time.Hour)
	if first.Hour() != 2 || second.Hour() != 2 || first.Format("MST") == second.Format("MST") {
		t.Fatalf("the zone has no autumn repeat: %s %s", first, second)
	}
	for _, now := range []time.Time{first, second} {
		if on, _ := autumn.Due(now); !on {
			t.Errorf("%s is not due", now)
		}
	}
	if on, _ := autumn.Due(at(time.October, 25, 3, 0)); on {
		t.Error("03:00 after the repeated hour fired the skipped-minute rule")
	}
}
