package main

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// Every transform's mapping is a live-verified wire form; these tables pin them.
func TestTransformsGroundedMappings(t *testing.T) {
	cases := []struct {
		tf   string
		in   any
		want any
	}{
		{"pause_schedule", "30m", "30_minutes"},
		{"pause_schedule", "1h", "1_hour"},
		{"pause_schedule", "2h", "2_hour"},
		{"pause_schedule", "4h", "4_hour"},
		{"pause_schedule", "until-morning", "Until_tomorrow_morning"},
		{"tz_short", "EST", "EST"},
		{"tz_short", "PST", "PST"},
		{"bool01", true, 1},
		{"bool01", false, 0},
		{"allow_block_ab", "allow", "a"},
		{"allow_block_ab", "Block", "b"},
		{"allow_block_ab", "b", "b"},
		{"day3_lower", "Mon,tuesday,WED", []string{"mon", "tue", "wed"}},
		{"day3_lower", "monday,Tuesday,wednesday,thursday,friday,saturday,sunday", []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}},
		{"day3_lower", "weekdays", []string{"mon", "tue", "wed", "thu", "fri"}},
		{"day3_lower", []string{"weekends", "mon"}, []string{"sat", "sun", "mon"}},
		{"day3_lower", "all", []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}},
		{"day3_title", "sun,mon", []string{"Sun", "Mon"}},
		{"day3_title", "mon,mon", []string{"Mon"}}, // de-duplicated
		{"iso_micro", time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), "2026-09-01T00:00:00.000000Z"},
		{"iso_micro", "2026-09-01", "2026-09-01T00:00:00.000000Z"},
		{"iso_micro", "2026-09-01T12:34:56Z", "2026-09-01T12:34:56.000000Z"},
		{"epoch_ms", time.UnixMilli(1700000000000).UTC(), int64(1700000000000)},
		{"epoch_ms", int64(1700000000000), int64(1700000000000)},
		{"", "untouched", "untouched"}, // no transform: identity
	}
	for _, c := range cases {
		got, err := applyTransform(c.tf, c.in)
		if err != nil {
			t.Errorf("%s(%v): %v", c.tf, c.in, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s(%v) = %#v, want %#v", c.tf, c.in, got, c.want)
		}
	}
}

// An IANA zone renders as its current letter abbreviation (PDT or PST for Los Angeles).
func TestTZShortFromIANA(t *testing.T) {
	got, err := applyTransform("tz_short", "America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	if s := got.(string); s != "PST" && s != "PDT" {
		t.Errorf("America/Los_Angeles -> %q, want PST or PDT", s)
	}
}

// The --last 7d sugar is a span back from now, rendered in the op's format.
func TestISOMicroDurationBack(t *testing.T) {
	got, err := applyTransform("iso_micro", "7d")
	if err != nil {
		t.Fatal(err)
	}
	ts, err := time.Parse("2006-01-02T15:04:05.000000Z", got.(string))
	if err != nil {
		t.Fatalf("not in the microsecond ISO form: %v (%v)", err, got)
	}
	if d := time.Since(ts); d < 7*24*time.Hour-time.Minute || d > 7*24*time.Hour+time.Minute {
		t.Errorf("7d should be ~7 days ago, got %v ago", d)
	}
}

// Bad inputs name what was wrong and what is accepted.
func TestTransformsRejectBadInput(t *testing.T) {
	cases := []struct {
		tf   string
		in   any
		want string
	}{
		{"pause_schedule", "45m", "30m|1h|2h|4h|until-morning"},
		{"tz_short", "Mars/Olympus", "IANA zone"},
		{"iso_micro", "yesterday-ish", "not a time"},
		{"iso_micro", "106752d", "too large"},               // would overflow time.Duration into the FUTURE
		{"iso_micro", "99999999999999999999d", "too large"}, // overflows the integer itself
		{"epoch_ms", "3650000h", "too large"},
		{"day3_lower", "funday", "not a day name"},
		{"day3_lower", "mondayx", "not a day name"}, // a prefix match would silently make this mon
		{"day3_lower", "sundae", "not a day name"},
		{"day3_title", "tue-ish", "not a day name"},
		{"day3_lower", "", "no days"},
		{"allow_block_ab", "maybe", "allow or block"},
		{"bool01", "yes", "wants a bool"},
		{"weekday_ints", []int{1}, "not implemented"}, // ungrounded: deliberately absent
	}
	for _, c := range cases {
		_, err := applyTransform(c.tf, c.in)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s(%v): want error containing %q, got %v", c.tf, c.in, c.want, err)
		}
	}
}
