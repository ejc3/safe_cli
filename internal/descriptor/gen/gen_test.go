package gen

import (
	"regexp"
	"strings"
	"testing"

	"github.com/ejc3/safe_cli/internal/descriptor"
)

// A small descriptor whose verb has a required flag and a select branch onto a
// destructive op, while the base op is not destructive.
const fixture = `{"name":"t","base_url":"https://h","areas":{"a":"help"},"entities":{"t":{"id_field":"","operations":{
  "base":{"method":"GET","path":"/b","query":["q","q2"],
    "cli":{"area":"a","verb":"v","priority":"core","target":"child","summary":"s",
      "select":[{"when":"flag:x","op":"t.del"}],
      "flags":[{"name":"x","type":"string","required":true,"maps_to":"query:q","help":"h"},
               {"name":"y","type":"int","maps_to":"query:q2","help":"h"}]}},
  "del":{"method":"DELETE","path":"/d","query":["q","q2"],"destructive":true}}}}}`

func source(t *testing.T) string {
	t.Helper()
	d, err := descriptor.Parse([]byte(fixture))
	if err != nil {
		t.Fatalf("fixture must validate: %v", err)
	}
	src, err := Source(d)
	if err != nil {
		t.Fatalf("Source: %v", err)
	}
	return string(src)
}

// Codex #71-4: a descriptor flag marked required is required to kong too, so the usage
// line shows it and a missing value is a parse error, not a later request error.
func TestRequiredFlagIsRequiredToKong(t *testing.T) {
	src := source(t)
	// gofmt aligns struct columns, so match with flexible whitespace.
	if !regexp.MustCompile("X\\s+\\*string\\s+`name:\"x\" help:\"h\" required:\"\"`").MatchString(src) {
		t.Errorf("required flag must carry kong's required marker:\n%s", src)
	}
	if !regexp.MustCompile("Y\\s+\\*int64\\s+`name:\"y\" help:\"h\"`").MatchString(src) {
		t.Errorf("an optional flag must not be marked required:\n%s", src)
	}
}

// Codex #71-5: when a select branch reaches a destructive op, the verb must offer
// --confirm even though its base op is not destructive — the engine demands it for the
// selected op, and a flag the command does not have cannot be passed.
func TestConfirmOfferedWhenABranchIsDestructive(t *testing.T) {
	src := source(t)
	if !regexp.MustCompile("Confirm\\s+bool\\s+`name:\"confirm\"").MatchString(src) {
		t.Errorf("a verb with a destructive branch must offer --confirm:\n%s", src)
	}
	if !strings.Contains(src, "given, deref(c.Child), c.DryRun, c.Confirm, false)") {
		t.Errorf("Run must pass c.Confirm through:\n%s", src)
	}
}

// Codex #71-6: a verb whose select branch targets a device offers --allow-unpaired (and the
// UNPAIRED help line), because the engine applies the selected device contract.
func TestAllowUnpairedOfferedWhenABranchTargetsDevice(t *testing.T) {
	const fx = `{"name":"t","base_url":"https://h","areas":{"a":"help"},"entities":{"t":{"id_field":"","operations":{
	  "base":{"method":"GET","path":"/b",
	    "cli":{"area":"a","verb":"v","priority":"core","target":"child","summary":"s",
	      "select":[{"when":"flag:x","op":"t.dev","target":"device"}],
	      "flags":[{"name":"x","type":"string","maps_to":"filter:role","help":"h"}]}},
	  "dev":{"method":"GET","path":"/d"}}}}}`
	d, err := descriptor.Parse([]byte(fx))
	if err != nil {
		t.Fatalf("fixture must validate: %v", err)
	}
	src, err := Source(d)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`AllowUnpaired\s+bool`).MatchString(string(src)) || !strings.Contains(string(src), "c.AllowUnpaired)") {
		t.Errorf("a device branch must offer --allow-unpaired and pass it through:\n%s", src)
	}
	if !strings.Contains(string(src), "An UNPAIRED target is refused") {
		t.Errorf("the verb help must state the UNPAIRED rule:\n%s", src)
	}
}
