package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata" // Windows has no zone database for LoadLocation
)

// Schedule frequencies.
const (
	ScheduleDaily  = "daily"
	ScheduleWeekly = "weekly"
)

// DepartmentSchedule is a department's regular check (plan §4.3, stage
// 8): every day or every week at a time of day, in the time zone of the
// PC when it was saved. Each due slot asks the department to look for
// new work, the same way a person's check request does.
type DepartmentSchedule struct {
	ID           string `json:"id"`
	ProjectID    string `json:"projectId"`
	DepartmentID string `json:"departmentId"`
	Frequency    string `json:"frequency"` // daily | weekly
	TimeOfDay    string `json:"timeOfDay"` // HH:MM
	Weekday      *int   `json:"weekday"`   // weekly only; 0 is Sunday
	Timezone     string `json:"timezone"`  // IANA name, e.g. Asia/Seoul
	Note         string `json:"note"`      // what the department looks for
	Enabled      bool   `json:"enabled"`
	// ActiveFrom is the last save: slots due before it are not run.
	ActiveFrom string `json:"activeFrom"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

// Check reports what makes the schedule unusable, in words for the person.
func (s DepartmentSchedule) Check() error {
	switch s.Frequency {
	case ScheduleDaily:
		if s.Weekday != nil {
			return errors.New("매일 점검에는 요일을 고르지 않습니다")
		}
	case ScheduleWeekly:
		if s.Weekday == nil || *s.Weekday < 0 || *s.Weekday > 6 {
			return errors.New("매주 점검할 요일을 고르세요")
		}
	default:
		return errors.New("점검 주기는 매일 또는 매주입니다")
	}
	if _, _, err := s.clock(); err != nil {
		return err
	}
	if _, err := s.location(); err != nil {
		return err
	}
	return nil
}

func (s DepartmentSchedule) clock() (int, int, error) {
	t, err := time.Parse("15:04", s.TimeOfDay)
	if err != nil || len(s.TimeOfDay) != 5 {
		return 0, 0, errors.New("점검 시각은 00:00 형식입니다")
	}
	return t.Hour(), t.Minute(), nil
}

func (s DepartmentSchedule) location() (*time.Location, error) {
	if strings.TrimSpace(s.Timezone) == "" {
		return nil, errors.New("시간대가 없습니다")
	}
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return nil, fmt.Errorf("알 수 없는 시간대입니다: %s", s.Timezone)
	}
	return loc, nil
}

// LastSlot is the schedule's latest due time at or before now, with the
// key that names that slot: the day ("daily:2026-10-27") or the ISO week
// ("weekly:2026-W44") in the schedule's time zone. A slot keeps its key
// when the time of day changes, so a changed schedule does not run the
// same day or week twice.
func (s DepartmentSchedule) LastSlot(now time.Time) (time.Time, string, error) {
	if err := s.Check(); err != nil {
		return time.Time{}, "", err
	}
	loc, _ := s.location()
	hh, mm, _ := s.clock()
	local := now.In(loc)
	due := time.Date(local.Year(), local.Month(), local.Day(), hh, mm, 0, 0, loc)
	if s.Frequency == ScheduleDaily {
		if due.After(now) {
			due = due.AddDate(0, 0, -1)
		}
		return due, "daily:" + due.Format("2006-01-02"), nil
	}
	due = due.AddDate(0, 0, -((int(local.Weekday()) - *s.Weekday + 7) % 7))
	if due.After(now) {
		due = due.AddDate(0, 0, -7)
	}
	year, week := due.ISOWeek()
	return due, fmt.Sprintf("weekly:%04d-W%02d", year, week), nil
}

// NextSlot is the first due time after now.
func (s DepartmentSchedule) NextSlot(now time.Time) (time.Time, error) {
	last, _, err := s.LastSlot(now)
	if err != nil {
		return time.Time{}, err
	}
	if s.Frequency == ScheduleDaily {
		return last.AddDate(0, 0, 1), nil
	}
	return last.AddDate(0, 0, 7), nil
}
