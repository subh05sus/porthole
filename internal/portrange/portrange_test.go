package portrange

import "testing"

func TestParseSinglePort(t *testing.T) {
	got, err := Parse("3000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != 3000 {
		t.Fatalf("got %v, want [3000]", got)
	}
}

func TestParseRange(t *testing.T) {
	got, err := Parse("3000-3003")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []int{3000, 3001, 3002, 3003}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestParseDescendingRangeErrors(t *testing.T) {
	_, err := Parse("3010-3000")
	if err == nil {
		t.Fatalf("expected an error for a descending range")
	}
}

func TestParseNonNumericErrors(t *testing.T) {
	_, err := Parse("abc")
	if err == nil {
		t.Fatalf("expected an error for a non-numeric token")
	}
}

func TestParseSinglePortWithWhitespace(t *testing.T) {
	got, err := Parse(" 3000 ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != 3000 {
		t.Fatalf("got %v, want [3000]", got)
	}
}
