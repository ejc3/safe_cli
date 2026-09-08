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
      "select":[{"when":"flag:sel","op":"t.del"}],
      "flags":[{"name":"sel","type":"string","maps_to":"filter:role","help":"h"},{"name":"x","type":"string","required":true,"maps_to":"query:q","help":"h"},
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

// Closure batch (verification sweep, generator): help renders an int default as an integer,
// still states a default when the help merely mentions the word, escapes kong's ${...}
// interpolation, and says a list flag repeats.
func TestFlagHelpRendering(t *testing.T) {
	const fx = `{"name":"t","base_url":"https://h","areas":{"a":"help"},"entities":{"t":{"id_field":"","operations":{
	  "base":{"method":"GET","path":"/b","query":["limit","zone","tpl","ids"],
	    "cli":{"area":"a","verb":"v","priority":"core","target":"account","summary":"s",
	      "flags":[{"name":"limit","type":"int","default":1000000,"maps_to":"query:limit","help":"How many."},
	               {"name":"zone","type":"string","default":"UTC","maps_to":"query:zone","help":"Uses the default zone unless set."},
	               {"name":"tpl","type":"string","maps_to":"query:tpl","help":"A literal ${name} placeholder."},
	               {"name":"ids","type":"list","maps_to":"query:ids","help":"Ids."}]}}}}}}`
	d, err := descriptor.Parse([]byte(fx))
	if err != nil {
		t.Fatalf("fixture must validate: %v", err)
	}
	src, err := Source(d)
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for _, want := range []string{"(default: 1000000)", "Uses the default zone unless set. (default: UTC)", "$${name}", "Ids. Repeatable"} {
		if !strings.Contains(s, want) {
			t.Errorf("generated help lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "1e+06") {
		t.Errorf("an int default must not render in exponent notation:\n%s", s)
	}
}

// --child is required to kong only when every contract of the verb needs it: a child base
// with a branch that overrides the target to account/self must leave it optional and let
// the engine enforce it for the branches that do (Codex #71 round 6).
func TestChildOptionalWhenABranchDropsTheTarget(t *testing.T) {
	const fx = `{"name":"t","base_url":"https://h","areas":{"a":"help"},"entities":{"t":{"id_field":"","operations":{
	  "base":{"method":"GET","path":"/b","headers":["x-fp-identifier-target-serviceid"],
	    "cli":{"area":"a","verb":"v","priority":"core","target":"child","summary":"s",
	      "select":[{"when":"flag:all","op":"t.acct","target":"account"}],
	      "flags":[{"name":"all","type":"string","maps_to":"filter:role","help":"h"}]}},
	  "acct":{"method":"GET","path":"/acct","headers":["x-fp-identifier-target-serviceid"]}}}}}`
	d, err := descriptor.Parse([]byte(fx))
	if err != nil {
		t.Fatalf("fixture must validate: %v", err)
	}
	src, err := Source(d)
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`Child\s+\*string\s+` + "`" + `name:"child"[^` + "`" + `]*required:""`).MatchString(string(src)) {
		t.Errorf("--child must not be required to kong when a branch drops the child target:\n%s", src)
	}
}
