package domain

import (
	"errors"
	"testing"
)

func mustDate(t *testing.T, value string) CalendarDate {
	t.Helper()
	d, err := ParseDate(value)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func TestNextOccurrenceJumpsAcrossCenturiesWithoutDurationOverflow(t *testing.T) {
	interval := 7
	rule := RecurrenceRule{Type: RecurrenceEveryNDays, StartDate: mustDate(t, "0001-01-01"), IntervalDays: &interval}
	from := mustDate(t, "2026-07-11")
	next, ok := NextOccurrence(rule, from)
	if !ok || next.Before(from) || DaysBetween(from, next) > 6 || DaysBetween(rule.StartDate, next)%7 != 0 {
		t.Fatalf("invalid next occurrence: %v, %v", next, ok)
	}
	if !OccursOn(rule, next) {
		t.Fatal("next occurrence is not recognized by OccursOn")
	}
	if OccursOn(rule, next.AddDays(1)) {
		t.Fatal("non-occurrence accepted")
	}
	huge := int(^uint(0) >> 1)
	rule.IntervalDays = &huge
	if _, ok := NextOccurrence(rule, from); ok {
		t.Fatal("interval outside supported date range must have no next occurrence")
	}
}
func TestMonthlyOccurrenceLeapClampAndBounds(t *testing.T) {
	day := 31
	rule := RecurrenceRule{Type: RecurrenceMonthlyOnDay, StartDate: mustDate(t, "2024-01-31"), DayOfMonth: &day}
	next, ok := NextOccurrence(rule, mustDate(t, "2024-02-01"))
	if !ok || next.String() != "2024-02-29" {
		t.Fatalf("leap clamp: %v %v", next, ok)
	}
	end := next
	rule.Until = &end
	if _, ok := NextOccurrence(rule, mustDate(t, "2024-03-01")); ok {
		t.Fatal("recurrence ignored until")
	}
	if _, err := ParseDate("0000-01-01"); err == nil {
		t.Fatal("year zero accepted")
	}
}
func TestMoneyArithmeticChecksBothBounds(t *testing.T) {
	for _, v := range [][2]int64{{MaxMoney, 1}, {-MaxMoney, -1}, {MaxMoney + 1, -1}} {
		if _, err := AddMoney(v[0], v[1]); !errors.Is(err, ErrConflict) {
			t.Fatalf("overflow %v was accepted: %v", v, err)
		}
	}
	if v, err := AddMoney(-MaxMoney, MaxMoney); err != nil || v != 0 {
		t.Fatalf("valid arithmetic: %d %v", v, err)
	}
}
