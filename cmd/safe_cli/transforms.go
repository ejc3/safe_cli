package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// transforms is the engine's fixed registry, named by a flag's `transform` in the
// descriptor (docs/CLI-DESIGN.md §4). Each turns a flag's parsed value into the wire form
// the op wants. Every mapping here is grounded in a live-verified body_example or op
// description; a form that has not been observed on the wire (postScheduleAlert's
// weekDays ints) is deliberately absent, so a descriptor that names it fails at Parse
// rather than shipping a guess.
var transforms = map[string]func(any) (any, error){
	"pause_schedule": tfPauseSchedule,
	"tz_short":       tfTZShort,
	"iso_micro":      tfISOMicro,
	"epoch_ms":       tfEpochMs,
	"day3_lower":     tfDay3Lower,
	"day3_title":     tfDay3Title,
	"bool01":         tfBool01,
	"allow_block_ab": tfAllowBlockAB,
}

// applyTransform runs the named transform, or returns v unchanged for "".
func applyTransform(name string, v any) (any, error) {
	if name == "" {
		return v, nil
	}
	f, ok := transforms[name]
	if !ok {
		return nil, fmt.Errorf("transform %q is not implemented by the engine", name)
	}
	return f(v)
}

// pauseSchedules is pause_internet.pauseInternet's pauseSchedule vocabulary (the app's
// display timing with spaces->underscores; from the op description and live getDevices
// pauseTimings): the CLI's short --for values map onto it.
var pauseSchedules = map[string]string{
	"30m":           "30_minutes",
	"1h":            "1_hour",
	"2h":            "2_hour",
	"4h":            "4_hour",
	"until-morning": "Until_tomorrow_morning",
}

func tfPauseSchedule(v any) (any, error) {
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("pause_schedule wants a string, got %T", v)
	}
	if w, ok := pauseSchedules[s]; ok {
		return w, nil
	}
	return nil, fmt.Errorf("--for %q is not one of 30m|1h|2h|4h|until-morning", s)
}

var shortZoneRe = regexp.MustCompile(`^[A-Z]{3,5}$`)

// tfTZShort renders a zone as the short code the pause/schedule ops take ("EST", "PST").
// A value that already is a short code passes through; an IANA name ("America/Los_Angeles")
// becomes its current abbreviation. Live note: pauseInternet accepted EST on a Pacific
// account, so the code steers timing display, not authorization.
func tfTZShort(v any) (any, error) {
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("tz_short wants a string, got %T", v)
	}
	if shortZoneRe.MatchString(s) {
		return s, nil
	}
	loc, err := time.LoadLocation(s)
	if err != nil {
		return nil, fmt.Errorf("--timezone %q is neither a short code (EST) nor an IANA zone (America/New_York)", s)
	}
	abbr := time.Now().In(loc).Format("MST")
	if !shortZoneRe.MatchString(abbr) {
		return nil, fmt.Errorf("zone %q has no letter abbreviation (%s); pass a short code with --timezone", s, abbr)
	}
	return abbr, nil
}

// durationBackRe matches the "--last 7d | 24h | 90m | 30s" sugar: a span back from now.
var durationBackRe = regexp.MustCompile(`^(\d+)([smhd])$`)

// parseTimeSpec accepts the time forms every time-range flag takes (docs/CLI-DESIGN.md
// §3): "now", RFC3339 (with or without fractional seconds), a bare date (midnight UTC),
// or a duration back from now like 7d/24h/90m.
func parseTimeSpec(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "now" {
		return now, nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000000Z"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	if m := durationBackRe.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		unit := map[string]time.Duration{"s": time.Second, "m": time.Minute, "h": time.Hour, "d": 24 * time.Hour}[m[2]]
		return now.Add(-time.Duration(n) * unit), nil
	}
	return time.Time{}, fmt.Errorf("%q is not a time: use now, RFC3339, YYYY-MM-DD, or a span back like 7d/24h/90m", s)
}

func toTime(v any) (time.Time, error) {
	switch t := v.(type) {
	case time.Time:
		return t, nil
	case string:
		return parseTimeSpec(t, time.Now())
	default:
		return time.Time{}, fmt.Errorf("wants a time, got %T", v)
	}
}

// tfISOMicro renders the yyyy-MM-dd'T'HH:mm:ss.SSSSSS'Z' form the callandtext v7 ops
// require (verified live: the API 400s any other start/end date).
func tfISOMicro(v any) (any, error) {
	t, err := toTime(v)
	if err != nil {
		return nil, fmt.Errorf("iso_micro: %w", err)
	}
	return t.UTC().Format("2006-01-02T15:04:05.000000Z"), nil
}

// tfEpochMs renders epoch milliseconds (family_line.getSpcToken tokenIssued,
// postScheduleAlert eventDateTime). An integer passes through.
func tfEpochMs(v any) (any, error) {
	switch t := v.(type) {
	case int64:
		return t, nil
	case int:
		return int64(t), nil
	case float64:
		return int64(t), nil
	}
	t, err := toTime(v)
	if err != nil {
		return nil, fmt.Errorf("epoch_ms: %w", err)
	}
	return t.UnixMilli(), nil
}

// dayNames indexes Monday-first so expansions read the way a schedule is written.
var dayNames = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

// dayByName accepts exactly the 3-letter or full English day names (lowercase).
var dayByName = map[string]string{
	"mon": "mon", "monday": "mon", "tue": "tue", "tuesday": "tue", "wed": "wed", "wednesday": "wed",
	"thu": "thu", "thursday": "thu", "fri": "fri", "friday": "fri", "sat": "sat", "saturday": "sat",
	"sun": "sun", "sunday": "sun",
}

// parseDays accepts a list (or comma-separated string) of day names in any case, full or
// 3-letter, plus the expansions weekdays | weekends | all, and returns lowercase 3-letter
// names in the order given (expansions in Monday-first order), de-duplicated.
func parseDays(v any) ([]string, error) {
	var toks []string
	switch t := v.(type) {
	case string:
		toks = strings.Split(t, ",")
	case []string:
		toks = t
	case []any:
		for _, el := range t {
			s, ok := el.(string)
			if !ok {
				return nil, fmt.Errorf("day list contains a non-string %T", el)
			}
			toks = append(toks, s)
		}
	default:
		return nil, fmt.Errorf("wants day names, got %T", v)
	}
	seen := make(map[string]bool)
	var out []string
	add := func(d string) {
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	for _, tok := range toks {
		s := strings.ToLower(strings.TrimSpace(tok))
		switch s {
		case "":
			continue
		case "weekdays":
			for _, d := range dayNames[:5] {
				add(d)
			}
		case "weekends":
			add("sat")
			add("sun")
		case "all", "everyday", "every-day", "daily":
			for _, d := range dayNames {
				add(d)
			}
		default:
			// Exact 3-letter or full name only — a prefix match would silently turn a typo
			// like "mondayx" into mon and create a schedule on the wrong day.
			d, ok := dayByName[s]
			if !ok {
				return nil, fmt.Errorf("%q is not a day name (mon..sun, weekdays, weekends, all)", tok)
			}
			add(d)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no days given")
	}
	return out, nil
}

// tfDay3Lower: schedules.createAppLimit limits.days / postScreenTimeData weeklyLimits keys
// (lowercase 3-letter, verified live).
func tfDay3Lower(v any) (any, error) {
	days, err := parseDays(v)
	if err != nil {
		return nil, fmt.Errorf("day3_lower: %w", err)
	}
	return days, nil
}

// tfDay3Title: schedules.postSchedule days ("Mon", 3-letter title case, verified live).
func tfDay3Title(v any) (any, error) {
	days, err := parseDays(v)
	if err != nil {
		return nil, fmt.Errorf("day3_title: %w", err)
	}
	out := make([]string, len(days))
	for i, d := range days {
		out[i] = strings.ToUpper(d[:1]) + d[1:]
	}
	return out, nil
}

// tfBool01: contacts.updateTrustedContacts settingValue 0|1 (verified live).
func tfBool01(v any) (any, error) {
	b, ok := v.(bool)
	if !ok {
		return nil, fmt.Errorf("bool01 wants a bool, got %T", v)
	}
	if b {
		return 1, nil
	}
	return 0, nil
}

// tfAllowBlockAB: website.postWebsites domains[].status "a"=allow | "b"=block (verified live).
func tfAllowBlockAB(v any) (any, error) {
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("allow_block_ab wants a string, got %T", v)
	}
	switch strings.ToLower(s) {
	case "allow", "a":
		return "a", nil
	case "block", "b":
		return "b", nil
	}
	return nil, fmt.Errorf("%q must be allow or block", s)
}
