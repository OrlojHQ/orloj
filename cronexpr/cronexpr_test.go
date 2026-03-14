package cronexpr

import (
	"testing"
	"time"
)

func TestParseRejectsInvalidFieldCount(t *testing.T) {
	if _, err := Parse("* * * *"); err == nil {
		t.Fatal("expected parse error for invalid field count")
	}
}

func TestNextAndPrev(t *testing.T) {
	expr, err := Parse("*/15 9-10 * * 1-5")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	base := time.Date(2026, time.March, 13, 9, 7, 30, 0, time.UTC) // Friday
	next, ok := expr.Next(base)
	if !ok {
		t.Fatal("expected next match")
	}
	if next.Format(time.RFC3339) != "2026-03-13T09:15:00Z" {
		t.Fatalf("unexpected next %s", next.Format(time.RFC3339))
	}

	prev, ok := expr.Prev(base)
	if !ok {
		t.Fatal("expected prev match")
	}
	if prev.Format(time.RFC3339) != "2026-03-13T09:00:00Z" {
		t.Fatalf("unexpected prev %s", prev.Format(time.RFC3339))
	}
}

func TestDayOfWeekSevenMapsToSunday(t *testing.T) {
	expr, err := Parse("0 0 * * 7")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	base := time.Date(2026, time.March, 13, 0, 0, 0, 0, time.UTC) // Friday
	next, ok := expr.Next(base)
	if !ok {
		t.Fatal("expected sunday match")
	}
	if next.Weekday() != time.Sunday {
		t.Fatalf("expected sunday, got %s", next.Weekday())
	}
}
