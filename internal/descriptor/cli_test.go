package descriptor

import (
	"strings"
	"testing"
)

// cliFixture wraps one op (with its cli block) in a minimal descriptor. extra is spliced
// into the op alongside the cli block (e.g. "query", "required_query", "takes_body").
func cliFixture(cli string, extra string) []byte {
	if extra != "" {
		extra = "," + extra
	}
	return []byte(`{"name":"t","base_url":"https://h","entities":{"pause":{"id_field":"","operations":{
	  "pauseIt":{"method":"POST","path":"/p","takes_body":true` + extra + `,"cli":` + cli + `},
	  "other":{"method":"GET","path":"/o"}}}}}`)
}

// A complete, valid verb — the pause-internet `pause` shape from docs/CLI-DESIGN.md §4,
// written in the strict-JSON template form ("$var" strings).
const validPause = `{
  "area":"pause-internet","verb":"pause","priority":"core","target":"device",
  "summary":"Pause the child's internet now.",
  "body_template":"{\"timeZone\":\"$tz\",\"profiles\":[{\"profileId\":\"$child.profileId\",\"devices\":[{\"serviceId\":\"$child.serviceId\",\"deviceId\":\"$child.deviceId\",\"pauseSchedule\":\"$for?\",\"untilIUnpause\":\"$indefinite\",\"callOnlyMode\":\"$callOnly\"}]}]}",
  "flags":[
    {"name":"for","type":"enum","enum":["30m","1h","2h","4h","until-morning"],"default":"30m","maps_to":"body:$for","transform":"pause_schedule","help":"How long to pause (default: 30m)."},
    {"name":"indefinite","type":"bool","default":false,"maps_to":"body:$indefinite","excludes":["for"],"nulls":["for"],"help":"Pause until resume; omits pauseSchedule."},
    {"name":"call-only","type":"bool","default":false,"maps_to":"body:$callOnly","help":"Keep calls working."},
    {"name":"timezone","type":"tz","default":"$account.timezone","maps_to":"body:$tz","transform":"tz_short","help":"Short zone code (default: account timezone)."}
  ],
  "resolve":["$child.profileId","$child.serviceId","$child.deviceId","$account.timezone"]
}`

func TestCLIValidVerbParses(t *testing.T) {
	if _, err := Parse(cliFixture(validPause, "")); err != nil {
		t.Fatalf("a valid verb must parse: %v", err)
	}
}

// Each case is one validation rule; the fixture violates exactly that rule, and Parse must
// refuse with a message naming it. Observed RED with validateCLIBlocks not hooked into
// Parse (every case parsed cleanly), GREEN once hooked.
func TestCLIValidationRejects(t *testing.T) {
	cases := []struct {
		name, cli, extra, want string
	}{
		{"two shapes", `{"area":"a","verb":"v","call_only":true,"reason":"x"}`, "", "exactly one of"},
		{"alias to missing op", `{"alias_of":"pause.nope"}`, "", "does not name an existing"},
		{"call_only without reason", `{"call_only":true}`, "", "needs a reason"},
		{"bad priority", `{"area":"a","verb":"v","priority":"urgent","target":"child","summary":"s"}`, "", "priority"},
		{"bad target", `{"area":"a","verb":"v","priority":"core","target":"kid","summary":"s"}`, "", "target"},
		{"no summary", `{"area":"a","verb":"v","priority":"core","target":"child"}`, "", "summary"},
		{"unknown flag type", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"blob","maps_to":"body:$x","help":"h"}]}`, "", "not a known flag type"},
		{"unknown transform", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","maps_to":"body:$x","transform":"yolo","help":"h"}]}`, "", "registry"},
		{"flag without help", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","maps_to":"body:$x"}]}`, "", "help"},
		{"query flag not declared on op", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","flags":[{"name":"q","type":"string","maps_to":"query:nope","help":"h"}]}`, `"takes_body":false`, "not one of the op's declared query params"},
		// Codex #66-2: a required query param must have a source.
		{"required query with no source", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s"}`, `"takes_body":false,"query":["startDate"],"required_query":["startDate"]`, "has no source"},
		{"query constant not declared on op", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","query":{"nope":"v6"}}`, `"takes_body":false`, "not one of the op's declared query params"},
		{"query constant and flag both", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","query":{"q":"1"},"flags":[{"name":"q","type":"string","maps_to":"query:q","help":"h"}]}`, `"takes_body":false,"query":["q"]`, "also mapped from a flag"},
		{"body op without template", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s"}`, "", "no body_template"},
		{"template not strict JSON", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":$x}"}`, "", "strict JSON"},
		// Codex #66-3: an unclassified example field is rejected, not kept.
		{"unclassified literal field", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"geofenceId\":123,\"x\":\"$x\"}","flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"}]}`, "", "unclassified example value"},
		{"unclassified array field", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"days\":[\"Mon\"],\"x\":\"$x\"}","flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"}]}`, "", "unclassified example value"},
		{"unknown var", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$ghost\"}"}`, "", "neither a flag"},
		{"resolved var not listed", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"p\":\"$child.profileId\"}"}`, "", "not listed in resolve"},
		{"flag var unused by template", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"},{"name":"y","type":"string","maps_to":"body:$y","help":"h"}]}`, "", "never uses"},
		// Codex #66-1: "?" (omission) must be backed by a flag that can actually be unset.
		{"optional var on a required flag nothing nulls", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x?\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "nothing nulls it"},
		{"optional resolved var", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"p\":\"$child.profileId?\"}","resolve":["$child.profileId"]}`, "", "cannot be optional"},
		{"nulls unknown flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","maps_to":"body:$x","nulls":["ghost"],"help":"h"}]}`, "", "unknown flag"},
		// Codex #66 re-review: an exclusion against a defaulted flag is undecidable (the
		// default makes the flag always present), so it is rejected; precedence is nulls.
		{"excludes names a defaulted flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\",\"y\":\"$y\"}","flags":[{"name":"x","type":"string","default":"d","maps_to":"body:$x","help":"h"},{"name":"y","type":"bool","default":false,"maps_to":"body:$y","excludes":["x"],"help":"h"}]}`, "", "has a default"},
		// Codex #66 re-review: verb-level at_least_one (account set --family-name|--timezone).
		{"at_least_one with a single flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","at_least_one":["x"],"flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"}]}`, "", "at least two"},
		{"at_least_one names unknown flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","at_least_one":["x","ghost"],"flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"}]}`, "", "unknown flag"},
		{"at_least_one names a required flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\",\"y\":\"$y\"}","at_least_one":["x","y"],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"y","type":"string","maps_to":"body:$y","help":"h"}]}`, "", "already required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(cliFixture(tc.cli, tc.extra))
			if err == nil {
				t.Fatalf("Parse accepted a cli block that violates %q", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error should name the rule (%q), got: %v", tc.want, err)
			}
		})
	}
}

// Constants are the ONLY way a literal example field survives into a template — and then
// by name, so the classification is explicit (Codex #66-3).
func TestCLIDeclaredConstantIsAccepted(t *testing.T) {
	cli := `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s",
	  "body_template":"{\"MAPPVersion\":\"8.1\",\"days\":[\"Mon\"],\"x\":\"$x\"}",
	  "constants":{"MAPPVersion":"8.1","days":["Mon"]},
	  "flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"}]}`
	if _, err := Parse(cliFixture(cli, "")); err != nil {
		t.Fatalf("declared constants must be accepted: %v", err)
	}
}

// The two shapes the re-review rules are FOR must still parse: precedence over a defaulted
// flag via nulls (no excludes), and an optional pair guarded by at_least_one.
func TestCLINullsPrecedenceAndAtLeastOneAccepted(t *testing.T) {
	cli := `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s",
	  "body_template":"{\"x\":\"$x?\",\"y\":\"$y\",\"name\":\"$name?\",\"tz\":\"$tz?\"}",
	  "at_least_one":["name","tz"],
	  "flags":[
	    {"name":"x","type":"enum","enum":["a","b"],"default":"a","maps_to":"body:$x","help":"h"},
	    {"name":"y","type":"bool","default":false,"nulls":["x"],"maps_to":"body:$y","help":"h"},
	    {"name":"name","type":"string","maps_to":"body:$name","help":"h"},
	    {"name":"tz","type":"tz","maps_to":"body:$tz","help":"h"}]}`
	if _, err := Parse(cliFixture(cli, "")); err != nil {
		t.Fatalf("nulls precedence + at_least_one must be accepted: %v", err)
	}
}

// The embedded descriptor carries the first real cli blocks: pause-internet's three verbs,
// exactly as docs/CLI-DESIGN.md §5/§6 specify them.
func TestEmbeddedPauseInternetCLI(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	e, _ := d.Entity("pause_internet")
	want := map[string]struct{ verb, target, priority string }{
		"getDevices":      {"status", "child", "core"}, // a read: needs --child, no pairing guard
		"pauseInternet":   {"pause", "device", "core"},
		"unPauseInternet": {"resume", "device", "core"},
	}
	for op, w := range want {
		c := e.Operations[op].CLI
		if c == nil {
			t.Errorf("pause_internet.%s has no cli block", op)
			continue
		}
		if c.Area != "pause-internet" || c.Verb != w.verb || c.Target != w.target || c.Priority != w.priority {
			t.Errorf("pause_internet.%s cli = area=%q verb=%q target=%q priority=%q, want pause-internet/%s/%s/%s",
				op, c.Area, c.Verb, c.Target, c.Priority, w.verb, w.target, w.priority)
		}
	}
	// pause: --indefinite must null --for so "$for?" is omitted (Codex #66-1).
	p := e.Operations["pauseInternet"].CLI
	var indef *Flag
	for i := range p.Flags {
		if p.Flags[i].Name == "indefinite" {
			indef = &p.Flags[i]
		}
	}
	if indef == nil || !contains(indef.Nulls, "for") {
		t.Errorf("pause --indefinite must declare nulls: [for]; got %+v", indef)
	}
	if !strings.Contains(p.BodyTemplate, `"$for?"`) {
		t.Errorf("pause body_template must mark pauseSchedule as the optional \"$for?\"; got %s", p.BodyTemplate)
	}
}
