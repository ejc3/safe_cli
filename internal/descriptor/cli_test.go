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
	  "purge":{"method":"GET","path":"/purge","destructive":true},
	  "withph":{"method":"GET","path":"/w/{id}"},
	  "reqq":{"method":"GET","path":"/r","query":["a"],"required_query":["a"]},
	  "gone":{"method":"GET","path":"/g","unavailable":"observed 403"},
	  "mp":{"method":"POST","path":"/mp","takes_body":true,"multipart":true},
	  "goneverb":{"method":"POST","path":"/p","takes_body":true,"unavailable":"observed 403","cli":{"area":"gv","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}},
	  "twin":{"method":"POST","path":"/p","takes_body":true,"cli":{"area":"tw","verb":"in","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}}}}}}`)
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

// Codex #70-1 (positive): the branch-only query flag is fine when it is the branch's own
// flag: condition, when it requires the flag that is, or when the branch is the child one
// (the engine names the missing selector at run time).
func TestCLIBranchOnlyQueryFlagReachableAccepted(t *testing.T) {
	for name, cli := range map[string]string{
		"own selector":          `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"flag:alt","op":"pause.other2"}],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"alt","type":"string","maps_to":"query:alt","help":"h"}]}`,
		"requires the selector": `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"flag:sel","op":"pause.other2"}],"flags":[{"name":"sel","type":"string","maps_to":"filter:role","help":"h"},{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"alt","type":"string","requires":["sel"],"maps_to":"query:alt","help":"h"}]}`,
		"child branch":          `{"area":"a","verb":"v","priority":"core","target":"account","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"child","op":"pause.other2","target":"child"}],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"alt","type":"string","maps_to":"query:alt","help":"h"}]}`,
	} {
		if _, err := Parse(cliFixture(cli, "")); err != nil {
			t.Errorf("%s: a reachable branch-only query flag must be accepted: %v", name, err)
		}
	}
}

// Round 3 (positive): shapes the new rules must keep accepting — at_least_one over
// lookup-defaulted flags (account set resends the untouched field) and resolver defaults of
// the matching type.
func TestCLIRoundThreePositives(t *testing.T) {
	for name, c := range map[string]struct{ cli, extra string }{
		"at_least_one over lookup defaults": {`{"area":"a","verb":"v","priority":"core","target":"account","summary":"s","body_template":"{\"familyName\":\"$fam\",\"tz\":\"$tz\"}","at_least_one":["family-name","timezone"],"resolve":["$lookup:pause.other::familyName","$lookup:pause.other::timeZone"],"flags":[{"name":"family-name","type":"string","default":"$lookup:pause.other::familyName","maps_to":"body:$fam","help":"h"},{"name":"timezone","type":"string","default":"$lookup:pause.other::timeZone","maps_to":"body:$tz","help":"h"}]}`, ""},
		"typed resolver defaults":           {`{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"a\":\"$a\",\"b\":\"$b\",\"c\":\"$c\"}","resolve":["$local.timezone","$lookup:pause.other::n"],"flags":[{"name":"a","type":"tz","default":"$local.timezone","maps_to":"body:$a","help":"h"},{"name":"b","type":"int","default":"$lookup:pause.other::n","maps_to":"body:$b","help":"h"},{"name":"c","type":"string","default":"$local.timezone","maps_to":"body:$c","help":"h"}]}`, ""},
	} {
		if _, err := Parse(cliFixture(c.cli, c.extra)); err != nil {
			t.Errorf("%s: must be accepted: %v", name, err)
		}
	}
}

// Codex #70-5 (positive): a header flag is fine on the op's own header, and on a branch
// op's header when the flag is that branch's selector.
func TestCLIHeaderFlagAccepted(t *testing.T) {
	for name, cli := range map[string]string{
		"own header":      `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"tz","type":"string","maps_to":"header:timezone","help":"h"}]}`,
		"branch selector": `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"flag:tz","op":"pause.other2"}],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"tz","type":"string","maps_to":"header:timezone","help":"h"}]}`,
	} {
		extra := ""
		if name == "own header" {
			extra = `"headers":["timezone"]`
		}
		if _, err := Parse(cliFixture(cli, extra)); err != nil {
			t.Errorf("%s: must be accepted: %v", name, err)
		}
	}
}

// Codex #70 round 7 (positive): a keyed lookup whose key flag is optional is fine when the
// lookup lives only in the branch that flag selects, and an exists: condition may be keyed
// by an optional flag (the engine treats its absence as "not found").
func TestCLIOptionalLookupKeyConfinedToItsBranch(t *testing.T) {
	cli := `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}",
	  "select":[{"when":"flag:cat","op":"pause.other2","body_template":"{\"x\":\"$x\",\"n\":\"$lookup:pause.other:id=cat:name\"}","resolve":["$lookup:pause.other:id=cat:name"]},
	            {"when":"exists:$lookup:pause.other:id=n:name","op":"pause.other2"}],
	  "flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"cat","type":"int","maps_to":"filter:role","help":"h"},{"name":"n","type":"string","maps_to":"filter:role","help":"h"}]}`
	if _, err := Parse(cliFixture(cli, "")); err != nil {
		t.Fatalf("must be accepted: %v", err)
	}
}

// Codex #70 round 9 (positive): a scalar flag and a spreads_to flag may share a body var when
// each excludes the other, whichever is declared first.
func TestCLISharedVarScalarAndSpreadEitherOrder(t *testing.T) {
	scalar := `{"name":"bc","type":"bool","excludes":["mode"],"maps_to":"body:$blockContent","help":"h"}`
	spread := `{"name":"mode","type":"enum","enum":["block","alert"],"excludes":["bc"],"spreads_to":["body:$blockContent","body:$alertOn"],"transform":"mode_block_alert","help":"h"}`
	for name, flags := range map[string]string{"scalar first": scalar + "," + spread, "spread first": spread + "," + scalar} {
		cli := `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"blockContent\":\"$blockContent?\",\"alertOn\":\"$alertOn?\"}","flags":[` + flags + `]}`
		if _, err := Parse(cliFixture(cli, "")); err != nil {
			t.Errorf("%s: must be accepted: %v", name, err)
		}
	}
}

// Codex #70-2 (positive): one op may back `a g1 show` and `a g2 show` — the group is part
// of the command path, so the per-op duplicate check must include it.
func TestCLIGroupDistinguishesVerbsOnOneOp(t *testing.T) {
	verb := func(group string) string {
		return `{"area":"a","group":"` + group + `","verb":"show","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`
	}
	if _, err := Parse(cliFixture("["+verb("g1")+","+verb("g2")+"]", "")); err != nil {
		t.Fatalf("distinct groups are distinct command paths: %v", err)
	}
	if _, err := Parse(cliFixture("["+verb("g1")+","+verb("g1")+"]", "")); err == nil || !strings.Contains(err.Error(), "declared twice") {
		t.Fatalf("the same group+verb twice on one op must still be rejected, got %v", err)
	}
}

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
		// Codex #70-5: a header: destination must be a header the op (or a reachable branch op) declares.
		{"header flag not declared on op", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"tz","type":"string","maps_to":"header:not-declared","help":"h"}]}`, "", "not one of the op's declared headers"},
		{"branch-only header flag not selected by its own flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"flag:sel","op":"pause.other2"}],"flags":[{"name":"sel","type":"string","maps_to":"filter:role","help":"h"},{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"tz","type":"string","maps_to":"header:timezone","help":"h"}]}`, "", "does not select"},
		// Codex #70-6: a required flag with a default is always populated, so "requires" could
		// not tell whether it was given; like excludes, requires may only name undefaulted flags.
		{"requires a defaulted flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\",\"m\":\"$m\"}","flags":[{"name":"x","type":"string","requires":["mode"],"maps_to":"body:$x","help":"h"},{"name":"mode","type":"string","default":"b","maps_to":"body:$m","help":"h"}]}`, "", "has a default"},
		// Codex #70-7: $child.* needs a child to resolve from; an account/self verb has none.
		{"child resolver on an account verb", `{"area":"a","verb":"v","priority":"core","target":"account","summary":"s","body_template":"{\"p\":\"$child.profileId\"}","resolve":["$child.profileId"]}`, "", "target account"},
		{"child resolver on a self verb query", `{"area":"a","verb":"v","priority":"core","target":"self","summary":"s","query":{"lat":"$child.profileId"},"resolve":["$child.profileId"]}`, `"takes_body":false,"query":["lat"]`, "target self"},
		// Codex #70-8: identical or nested one_of groups make exactly-one unsatisfiable or dead.
		{"one_of duplicate groups", `{"area":"a","verb":"v","priority":"core","target":"account","summary":"s","one_of":[["address"],["address"]],"flags":[{"name":"address","type":"string","maps_to":"query:address","help":"h"}]}`, `"takes_body":false,"query":["address"]`, "one_of"},
		{"one_of subset group", `{"area":"a","verb":"v","priority":"core","target":"account","summary":"s","one_of":[["lat","lon"],["lat"]],"flags":[{"name":"lat","type":"float","maps_to":"query:lat","help":"h"},{"name":"lon","type":"float","maps_to":"query:lon","help":"h"}]}`, `"takes_body":false,"query":["lat","lon"]`, "one_of"},
		// Round 3 (Codex #70 + the verification sweep): one presence vocabulary — every
		// presence-based reference (excludes, requires, nulls, one_of members, select flag:
		// conditions) names another, undefaulted flag; a select condition appears once; a
		// resolver-backed default has the flag's type; $child.* needs a child however it is
		// referenced; a child branch ends on a child target; a lookup is a plain read; identity
		// headers are the engine's; a query map cannot read a filter: flag.
		{"select condition repeated", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"flag:sel","op":"pause.other2"},{"when":"flag:sel","op":"pause.other2"}],"flags":[{"name":"sel","type":"string","maps_to":"filter:role","help":"h"},{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "repeats"},
		{"select condition child repeated", `{"area":"a","verb":"v","priority":"core","target":"account","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"child","op":"pause.other2","target":"child"},{"when":"child","op":"pause.other2","target":"child"}],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "repeats"},
		{"flag excludes itself", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h","excludes":["x"]}]}`, "", "itself"},
		{"flag requires itself", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h","requires":["x"]}]}`, "", "itself"},
		{"flag nulls itself", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x?\"}","flags":[{"name":"x","type":"string","nulls":["x"],"maps_to":"body:$x","help":"h"}]}`, "", "itself"},
		{"one_of member with a default", `{"area":"a","verb":"v","priority":"core","target":"account","summary":"s","one_of":[["lat","lon"],["address"]],"flags":[{"name":"lat","type":"float","maps_to":"query:lat","help":"h"},{"name":"lon","type":"float","maps_to":"query:lon","help":"h"},{"name":"address","type":"string","default":"home","maps_to":"query:address","help":"h"}]}`, `"takes_body":false,"query":["lat","lon","address"]`, "has a default"},
		{"select flag condition on a defaulted flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\",\"m\":\"$m\"}","select":[{"when":"flag:m","op":"pause.other2"}],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"m","type":"string","default":"d","maps_to":"body:$m","help":"h"}]}`, "", "has a default"},
		{"bool flag defaulted to a timezone", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"bool","default":"$local.timezone","maps_to":"body:$x","help":"h"}],"resolve":["$local.timezone"]}`, "", "type bool"},
		{"int flag defaulted to a uuid", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"int","default":"$uuid","maps_to":"body:$x","help":"h"}],"resolve":["$uuid"]}`, "", "not a supported default"},
		{"string flag defaulted to epoch ms", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","default":"$now.epochMs","maps_to":"body:$x","help":"h"}],"resolve":["$now.epochMs"]}`, "", "not a supported default"},
		{"child resolver as a flag default on an account verb", `{"area":"a","verb":"v","priority":"core","target":"account","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"int","default":"$child.profileId","maps_to":"body:$x","help":"h"}]}`, "", "not a supported default"},
		{"child branch without a child target", `{"area":"a","verb":"v","priority":"core","target":"account","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"child","op":"pause.other2"}],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "child branch"},
		{"lookup to an op with a path placeholder", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\",\"n\":\"$lookup:pause.withph::name\"}","resolve":["$lookup:pause.withph::name"],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "placeholder"},
		{"lookup to an op with a required query", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\",\"n\":\"$lookup:pause.reqq::name\"}","resolve":["$lookup:pause.reqq::name"],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "required query"},
		{"lookup to an unavailable op", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\",\"n\":\"$lookup:pause.gone::name\"}","resolve":["$lookup:pause.gone::name"],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "unavailable"},
		{"header flag names an identity header", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"t","type":"string","maps_to":"header:x-fp-identifier-target-serviceid","help":"h"}]}`, `"headers":["x-fp-identifier-target-serviceid"]`, "identity header"},
		{"headers constant names an identity header", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","headers":{"x-fp-identifier-target-serviceid":"1"},"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, `"headers":["x-fp-identifier-target-serviceid"]`, "identity header"},
		{"header flag names the dynamic placeholder", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"t","type":"string","maps_to":"header:(@HeaderMap dynamic)","help":"h"}]}`, `"headers":["(@HeaderMap dynamic)"]`, "not a header name"},
		{"query map reads a filter flag", `{"area":"a","verb":"v","priority":"core","target":"account","summary":"s","query":{"lat":"$only"},"flags":[{"name":"only","type":"string","maps_to":"filter:role","help":"h"}]}`, `"takes_body":false,"query":["lat"]`, "filter"},
		// Closure pass (verification sweep): the remaining schema invariants.
		// A required query param needs a source that is always present; filter:/find: flags act
		// only when given, so a default would be a lie; the generator owns some flag names;
		// repeatable means a string list; an unused resolve entry is a wasted read; alias_of and
		// call_only blocks carry nothing else; spc_token auth is not implemented by the engine;
		// a required template var needs a flag that is always there; body vars do not shadow
		// resolver names; a query map cannot read a spreads_to flag.
		{"required query fed by an optional undefaulted flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","flags":[{"name":"a","type":"string","maps_to":"query:a","help":"h"}]}`, `"takes_body":false,"query":["a"],"required_query":["a"]`, "neither required nor defaulted"},
		{"filter flag with a default", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"only","type":"string","default":"x","maps_to":"filter:role","help":"h"}]}`, "", "may not have a default"},
		{"reserved flag name child", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"child","type":"string","maps_to":"filter:role","help":"h"}]}`, "", "reserved"},
		{"reserved flag name dry-run", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"dry-run","type":"string","maps_to":"filter:role","help":"h"}]}`, "", "reserved"},
		{"reserved flag name json", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"json","type":"string","maps_to":"filter:role","help":"h"}]}`, "", "reserved"},
		{"repeatable int flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"ids\":[{\"id\":\"$n\"}]}","flags":[{"name":"n","type":"int","repeatable":true,"required":true,"maps_to":"body:$n","help":"h"}]}`, "", "repeatable"},
		{"unused resolve entry", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","resolve":["$lookup:pause.other::n"],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "never used"},
		{"alias block with flags", `{"alias_of":"pause.twin","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "alias_of"},
		{"call_only block with a body template", `{"call_only":true,"reason":"r","body_template":"{\"x\":\"$x\"}"}`, "", "call_only"},
		{"auth spc_token not implemented", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","auth":"spc_token","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "not implemented"},
		{"required template var fed by an optional undefaulted flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","maps_to":"body:$x","help":"h"}]}`, "", "optional and has no default"},
		{"body var named like a resolver", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"p\":\"$child.profileId\"}","flags":[{"name":"p","type":"int","required":true,"maps_to":"body:$child.profileId","help":"h"}]}`, "", "resolver"},
		{"query map reads a spreads_to flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"blockContent\":\"$blockContent\",\"alertOn\":\"$alertOn\"}","query":{"lat":"$mode"},"flags":[{"name":"mode","type":"enum","enum":["block","alert"],"default":"block","spreads_to":["body:$blockContent","body:$alertOn"],"transform":"mode_block_alert","help":"h"}]}`, `"query":["lat"]`, "spreads_to"},
		// Codex #70 round 4: identity headers are reserved whatever their case (HTTP header names
		// are case-insensitive, and the client would replace the engine's lowercase one); every
		// flag sharing a body var is checked against the template position, not just the first.
		{"header constant names an identity header in mixed case", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","headers":{"X-Fp-Identifier-Target-Serviceid":"1"},"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, `"headers":["X-Fp-Identifier-Target-Serviceid"]`, "identity header"},
		{"header flag names an identity header in mixed case", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"t","type":"string","maps_to":"header:X-FP-Identifier-Target-ServiceId","help":"h"}]}`, `"headers":["X-FP-Identifier-Target-ServiceId"]`, "identity header"},
		{"scalar then repeatable flag sharing a scalar var", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"v\":\"$v?\"}","flags":[{"name":"one","type":"string","excludes":["many"],"maps_to":"body:$v","help":"h"},{"name":"many","type":"string","repeatable":true,"excludes":["one"],"maps_to":"body:$v","help":"h"}]}`, "", "repeatable"},
		{"repeatable then scalar flag sharing a scalar var", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"v\":\"$v?\"}","flags":[{"name":"many","type":"string","repeatable":true,"excludes":["one"],"maps_to":"body:$v","help":"h"},{"name":"one","type":"string","excludes":["many"],"maps_to":"body:$v","help":"h"}]}`, "", "repeatable"},
		// Codex #70-3: a null list entry is a load error, never a nil dereference.
		{"null cli entry", `[null]`, "", "null"},
		// Codex #69-1: a lookup runs BEFORE --confirm, so it may only name a read-only GET.
		{"lookup to a mutating op", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"n\":\"$lookup:pause.twin::x\"}","resolve":["$lookup:pause.twin::x"]}`, "", "read-only GET"},
		{"lookup to a destructive read", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"n\":\"$lookup:pause.purge::x\"}","resolve":["$lookup:pause.purge::x"]}`, "", "read-only GET"},
		{"exists lookup to a mutating op", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"exists:$lookup:pause.twin::x","op":"pause.other2"}],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "read-only GET"},
		// Codex #70-1: a query param only a branch op declares must be reachable whenever its
		// flag is given — the flag selects that branch itself, requires the flag that does,
		// or the branch is the --child one; otherwise --alt alone selects the base op and
		// the engine would have to drop the value.
		{"branch-only query flag not selected by its own flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"flag:sel","op":"pause.other2"}],"flags":[{"name":"sel","type":"string","maps_to":"filter:role","help":"h"},{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"alt","type":"string","maps_to":"query:alt","help":"h"}]}`, "", "does not select"},
		{"branch-only query flag under an exists branch only", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"exists:$lookup:pause.other::id","op":"pause.other2"}],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"alt","type":"string","maps_to":"query:alt","help":"h"}]}`, "", "does not select"},
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
		{"unclassified literal field", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"geofenceId\":123,\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "unclassified example value"},
		{"unclassified array field", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"days\":[\"Mon\"],\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "unclassified example value"},
		{"unknown var", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$ghost\"}"}`, "", "neither a flag"},
		{"resolved var not listed", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"p\":\"$child.profileId\"}"}`, "", "not listed in resolve"},
		{"flag var unused by template", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"y","type":"string","maps_to":"body:$y","help":"h"}]}`, "", "never uses"},
		// Codex #66-1: "?" (omission) must be backed by a flag that can actually be unset.
		{"optional var on a required flag nothing nulls", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x?\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "nothing nulls it"},
		{"optional resolved var", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"p\":\"$child.profileId?\"}","resolve":["$child.profileId"]}`, "", "cannot be optional"},
		{"nulls unknown flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","maps_to":"body:$x","nulls":["ghost"],"help":"h"}]}`, "", "unknown flag"},
		// Codex #66 re-review: an exclusion against a defaulted flag is undecidable (the
		// default makes the flag always present), so it is rejected; precedence is nulls.
		{"excludes names a defaulted flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\",\"y\":\"$y\"}","flags":[{"name":"x","type":"string","default":"d","maps_to":"body:$x","help":"h"},{"name":"y","type":"bool","default":false,"maps_to":"body:$y","excludes":["x"],"help":"h"}]}`, "", "has a default"},
		// Codex #66 re-review: verb-level at_least_one (account set --family-name|--timezone).
		{"at_least_one with a single flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","at_least_one":["x"],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "at least two"},
		{"at_least_one names unknown flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","at_least_one":["ghost","x"],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "unknown flag"},
		{"at_least_one names a required flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\",\"y\":\"$y\"}","at_least_one":["x","y"],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"y","type":"string","maps_to":"body:$y","help":"h"}]}`, "", "already required"},
		// Codex #67: two flags competing for one template value must fail at Parse, not be
		// decided by map iteration or engine precedence.
		{"duplicate body var mapping", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"y","type":"string","maps_to":"body:$x","help":"h"}]}`, "", "already mapped"},
		// Codex #67: a path mapping must name a real {placeholder} of the op's path.
		{"path flag to unknown placeholder", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","flags":[{"name":"dev-id","type":"string","maps_to":"path:device-id","help":"h"}]}`, `"takes_body":false,"path":"/d/{deviceId}"`, "not a {placeholder}"},
		// Codex #67: a template on an op that declares no body is a classification typo.
		{"template on a body-less op", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, `"takes_body":false`, "declares no body"},
		// Codex #67: resolved variables are an exact vocabulary, not a prefix match.
		{"resolved var typo", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"p\":\"$child.profielId\"}","resolve":["$child.profielId"]}`, "", "not a supported resolved variable"},
		{"uuid typo", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"e\":\"$uuidTypo\"}","resolve":["$uuidTypo"]}`, "", "not a supported resolved variable"},
		{"malformed lookup", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"n\":\"$lookup:nope\"}","resolve":["$lookup:nope"]}`, "", "malformed $lookup"},
		{"lookup to missing op", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"n\":\"$lookup:pause.nope:id=x:name\",\"x\":\"$x\"}","resolve":["$lookup:pause.nope:id=x:name"],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "does not name an existing"},
		{"lookup keyed by unknown flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"n\":\"$lookup:pause.other:id=ghost:name\"}","resolve":["$lookup:pause.other:id=ghost:name"]}`, "", "unknown flag"},
		// Codex #66 round 3: conditional op selection is declared, and everything it names is
		// checked — the op, the flag, the lookup, and the condition vocabulary itself.
		{"select op missing", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"flag:sel","op":"pause.nope"}],"flags":[{"name":"sel","type":"string","maps_to":"filter:role","help":"h"},{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "does not name an existing"},
		{"select unknown condition", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"moon","op":"pause.other"}],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "condition"},
		{"select flag not declared", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"flag:ghost","op":"pause.other"}],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "unknown flag"},
		{"select malformed lookup", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"exists:$lookup:nope","op":"pause.other"}],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "malformed $lookup"},
		// Codex #66 round 3: one flag, several fields — only via a structured transform.
		{"spreads_to with maps_to too", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"blockContent\":\"$blockContent\",\"alertOn\":\"$alertOn\"}","flags":[{"name":"mode","type":"enum","enum":["block","alert"],"maps_to":"body:$blockContent","spreads_to":["body:$blockContent","body:$alertOn"],"transform":"mode_block_alert","help":"h"}]}`, "", "exactly one of maps_to or spreads_to"},
		{"spreads_to without structured transform", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"blockContent\":\"$blockContent\",\"alertOn\":\"$alertOn\"}","flags":[{"name":"mode","type":"enum","enum":["block","alert"],"spreads_to":["body:$blockContent","body:$alertOn"],"help":"h"}]}`, "", "structured transform"},
		{"spreads_to destination unused by template", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"blockContent\":\"$blockContent\"}","flags":[{"name":"mode","type":"enum","enum":["block","alert"],"default":"block","spreads_to":["body:$blockContent","body:$alertOn"],"transform":"mode_block_alert","help":"h"}]}`, "", "never uses"},
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
		{"constant value mismatch", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"MAPPVersion\":\"wrong\",\"x\":\"$x\"}","constants":{"MAPPVersion":"8.1"},"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "does not match the declared constant"},
		// one query parameter, one source.
		{"two flags to one query param", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","flags":[{"name":"a","type":"string","maps_to":"query:q","help":"h"},{"name":"b","type":"string","maps_to":"query:q","help":"h"}]}`, `"takes_body":false,"query":["q"]`, "already mapped"},
		// two repeatables in one expansion element have no defined expansion.
		{"two repeatables in one element", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"items\":[{\"a\":\"$a\",\"b\":\"$b\"}]}","flags":[{"name":"a","type":"string","repeatable":true,"maps_to":"body:$a","help":"h"},{"name":"b","type":"string","repeatable":true,"maps_to":"body:$b","help":"h"}]}`, "", "second repeatable"},
		// a select branch is validated against ITS op's contract: a bodyless GET cannot inherit a body.
		{"select branch to a bodyless op inherits a body", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"flag:sel","op":"pause.other"}],"flags":[{"name":"sel","type":"string","maps_to":"filter:role","help":"h"},{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "declares no body"},
		// dependent flag groups name declared flags and need real alternatives.
		{"requires unknown flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","requires":["ghost"],"maps_to":"body:$x","help":"h"}]}`, "", "unknown flag"},
		{"one_of with one group", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","one_of":[["x"]],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "at least two"},
		{"one_of unknown flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","one_of":[["ghost"],["x"]],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "unknown flag"},
		// Codex #66 round 5: one op, several verbs — `cli` may be a list; alias/call_only stay single.
		{"cli list mixing alias and verb", `[{"alias_of":"pause.other"},{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}]`, "", "only entry"},
		{"cli list duplicate verb", `[{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]},{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}]`, "", "declared twice"},
		// Codex #66 round 5: fixed header constants must name headers the op declares.
		{"header constant not declared on op", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","headers":{"x-nope":"1"},"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "not one of the op's declared headers"},
		// Codex #67: a structured transform fills a FIXED set of vars; spreads_to must match it.
		{"spreads_to wrong arity", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"a\":\"$blockContent\"}","flags":[{"name":"mode","type":"enum","enum":["block","alert"],"spreads_to":["body:$blockContent"],"transform":"mode_block_alert","help":"h"}]}`, "", "must spread to exactly"},
		{"spreads_to wrong names", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"a\":\"$a\",\"b\":\"$b\"}","flags":[{"name":"mode","type":"enum","enum":["block","alert"],"spreads_to":["body:$a","body:$b"],"transform":"mode_block_alert","help":"h"}]}`, "", "must spread to exactly"},
		// Codex #67 (round on 71d0426): four more load-time rules.
		{"command path declared by two ops", `{"area":"tw","verb":"in","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "declared by both"},
		{"alias to a different route", `{"alias_of":"pause.other"}`, "", "does not share method and path"},
		{"alias to an op with no verb", `{"alias_of":"pause.other2"}`, `"method":"POST","path":"/o2"`, "canonical verb"},
		{"two flags to one header", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","flags":[{"name":"a","type":"string","maps_to":"header:x-h","help":"h"},{"name":"b","type":"string","maps_to":"header:x-h","help":"h"}]}`, `"takes_body":false,"headers":["x-h"]`, "already mapped"},
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
		// Codex #70 round 5: a required query param fed through the query map needs an always-
		// present flag too; a required flag cannot be a one_of alternative; a header has one
		// source (constant or flag); every inherited global flag name is reserved.
		{"required query fed through the query map by an optional flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","query":{"a":"$x"},"flags":[{"name":"x","type":"string","maps_to":"query:b","help":"h"}]}`, `"takes_body":false,"query":["a","b"],"required_query":["a"]`, "neither required nor defaulted"},
		{"one_of member that is required", `{"area":"a","verb":"v","priority":"core","target":"account","summary":"s","one_of":[["a"],["b"]],"flags":[{"name":"a","type":"string","required":true,"maps_to":"query:a","help":"h"},{"name":"b","type":"string","maps_to":"query:b","help":"h"}]}`, `"takes_body":false,"query":["a","b"]`, "required"},
		{"header constant competing with a flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","headers":{"x-h":"1"},"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"h","type":"string","maps_to":"header:x-h","help":"h"}]}`, `"headers":["x-h"]`, "also mapped from"},
		{"reserved flag name plain", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"plain","type":"string","maps_to":"filter:role","help":"h"}]}`, "", "reserved"},
		{"reserved flag name service-id", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"service-id","type":"string","maps_to":"filter:role","help":"h"}]}`, "", "reserved"},
		{"reserved flag name profile-id", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"profile-id","type":"string","maps_to":"filter:role","help":"h"}]}`, "", "reserved"},
		{"reserved flag name device-id", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"device-id","type":"string","maps_to":"filter:role","help":"h"}]}`, "", "reserved"},
		{"reserved flag name force", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"force","type":"string","maps_to":"filter:role","help":"h"}]}`, "", "reserved"},
		// Codex #70 round 6 / #71: a header with a fixed op value (header_values) has no other
		// source; a transform only applies to the flag types it is defined for; a select
		// branch may not reach an op marked unavailable.
		{"header constant overriding a fixed header value", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","headers":{"app-name":"OTHER"},"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, `"headers":["app-name"],"header_values":{"app-name":"VSF"}`, "fixed value"},
		{"header flag overriding a fixed header value", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"an","type":"string","maps_to":"header:app-name","help":"h"}]}`, `"headers":["app-name"],"header_values":{"app-name":"VSF"}`, "fixed value"},
		{"transform on an incompatible flag type", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\",\"t\":\"$t\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"t","type":"bool","default":false,"maps_to":"body:$t","transform":"epoch_ms","help":"h"}]}`, "", "transform"},
		{"bool01 on a string flag", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\",\"t\":\"$t\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"t","type":"string","required":true,"maps_to":"body:$t","transform":"bool01","help":"h"}]}`, "", "transform"},
		{"select branch to an unavailable op", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"flag:sel","op":"pause.gone"}],"flags":[{"name":"sel","type":"string","maps_to":"filter:role","help":"h"},{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "unavailable"},
		// Codex #70 round 7: multipart ops get no verb; a required flag cannot select a branch
		// (it would always win); a keyed lookup's key flag is always present unless the lookup
		// is confined to the branch that flag selects; aliases target available ops; an enum
		// transform's domain bounds the flag's enum; fixed header values match case-insensitively.
		{"verb on a multipart op", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s"}`, `"multipart":true`, "multipart"},
		{"select branch to a multipart op", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"flag:sel","op":"pause.mp"}],"flags":[{"name":"sel","type":"string","maps_to":"filter:role","help":"h"},{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "multipart"},
		{"required flag as a branch selector", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"flag:x","op":"pause.other2"}],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "required"},
		{"keyed lookup whose key flag may be absent", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\",\"n\":\"$lookup:pause.other:id=cat:name\"}","resolve":["$lookup:pause.other:id=cat:name"],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"cat","type":"int","maps_to":"filter:role","help":"h"}]}`, "", "always present"},
		{"alias to an unavailable canonical op", `{"alias_of":"pause.goneverb"}`, "", "unavailable"},
		{"enum outside the transform's domain", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\",\"f\":\"$f\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"f","type":"enum","enum":["nonsense"],"default":"nonsense","maps_to":"body:$f","transform":"pause_schedule","help":"h"}]}`, "", "pause_schedule accepts"},
		{"header constant overriding a fixed value in another case", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","headers":{"App-Name":"OTHER"},"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, `"headers":["App-Name"],"header_values":{"app-name":"VSF"}`, "fixed value"},
		// Codex #70 round 8: a child branch on a child/device base would always match; every
		// producer of a shared body var excludes every other; an exclusion may not involve a
		// required flag; an at_least_one member may carry a lookup default (resend the current
		// value) but not a literal one.
		{"child branch on a child base", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"child","op":"pause.other2"}],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`, "", "always"},
		{"three producers not pairwise exclusive", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\",\"v\":\"$v?\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"a","type":"string","excludes":["b","c"],"maps_to":"body:$v","help":"h"},{"name":"b","type":"string","excludes":["a"],"maps_to":"body:$v","help":"h"},{"name":"c","type":"string","excludes":["a"],"maps_to":"body:$v","help":"h"}]}`, "", "pairwise"},
		{"optional flag excluding a required one", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\",\"o\":\"$o?\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"o","type":"string","excludes":["x"],"maps_to":"body:$o","help":"h"}]}`, "", "required"},
		{"at_least_one member with a literal default", `{"area":"a","verb":"v","priority":"core","target":"account","summary":"s","body_template":"{\"a\":\"$a\",\"b\":\"$b?\"}","at_least_one":["a","b"],"flags":[{"name":"a","type":"string","default":"lit","maps_to":"body:$a","help":"h"},{"name":"b","type":"string","maps_to":"body:$b","help":"h"}]}`, "", "default"},
		// Codex #70 round 9: a later flag: selector whose flag requires (transitively) an earlier
		// selector can never be reached; a resolver-backed default is part of the resolve plan;
		// a flag cannot both require and exclude the same flag.
		{"selector shadowed by the flag it requires", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"flag:a","op":"pause.other2"},{"when":"flag:b","op":"pause.other2"}],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"a","type":"string","maps_to":"filter:role","help":"h"},{"name":"b","type":"string","maps_to":"filter:role","help":"h","requires":["a"]}]}`, "", "unreachable"},
		{"selector shadowed transitively", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\"}","select":[{"when":"flag:a","op":"pause.other2"},{"when":"flag:c","op":"pause.other2"}],"flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"a","type":"string","maps_to":"filter:role","help":"h"},{"name":"b","type":"string","maps_to":"filter:role","help":"h","requires":["a"]},{"name":"c","type":"string","maps_to":"filter:role","help":"h","requires":["b"]}]}`, "", "unreachable"},
		{"resolver default not listed in resolve", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\",\"t\":\"$t\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"t","type":"tz","default":"$local.timezone","maps_to":"body:$t","help":"h"}]}`, "", "not listed in resolve"},
		{"flag both requiring and excluding another", `{"area":"a","verb":"v","priority":"core","target":"child","summary":"s","body_template":"{\"x\":\"$x\",\"a\":\"$a?\",\"b\":\"$b?\"}","flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"},{"name":"a","type":"string","requires":["b"],"excludes":["b"],"maps_to":"body:$a","help":"h"},{"name":"b","type":"string","excludes":["a"],"maps_to":"body:$b","help":"h"}]}`, "", "contradict"},
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
	  "flags":[{"name":"x","type":"string","required":true,"maps_to":"body:$x","help":"h"}]}`
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
	  "flags":[{"name":"cat","required":true,"type":"int","maps_to":"body:$cat","help":"h"},{"name":"dev-id","type":"string","maps_to":"path:deviceId","help":"h"}]}`
	if _, err := Parse(cliFixture(cli, `"path":"/d/{deviceId}"`)); err != nil {
		t.Fatalf("exact resolve names, a valid $lookup, and a real path placeholder must be accepted: %v", err)
	}
}

// The three round-3 mechanisms in their valid shapes must parse: a select list over an
// existing op keyed by a declared flag and a valid lookup; a spreads_to pair through the
// structured mode_block_alert transform; a repeatable var inside a single-element array.
func TestCLISelectSpreadsAndRepeatableAccepted(t *testing.T) {
	cli := `{"area":"a","verb":"v","priority":"core","target":"account","summary":"s",
	  "body_template":"{\"blockContent\":\"$blockContent\",\"alertOn\":\"$alertOn\",\"domains\":[{\"status\":\"$status\",\"url\":\"$url\"}],\"n\":\"$n?\"}",
	  "select":[{"when":"flag:n","op":"pause.other2"},{"when":"exists:$lookup:pause.other:id=n:name","op":"pause.other2"},{"when":"child","op":"pause.other2","target":"child"}],
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
	  "resolve":["$child.profileId","$local.timezone","$lookup:pause.other::screenTimeLimitId","$lookup:pause.other::familyName"],
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
