package domain

import (
	"testing"
	"time"
)

func at(t *testing.T, zone, value string) time.Time {
	t.Helper()
	loc, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	v, err := time.ParseInLocation("2006-01-02 15:04", value, loc)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestScheduleLastSlot(t *testing.T) {
	monday := 1
	daily := DepartmentSchedule{Frequency: ScheduleDaily, TimeOfDay: "09:00", Timezone: "Asia/Seoul"}
	weekly := DepartmentSchedule{Frequency: ScheduleWeekly, TimeOfDay: "09:00", Weekday: &monday, Timezone: "Asia/Seoul"}
	for _, c := range []struct {
		name, now string
		s         DepartmentSchedule
		due, key  string
	}{
		{"daily before the time is yesterday's", "2026-10-27 08:59", daily, "2026-10-26 09:00", "daily:2026-10-26"},
		{"daily at the time is today's", "2026-10-27 09:00", daily, "2026-10-27 09:00", "daily:2026-10-27"},
		{"weekly on a later day is this week's", "2026-10-29 12:00", weekly, "2026-10-26 09:00", "weekly:2026-W44"},
		{"weekly on the day before the time is last week's", "2026-10-26 08:00", weekly, "2026-10-19 09:00", "weekly:2026-W43"},
		{"weekly on Sunday is this ISO week's Monday", "2026-11-01 23:00", weekly, "2026-10-26 09:00", "weekly:2026-W44"},
	} {
		due, key, err := c.s.LastSlot(at(t, "Asia/Seoul", c.now))
		if err != nil {
			t.Fatal(err)
		}
		if got := due.Format("2006-01-02 15:04"); got != c.due || key != c.key {
			t.Errorf("%s: %s %s, want %s %s", c.name, got, key, c.due, c.key)
		}
	}

	// The slot is the schedule's time zone's day, whatever zone now is in.
	due, key, _ := daily.LastSlot(at(t, "UTC", "2026-10-27 01:00")) // 10:00 in Seoul
	if key != "daily:2026-10-27" || !due.Equal(at(t, "Asia/Seoul", "2026-10-27 09:00")) {
		t.Fatalf("zone: %s %s", due, key)
	}
	// Changing the time of day keeps the day's key: the same day is not run twice.
	later := daily
	later.TimeOfDay = "18:00"
	if _, k, _ := later.LastSlot(at(t, "Asia/Seoul", "2026-10-27 19:00")); k != "daily:2026-10-27" {
		t.Fatalf("changed time: %s", k)
	}
	// Across a daylight saving change the wall-clock time stays.
	ny := DepartmentSchedule{Frequency: ScheduleDaily, TimeOfDay: "09:00", Timezone: "America/New_York"}
	due, _, _ = ny.LastSlot(at(t, "America/New_York", "2026-10-31 10:00")) // DST ends that night
	next, _ := ny.NextSlot(at(t, "America/New_York", "2026-10-31 10:00"))
	if due.Format("15:04") != "09:00" || next.Format("2006-01-02 15:04") != "2026-11-01 09:00" || next.Sub(due) != 25*time.Hour {
		t.Fatalf("DST: %s → %s", due, next)
	}
}

func TestScheduleCheck(t *testing.T) {
	sunday, eight := 0, 8
	for _, c := range []struct {
		s  DepartmentSchedule
		ok bool
	}{
		{DepartmentSchedule{Frequency: ScheduleDaily, TimeOfDay: "00:00", Timezone: "Asia/Seoul"}, true},
		{DepartmentSchedule{Frequency: ScheduleWeekly, TimeOfDay: "23:59", Weekday: &sunday, Timezone: "UTC"}, true},
		{DepartmentSchedule{Frequency: ScheduleWeekly, TimeOfDay: "09:00", Timezone: "UTC"}, false},
		{DepartmentSchedule{Frequency: ScheduleWeekly, TimeOfDay: "09:00", Weekday: &eight, Timezone: "UTC"}, false},
		{DepartmentSchedule{Frequency: ScheduleDaily, TimeOfDay: "09:00", Weekday: &sunday, Timezone: "UTC"}, false},
		{DepartmentSchedule{Frequency: ScheduleDaily, TimeOfDay: "9:00", Timezone: "UTC"}, false},
		{DepartmentSchedule{Frequency: ScheduleDaily, TimeOfDay: "24:00", Timezone: "UTC"}, false},
		{DepartmentSchedule{Frequency: ScheduleDaily, TimeOfDay: "09:00", Timezone: "Mars/Base"}, false},
		{DepartmentSchedule{Frequency: "hourly", TimeOfDay: "09:00", Timezone: "UTC"}, false},
	} {
		if err := c.s.Check(); (err == nil) != c.ok {
			t.Errorf("%+v: %v", c.s, err)
		}
	}
}
