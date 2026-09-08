package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// The pause-internet template, as in the descriptor: typed substitution for every var,
// and the nested "$for?" property omitted when --indefinite nulls it.
const pauseTpl = `{"timeZone":"$tz","profiles":[{"profileId":"$child.profileId","devices":[{"serviceId":"$child.serviceId","deviceId":"$child.deviceId","pauseSchedule":"$for?","untilIUnpause":"$indefinite","callOnlyMode":"$callOnly"}]}]}`

func TestRenderTemplateTypedSubstitution(t *testing.T) {
	vars := map[string]any{
		"tz": "PST", "child.profileId": int64(3000001), "child.serviceId": int64(2000001),
		"child.deviceId": int64(4000001), "for": "30_minutes", "indefinite": false, "callOnly": false,
	}
	b, err := renderTemplate(pauseTpl, vars)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// numbers stay numbers, bools stay bools, strings are quoted
	for _, want := range []string{`"profileId":3000001`, `"serviceId":2000001`, `"deviceId":4000001`, `"untilIUnpause":false`, `"callOnlyMode":false`, `"pauseSchedule":"30_minutes"`, `"timeZone":"PST"`} {
		if !strings.Contains(s, want) {
			t.Errorf("rendered body missing %s:\n%s", want, s)
		}
	}
	if strings.Contains(s, "$") {
		t.Errorf("unrendered variable left in body: %s", s)
	}
}

// --indefinite: "$for?" has no value, so pauseSchedule is ABSENT (not null, not ""), and
// untilIUnpause=true — the wire-verified indefinite body (Codex #66-1).
func TestRenderTemplateOmitsOptionalWithoutValue(t *testing.T) {
	vars := map[string]any{
		"tz": "PST", "child.profileId": int64(1), "child.serviceId": int64(2), "child.deviceId": int64(3),
		"indefinite": true, "callOnly": false, // no "for" at all
	}
	b, err := renderTemplate(pauseTpl, vars)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "pauseSchedule") {
		t.Errorf("optional pauseSchedule must be omitted entirely, got %s", b)
	}
	if !strings.Contains(string(b), `"untilIUnpause":true`) {
		t.Errorf("untilIUnpause must be true: %s", b)
	}
	// An explicit nil value omits too (that is how a nulled flag is represented).
	vars["for"] = nil
	b, _ = renderTemplate(pauseTpl, vars)
	if strings.Contains(string(b), "pauseSchedule") {
		t.Errorf("nil optional must be omitted, got %s", b)
	}
}

// A required variable with no value is an error naming it — never an empty field.
func TestRenderTemplateRequiredMissingErrors(t *testing.T) {
	_, err := renderTemplate(`{"a":"$a","b":"$b?"}`, map[string]any{"b": "x"})
	if err == nil || !strings.Contains(err.Error(), "$a") {
		t.Fatalf("want an error naming $a, got %v", err)
	}
}

// Optional elements inside arrays are dropped; literals and non-variable strings pass
// through untouched; a "$" that is not a variable name is still a variable (no escaping).
func TestRenderTemplateArraysAndLiterals(t *testing.T) {
	b, err := renderTemplate(`{"days":["$mon?","$tue?","Wed"],"fixed":7,"note":"plain"}`, map[string]any{"tue": "Tue"})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	days, _ := m["days"].([]any)
	if len(days) != 2 || days[0] != "Tue" || days[1] != "Wed" {
		t.Errorf("days = %v, want [Tue Wed]", days)
	}
	if m["fixed"] != float64(7) || m["note"] != "plain" {
		t.Errorf("literals altered: %v", m)
	}
}

func TestRenderQuery(t *testing.T) {
	q, err := renderQuery(map[string]string{
		"categorySupported": "v6", "startDate": "$since", "betaProviders": "$providers", "skip": "$absent",
	}, map[string]any{"since": "2026-01-01T00:00:00.000000Z", "providers": []string{"RCS", "GIZMO"}})
	if err != nil {
		t.Fatal(err)
	}
	if q.Get("categorySupported") != "v6" || q.Get("startDate") != "2026-01-01T00:00:00.000000Z" || q.Get("betaProviders") != "RCS,GIZMO" {
		t.Errorf("query = %v", q)
	}
	if _, present := q["skip"]; present {
		t.Errorf("a $flag with no value must be omitted, got %v", q)
	}
	if got, _ := renderQuery(nil, nil); got != nil {
		t.Errorf("empty query map must render nil, got %v", got)
	}
}
