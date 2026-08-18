package backup

import (
	"testing"
	"time"
)

func TestNextRunDaily(t *testing.T) {
	// before today's 03:00 -> today 03:00
	after := time.Date(2026, 8, 18, 1, 0, 0, 0, time.UTC)
	next := NextRun(Schedule{Type: "daily", Time: "03:00"}, after)
	if !next.Equal(time.Date(2026, 8, 18, 3, 0, 0, 0, time.UTC)) {
		t.Errorf("daily next = %v", next)
	}

	// after today's 03:00 -> tomorrow 03:00
	after = time.Date(2026, 8, 18, 4, 0, 0, 0, time.UTC)
	next = NextRun(Schedule{Type: "daily", Time: "03:00"}, after)
	if !next.Equal(time.Date(2026, 8, 19, 3, 0, 0, 0, time.UTC)) {
		t.Errorf("daily next = %v", next)
	}
}

func TestNextRunWeekly(t *testing.T) {
	// 2026-08-18 is a Tuesday. Schedule weekly Monday (1) 03:00.
	after := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	next := NextRun(Schedule{Type: "weekly", Time: "03:00", Day: 1}, after)
	if !next.Equal(time.Date(2026, 8, 24, 3, 0, 0, 0, time.UTC)) {
		t.Errorf("weekly next = %v", next)
	}
}

func TestNextRunMonthly(t *testing.T) {
	// schedule monthly day 15 03:00
	after := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	next := NextRun(Schedule{Type: "monthly", Time: "03:00", Day: 15}, after)
	if !next.Equal(time.Date(2026, 9, 15, 3, 0, 0, 0, time.UTC)) {
		t.Errorf("monthly next = %v", next)
	}

	// day 31 in a month without 31 -> clamps to last day (Feb 28)
	after = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	next = NextRun(Schedule{Type: "monthly", Time: "03:00", Day: 31}, after)
	if !next.Equal(time.Date(2026, 2, 28, 3, 0, 0, 0, time.UTC)) {
		t.Errorf("monthly clamp next = %v", next)
	}
}
