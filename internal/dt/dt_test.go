package dt

import (
	"testing"
	"time"
)

// ─── ParseCalDuration ─────────────────────────────────────────────────────────

func TestParseCalDuration(t *testing.T) {
	tests := []struct {
		input string
		want  CalDuration
		isErr bool
	}{
		{"1h", CalDuration{Hours: 1}, false},
		{"2d", CalDuration{Days: 2}, false},
		{"1mo", CalDuration{Months: 1}, false},
		{"1y2mo3d", CalDuration{Years: 1, Months: 2, Days: 3}, false},
		{"5w2d", CalDuration{Weeks: 5, Days: 2}, false},
		{"-2d", CalDuration{Neg: true, Days: 2}, false},
		{"+3h", CalDuration{Hours: 3}, false},
		{"", CalDuration{}, true},
		{"abc", CalDuration{}, true},
	}
	for _, tc := range tests {
		got, err := ParseCalDuration(tc.input)
		if tc.isErr {
			if err == nil {
				t.Errorf("ParseCalDuration(%q): expected error, got nil", tc.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseCalDuration(%q): unexpected error: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseCalDuration(%q) = %+v, want %+v", tc.input, got, tc.want)
		}
	}
}

// ─── CalDuration.AddTo ────────────────────────────────────────────────────────

func TestAddTo(t *testing.T) {
	base := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		input string
		want  time.Time
	}{
		// 5w2d = 37 days → 2026-03-11
		{"5w2d", time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)},
		// 1mo → 2026-03-02
		{"1mo", time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)},
		// -7d → 2026-01-26
		{"-7d", time.Date(2026, 1, 26, 0, 0, 0, 0, time.UTC)},
		// 1y → 2027-02-02
		{"1y", time.Date(2027, 2, 2, 0, 0, 0, 0, time.UTC)},
		// 1h → same day, +1h
		{"1h", base.Add(time.Hour)},
	}
	for _, tc := range tests {
		d, err := ParseCalDuration(tc.input)
		if err != nil {
			t.Fatalf("ParseCalDuration(%q): %v", tc.input, err)
		}
		got := d.AddTo(base)
		if !got.Equal(tc.want) {
			t.Errorf("AddTo(%q) from %s = %s, want %s", tc.input, base.Format("2006-01-02"), got.Format("2006-01-02"), tc.want.Format("2006-01-02"))
		}
	}
}

// ─── ParseDate ────────────────────────────────────────────────────────────────

func TestParseDate(t *testing.T) {
	want := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)

	inputs := []string{
		"2026-02-02",
		"2026/02/02",
		"02/02/2026", // DD/MM/YYYY
		"Feb 2, 2026",
		"2 Feb 2026",
	}
	for _, s := range inputs {
		got, err := ParseDate(s)
		if err != nil {
			t.Errorf("ParseDate(%q): unexpected error: %v", s, err)
			continue
		}
		if !got.Equal(want) {
			t.Errorf("ParseDate(%q) = %s, want %s", s, got.Format("2006-01-02"), want.Format("2006-01-02"))
		}
	}

	// Unambiguous DD/MM/YYYY: 03/02/2026 must be Feb 3, not Mar 2.
	got, err := ParseDate("03/02/2026")
	if err != nil {
		t.Fatalf("ParseDate(\"03/02/2026\"): unexpected error: %v", err)
	}
	wantDDMM := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)
	if !got.Equal(wantDDMM) {
		t.Errorf("ParseDate(\"03/02/2026\") = %s, want %s (DD/MM/YYYY)", got.Format("2006-01-02"), wantDDMM.Format("2006-01-02"))
	}

	_, err = ParseDate("not-a-date")
	if err == nil {
		t.Error("ParseDate(\"not-a-date\"): expected error, got nil")
	}
}
