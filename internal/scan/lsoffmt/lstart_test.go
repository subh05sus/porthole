package lsoffmt

import (
	"testing"
	"time"
)

func TestParseLstartDoubleDigitDay(t *testing.T) {
	got, err := ParseLstart("Wed Aug 13 20:15:00 2026")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2026, time.August, 13, 20, 15, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseLstartSingleDigitDayExtraSpace(t *testing.T) {
	// ps right-aligns the day in a fixed-width field, so single-digit days
	// produce a double space that strings.Fields must absorb.
	got, err := ParseLstart("Wed Aug  3 09:05:07 2026")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2026, time.August, 3, 9, 5, 7, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseLstartMalformedErrors(t *testing.T) {
	if _, err := ParseLstart("not a timestamp"); err == nil {
		t.Fatalf("expected error for malformed lstart")
	}
}
