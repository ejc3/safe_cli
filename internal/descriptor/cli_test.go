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
	  "other":{"method":"GET","path":"/o"},
	  "other2":{"method":"POST","path":"/o2","takes_body":true,"query":["lat","lon","address","alt"],"headers":["timezone"]},
	  "twin":{"method":"POST","path":"/p","takes_body":true,"cli":{"area":"tw","verb":"in","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"}]}}}}}}`)
}

// A complete, valid verb — the pause-internet `pause` shape from docs/CLI-DESIGN.md §4,
// written in the strict-JSON template form ("$var" strings).
const validPause = `{
  "area":"pause-internet","verb":"pause","priority":"core","target":"device",
  "summary":"Pause the child's internet now.",
  "body_template":"{\"timeZone\":\"$tz\",\"profiles\":[{\"profileId\":\"$child.profileId\",\"devices\":[{\"serviceId\":\"$child.serviceId\",\"deviceId\":\"$child.deviceId\",\"pauseSchedule\":\"$for?\",\"untilIUnpause\":\"$indefinite\",\"callOnlyMode\":\"$callOnly\"}]}]}",
  "flags":[
    {"name":"for","type":"enum","enum":["30m","1h","2h","4h","until-morning"],"default":"30m","maps_to":"body:$for","transform":"pause_schedule","help":"How long to pause (default: 30m)."},
    {"name":"indefinite","type":"bool","default":false,"maps_to":"body:$indefinite","nulls":["for"],"help":"Pause until resume; omits pauseSchedule."},
    {"name":"call-only","type":"bool","default":false,"maps_to":"body:$callOnly","help":"Keep calls working."},
    {"name":"timezone","type":"tz","default":"$local.timezone","maps_to":"body:$tz","transform":"tz_short","help":"Short zone code (default: local timezone)."}
  ],
  "resolve":["$child.profileId","$child.serviceId","$child.deviceId","$local.timezone"]
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
		// Codex #67: two flags competing for one template value must fail at Parse, not be
		// decided by map iteration or engine precedence.
		{"duplicate body var mapping", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"},{"name":"y","type":"string","maps_to":"body:$x","help":"h"}]}`, "", "already mapped"},
		// Codex #67: a path mapping must name a real {placeholder} of the op's path.
		{"path flag to unknown placeholder", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","flags":[{"name":"device-id","type":"string","maps_to":"path:device-id","help":"h"}]}`, `"takes_body":false,"path":"/d/{deviceId}"`, "not a {placeholder}"},
		// Codex #67: a template on an op that declares no body is a classification typo.
		{"template on a body-less op", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"}]}`, `"takes_body":false`, "declares no body"},
		// Codex #67: resolved variables are an exact vocabulary, not a prefix match.
		{"resolved var typo", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"p\":\"$child.profielId\"}","resolve":["$child.profielId"]}`, "", "not a supported resolved variable"},
		{"uuid typo", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"e\":\"$uuidTypo\"}","resolve":["$uuidTypo"]}`, "", "not a supported resolved variable"},
		{"malformed lookup", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"n\":\"$lookup:nope\"}","resolve":["$lookup:nope"]}`, "", "malformed $lookup"},
		{"lookup to missing op", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"n\":\"$lookup:pause.nope:id=x:name\",\"x\":\"$x\"}","resolve":["$lookup:pause.nope:id=x:name"],"flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"}]}`, "", "does not name an existing"},
		{"lookup keyed by unknown flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"n\":\"$lookup:pause.other:id=ghost:name\"}","resolve":["$lookup:pause.other:id=ghost:name"]}`, "", "unknown flag"},
		// Codex #66 round 3: conditional op selection is declared, and everything it names is
		// checked — the op, the flag, the lookup, and the condition vocabulary itself.
		{"select op missing", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"flag:x","op":"pause.nope"}],"flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"}]}`, "", "does not name an existing"},
		{"select unknown condition", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"moon","op":"pause.other"}],"flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"}]}`, "", "condition"},
		{"select flag not declared", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"flag:ghost","op":"pause.other"}],"flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"}]}`, "", "unknown flag"},
		{"select malformed lookup", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"exists:$lookup:nope","op":"pause.other"}],"flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"}]}`, "", "malformed $lookup"},
		// Codex #66 round 3: one flag, several fields — only via a structured transform.
		{"spreads_to with maps_to too", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"blockContent\":\"$blockContent\",\"alertOn\":\"$alertOn\"}","flags":[{"name":"mode","type":"enum","enum":["block","alert"],"maps_to":"body:$blockContent","spreads_to":["body:$blockContent","body:$alertOn"],"transform":"mode_block_alert","help":"h"}]}`, "", "exactly one of maps_to or spreads_to"},
		{"spreads_to without structured transform", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"blockContent\":\"$blockContent\",\"alertOn\":\"$alertOn\"}","flags":[{"name":"mode","type":"enum","enum":["block","alert"],"spreads_to":["body:$blockContent","body:$alertOn"],"help":"h"}]}`, "", "structured transform"},
		{"spreads_to destination unused by template", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"blockContent\":\"$blockContent\"}","flags":[{"name":"mode","type":"enum","enum":["block","alert"],"spreads_to":["body:$blockContent","body:$alertOn"],"transform":"mode_block_alert","help":"h"}]}`, "", "never uses"},
		// Codex #66 round 3: a repeatable flag's var must live in a single-element array.
		{"repeatable var outside an array", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"url\":\"$url\"}","flags":[{"name":"url","type":"string","repeatable":true,"maps_to":"body:$url","help":"h"}]}`, "", "single-element array"},
		// Codex #67 (7a798ff): a typo in a cli key must not be silently discarded.
		{"unknown cli key", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","maps_to":"body:$x","xform":"tz_short","help":"h"}]}`, "", "unknown field"},
		// resolve entries are checked on bodyless verbs too.
		{"bodyless verb with bad resolve", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","resolve":["$child.profielId"]}`, `"takes_body":false`, "not a supported resolved variable"},
		// nulls targets must be optional body vars — the only place omission is defined.
		{"nulls a query flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"y\":\"$y\"}","flags":[{"name":"q","type":"string","maps_to":"query:q","help":"h"},{"name":"y","type":"bool","default":false,"nulls":["q"],"maps_to":"body:$y","help":"h"}]}`, `"query":["q"]`, "optional body variable"},
		{"nulls a required body var", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\",\"y\":\"$y\"}","flags":[{"name":"x","type":"string","default":"d","maps_to":"body:$x","help":"h"},{"name":"y","type":"bool","default":false,"nulls":["x"],"maps_to":"body:$y","help":"h"}]}`, "", "optional body variable"},
		// a retained literal must EQUAL its declared constant, not merely share the key.
		{"constant value mismatch", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"MAPPVersion\":\"wrong\",\"x\":\"$x\"}","constants":{"MAPPVersion":"8.1"},"flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"}]}`, "", "does not match the declared constant"},
		// one query parameter, one source.
		{"two flags to one query param", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","flags":[{"name":"a","type":"string","maps_to":"query:q","help":"h"},{"name":"b","type":"string","maps_to":"query:q","help":"h"}]}`, `"takes_body":false,"query":["q"]`, "already mapped"},
		// two repeatables in one expansion element have no defined expansion.
		{"two repeatables in one element", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"items\":[{\"a\":\"$a\",\"b\":\"$b\"}]}","flags":[{"name":"a","type":"string","repeatable":true,"maps_to":"body:$a","help":"h"},{"name":"b","type":"string","repeatable":true,"maps_to":"body:$b","help":"h"}]}`, "", "second repeatable"},
		// a select branch is validated against ITS op's contract: a bodyless GET cannot inherit a body.
		{"select branch to a bodyless op inherits a body", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"flag:x","op":"pause.other"}],"flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"}]}`, "", "declares no body"},
		// dependent flag groups name declared flags and need real alternatives.
		{"requires unknown flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","requires":["ghost"],"maps_to":"body:$x","help":"h"}]}`, "", "unknown flag"},
		{"one_of with one group", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","one_of":[["x"]],"flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"}]}`, "", "at least two"},
		{"one_of unknown flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","one_of":[["x"],["ghost"]],"flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"}]}`, "", "unknown flag"},
		// Codex #66 round 5: one op, several verbs — `cli` may be a list; alias/call_only stay single.
		{"cli list mixing alias and verb", `[{"alias_of":"pause.other"},{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"}]}]`, "", "only entry"},
		{"cli list duplicate verb", `[{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"}]},{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"}]}]`, "", "declared twice"},
		// Codex #66 round 5: fixed header constants must name headers the op declares.
		{"header constant not declared on op", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","headers":{"x-nope":"1"},"flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"}]}`, "", "not one of the op's declared headers"},
		// Codex #67: a structured transform fills a FIXED set of vars; spreads_to must match it.
		{"spreads_to wrong arity", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"a\":\"$blockContent\"}","flags":[{"name":"mode","type":"enum","enum":["block","alert"],"spreads_to":["body:$blockContent"],"transform":"mode_block_alert","help":"h"}]}`, "", "must spread to exactly"},
		{"spreads_to wrong names", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"a\":\"$a\",\"b\":\"$b\"}","flags":[{"name":"mode","type":"enum","enum":["block","alert"],"spreads_to":["body:$a","body:$b"],"transform":"mode_block_alert","help":"h"}]}`, "", "must spread to exactly"},
		// Codex #67 (round on 71d0426): four more load-time rules.
		{"command path declared by two ops", `{"area":"tw","verb":"in","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"}]}`, "", "declared by both"},
		{"alias to a different route", `{"alias_of":"pause.other"}`, "", "does not share method and path"},
		{"alias to an op with no verb", `{"alias_of":"pause.other2"}`, `"method":"POST","path":"/o2"`, "canonical verb"},
		{"two flags to one header", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","flags":[{"name":"a","type":"string","maps_to":"header:x-h","help":"h"},{"name":"b","type":"string","maps_to":"header:x-h","help":"h"}]}`, `"takes_body":false`, "already mapped"},
		{"two flags to one placeholder", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","flags":[{"name":"a","type":"string","maps_to":"path:thing","help":"h"},{"name":"b","type":"string","maps_to":"path:thing","help":"h"}]}`, `"takes_body":false,"path":"/x/{thing}"`, "already mapped"},
		{"enum default not in list", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"enum","enum":["a","b"],"default":"zzz","maps_to":"body:$x","help":"h"}]}`, "", "default"},
		{"int flag with string default", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"int","default":"seven","maps_to":"body:$x","help":"h"}]}`, "", "default"},
		{"bool flag with string default", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"bool","default":"yes","maps_to":"body:$x","help":"h"}]}`, "", "default"},
		// The design's eight mechanisms (docs/CLI-DESIGN.md §4, the expressiveness sweep).
		{"query map with a mistyped resolver var", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","query":{"lat":"$child.profielId"},"resolve":["$child.profileId"]}`, `"takes_body":false,"query":["lat"]`, "not a supported resolved variable"},
		{"query map resolver var not in resolve", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","query":{"lat":"$child.profileId"}}`, `"takes_body":false,"query":["lat"]`, "not listed in resolve"},
		{"header map with a mistyped resolver var", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","headers":{"timezone":"$ghost.zone"}}`, `"takes_body":false,"headers":["timezone"]`, "not a supported resolved variable"},
		{"unkeyed lookup without a field", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"n\":\"$lookup:pause.other::\"}","resolve":["$lookup:pause.other::"]}`, "", "malformed $lookup"},
		{"default that is an unknown resolver var", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","default":"$ghost.value","maps_to":"body:$x","help":"h"}]}`, "", "not a supported resolved variable"},
		{"placeholder with no source", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s"}`, `"takes_body":false,"path":"/d/{deviceId}/{thing}"`, "{thing} has no source"},
		{"target id placeholder on an account verb", `{"area":"a","verb":"v","priority":"core","target":"account","summary":"s"}`, `"takes_body":false,"path":"/d/{deviceId}"`, "{deviceId} has no source"},
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

// The shapes the #67 rules must NOT reject: every exact resolved name, a well-formed
// $lookup keyed by a declared flag against an existing op, and a path mapping to a real
// placeholder.
func TestCLIExactResolveLookupAndPathAccepted(t *testing.T) {
	cli := `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s",
	  "body_template":"{\"a\":\"$child.serviceId\",\"b\":\"$child.profileId\",\"c\":\"$child.deviceId\",\"d\":\"$child.pairing\",\"e\":\"$self.serviceId\",\"f\":\"$self.profileId\",\"g\":\"$account.id\",\"h\":\"$local.timezone\",\"i\":\"$now.epochMs\",\"j\":\"$uuid\",\"k\":\"$lookup:pause.other:id=cat:name\",\"cat\":\"$cat\"}",
	  "resolve":["$child.serviceId","$child.profileId","$child.deviceId","$child.pairing","$self.serviceId","$self.profileId","$account.id","$local.timezone","$now.epochMs","$uuid","$lookup:pause.other:id=cat:name"],
	  "flags":[{"name":"cat","type":"int","maps_to":"body:$cat","help":"h"},{"name":"device-id","type":"string","maps_to":"path:deviceId","help":"h"}]}`
	if _, err := Parse(cliFixture(cli, `"path":"/d/{deviceId}"`)); err != nil {
		t.Fatalf("exact resolve names, a valid $lookup, and a real path placeholder must be accepted: %v", err)
	}
}

// The three round-3 mechanisms in their valid shapes must parse: a select list over an
// existing op keyed by a declared flag and a valid lookup; a spreads_to pair through the
// structured mode_block_alert transform; a repeatable var inside a single-element array.
func TestCLISelectSpreadsAndRepeatableAccepted(t *testing.T) {
	cli := `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s",
	  "body_template":"{\"blockContent\":\"$blockContent\",\"alertOn\":\"$alertOn\",\"domains\":[{\"status\":\"$status\",\"url\":\"$url\"}],\"n\":\"$n?\"}",
	  "select":[{"when":"flag:n","op":"pause.other2"},{"when":"exists:$lookup:pause.other:id=n:name","op":"pause.other2"},{"when":"child","op":"pause.other2"}],
	  "flags":[
	    {"name":"mode","type":"enum","enum":["block","alert"],"default":"block","spreads_to":["body:$blockContent","body:$alertOn"],"transform":"mode_block_alert","help":"h"},
	    {"name":"status","type":"enum","enum":["allow","block"],"default":"block","maps_to":"body:$status","transform":"allow_block_ab","help":"h"},
	    {"name":"url","type":"string","repeatable":true,"required":true,"maps_to":"body:$url","help":"h"},
	    {"name":"n","type":"string","maps_to":"body:$n","help":"h"}]}`
	if _, err := Parse(cliFixture(cli, "")); err != nil {
		t.Fatalf("select + spreads_to + repeatable-in-array must be accepted: %v", err)
	}
}

// A select branch may carry its own contract: here the child branch overrides the target,
// the body template and the resolve list for a different body-bearing op, and validates
// against that op; requires/one_of name declared flags.
func TestCLISelectOverridesAndDependentFlagsAccepted(t *testing.T) {
	cli := `{"area":"alerts","verb":"set","priority":"core","target":"account","summary":"s",
	  "body_template":"{\"newContactAlertV2\":\"$contact\"}",
	  "select":[{"when":"child","op":"pause.other2","target":"child","body_template":"{\"newContactAlert\":\"$contact\",\"profileId\":\"$child.profileId\"}","resolve":["$child.profileId"]}],
	  "one_of":[["lat","lon"],["address"]],
	  "flags":[
	    {"name":"contact","type":"bool","default":false,"maps_to":"body:$contact","help":"h"},
	    {"name":"lat","type":"float","requires":["lon"],"maps_to":"query:lat","help":"h"},
	    {"name":"lon","type":"float","requires":["lat"],"maps_to":"query:lon","help":"h"},
	    {"name":"address","type":"string","maps_to":"query:address","help":"h"}]}`
	if _, err := Parse(cliFixture(cli, `"query":["lat","lon","address"]`)); err != nil {
		t.Fatalf("select overrides + requires/one_of must be accepted: %v", err)
	}
}

// One operation backing two verbs: sibling blocks share the op's templates and differ by a
// constant (filter block/allow both drive updateSubcategory with enabled true/false). A
// verb may also declare fixed header constants the op declares.
func TestCLIListOfVerbsAndHeadersAccepted(t *testing.T) {
	block := func(verb string, enabled bool) string {
		b := map[bool]string{true: "true", false: "false"}[enabled]
		return `{"area":"filter","verb":"` + verb + `","priority":"core","target":"child","summary":"s",
		  "body_template":"{\"enabled\":` + b + `,\"id\":\"$id\"}",
		  "constants":{"enabled":` + b + `},
		  "headers":{"x-pending-activation":"false"},
		  "flags":[{"name":"id","type":"int","required":true,"maps_to":"body:$id","help":"h"}]}`
	}
	cli := "[" + block("block", true) + "," + block("allow", false) + "]"
	d, err := Parse(cliFixture(cli, `"headers":["x-pending-activation"]`))
	if err != nil {
		t.Fatalf("a list of verb blocks with header constants must parse: %v", err)
	}
	op := d.Entities["pause"].Operations["pauseIt"]
	if len(op.CLI) != 2 || op.CLI[0].Verb != "block" || op.CLI[1].Verb != "allow" {
		t.Fatalf("want two verb blocks block/allow, got %+v", op.CLI)
	}
	// The single-object form still parses to a one-element list.
	d, err = Parse(cliFixture(validPause, ""))
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Entities["pause"].Operations["pauseIt"].CLI; len(got) != 1 || got[0].Verb != "pause" {
		t.Errorf("single-object cli must be a one-element list, got %+v", got)
	}
}

// The eight mechanisms in their valid shapes: a resolver var as a query value, a query flag
// declared only by a select branch's op, a resolver var as a header value, an unkeyed
// lookup (also as a flag default and an exists: condition), a filter: destination, two
// flags sharing a body var under mutual excludes, and target-id placeholders on a child verb.
func TestCLIExpressivenessSweepAccepted(t *testing.T) {
	cli := `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s",
	  "body_template":"{\"mon\":\"$mon?\",\"name\":\"$name\",\"limitId\":\"$lookup:pause.other::screenTimeLimitId\"}",
	  "query":{"lat":"$child.profileId"},
	  "headers":{"timezone":"$local.timezone"},
	  "resolve":["$child.profileId","$local.timezone","$lookup:pause.other::screenTimeLimitId"],
	  "select":[{"when":"exists:$lookup:pause.other::screenTimeLimitId","op":"pause.other2"},{"when":"flag:alt","op":"pause.other2"}],
	  "flags":[
	    {"name":"weekdays","type":"int","excludes":["mon"],"maps_to":"body:$mon","help":"h"},
	    {"name":"mon","type":"int","excludes":["weekdays"],"maps_to":"body:$mon","help":"h"},
	    {"name":"name","type":"string","default":"$lookup:pause.other::familyName","maps_to":"body:$name","help":"h"},
	    {"name":"alt","type":"string","maps_to":"query:alt","help":"h"},
	    {"name":"only","type":"string","maps_to":"filter:role","help":"h"}]}`
	if _, err := Parse(cliFixture(cli, `"query":["lat"],"headers":["timezone"],"path":"/d/{deviceId}/{profileId}"`)); err != nil {
		t.Fatalf("the sweep's valid shapes must parse: %v", err)
	}
}

// alias_of must name a verb-bearing op on the SAME method and path: "twin" shares POST /p
// with pauseIt, so pauseIt may alias it.
func TestCLIAliasToDuplicateRouteAccepted(t *testing.T) {
	if _, err := Parse(cliFixture(`{"alias_of":"pause.twin"}`, "")); err != nil {
		t.Fatalf("alias to a same-route verb op must parse: %v", err)
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
		blocks := e.Operations[op].CLI
		if len(blocks) == 0 {
			t.Errorf("pause_internet.%s has no cli block", op)
			continue
		}
		c := blocks[0]
		if c.Area != "pause-internet" || c.Verb != w.verb || c.Target != w.target || c.Priority != w.priority {
			t.Errorf("pause_internet.%s cli = area=%q verb=%q target=%q priority=%q, want pause-internet/%s/%s/%s",
				op, c.Area, c.Verb, c.Target, c.Priority, w.verb, w.target, w.priority)
		}
	}
	// pause: --indefinite must null --for so "$for?" is omitted (Codex #66-1).
	p := e.Operations["pauseInternet"].CLI[0]
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
