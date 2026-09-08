package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ejc3/safe_cli/internal/client"
	"github.com/ejc3/safe_cli/internal/descriptor"
)

// fakeBackend serves the account read for the synthetic family plus whatever routes a
// test registers, capturing the last request per path.
type fakeBackend struct {
	srv   *httptest.Server
	seen  map[string]capturedReq
	extra map[string]func(w http.ResponseWriter, r *http.Request)
}

type capturedReq struct {
	method, query string
	body          string
	headers       http.Header
}

func newFakeBackend(t *testing.T) *fakeBackend {
	t.Helper()
	accountCache = nil
	t.Cleanup(func() { accountCache = nil })
	fb := &fakeBackend{seen: map[string]capturedReq{}, extra: map[string]func(http.ResponseWriter, *http.Request){}}
	fb.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		fb.seen[r.URL.Path] = capturedReq{method: r.Method, query: r.URL.RawQuery, body: string(b), headers: r.Header.Clone()}
		if strings.Contains(r.URL.Path, "userprofiles") {
			_, _ = w.Write([]byte(syntheticAccount))
			return
		}
		if h, ok := fb.extra[r.URL.Path]; ok {
			h(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"statusCode":200}`))
	}))
	t.Cleanup(fb.srv.Close)
	return fb
}

func (fb *fakeBackend) do() doFunc {
	cl := client.New("T")
	cl.BaseURL = fb.srv.URL
	return cl.DoH
}

func pauseCall(given map[string]any, child string) verbCall {
	return verbCall{entity: "pause_internet", op: "pauseInternet", area: "pause-internet", verb: "pause",
		given: given, child: child, selfSvc: "1000001", selfPid: "1000002"}
}

const pausePath = "/frisco/parental-control/v5/device/pause"

// End to end on the REAL pause-internet block: --child resolves profileId/serviceId/deviceId
// from the account read, --for is transformed, defaults fill the rest, the target header
// is the child's service id, and no id was typed by the caller.
func TestInvokePauseBuildsBodyFromTarget(t *testing.T) {
	fb := newFakeBackend(t)
	d, _ := descriptor.Default()
	var out strings.Builder
	if err := invoke(context.Background(), fb.do(), d, pauseCall(map[string]any{"for": "1h"}, "2000001"), &out, true); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	req := fb.seen[pausePath]
	if req.method != "POST" {
		t.Fatalf("method = %q", req.method)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(req.body), &body); err != nil {
		t.Fatalf("body not JSON: %v (%s)", err, req.body)
	}
	dev := body["profiles"].([]any)[0].(map[string]any)["devices"].([]any)[0].(map[string]any)
	if body["profiles"].([]any)[0].(map[string]any)["profileId"] != float64(3000001) ||
		dev["serviceId"] != float64(2000001) || dev["deviceId"] != float64(4000001) {
		t.Errorf("resolved ids wrong: %s", req.body)
	}
	if dev["pauseSchedule"] != "1_hour" || dev["untilIUnpause"] != false || dev["callOnlyMode"] != false {
		t.Errorf("semantic fields wrong: %s", req.body)
	}
	if tz, _ := body["timeZone"].(string); len(tz) < 3 || len(tz) > 5 {
		t.Errorf("timeZone should be a short code, got %q", tz)
	}
	if req.headers.Get("x-fp-identifier-target-serviceid") != "2000001" {
		t.Errorf("target header = %q", req.headers.Get("x-fp-identifier-target-serviceid"))
	}
	// --json output carries _meta.target so an agent can chain.
	if !strings.Contains(out.String(), "_meta") || !strings.Contains(out.String(), "2000001") {
		t.Errorf("--json output lacks _meta.target: %s", out.String())
	}
}

// --indefinite: untilIUnpause=true and pauseSchedule ABSENT (the wire-verified body).
func TestInvokePauseIndefiniteOmitsSchedule(t *testing.T) {
	fb := newFakeBackend(t)
	d, _ := descriptor.Default()
	if err := invoke(context.Background(), fb.do(), d, pauseCall(map[string]any{"indefinite": true}, "2000001"), &strings.Builder{}, true); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	b := fb.seen[pausePath].body
	if strings.Contains(b, "pauseSchedule") || !strings.Contains(b, `"untilIUnpause":true`) {
		t.Errorf("indefinite body wrong: %s", b)
	}
}

// An UNPAIRED child is refused for a device verb with an actionable message; --allow-unpaired
// sends anyway. A read verb (status, target child) is not guarded.
func TestInvokePairingGuard(t *testing.T) {
	fb := newFakeBackend(t)
	d, _ := descriptor.Default()
	err := invoke(context.Background(), fb.do(), d, pauseCall(nil, "2000002"), &strings.Builder{}, true)
	if err == nil || !strings.Contains(err.Error(), "UNPAIRED") || !strings.Contains(err.Error(), "--allow-unpaired") {
		t.Fatalf("want an unpaired refusal, got %v", err)
	}
	if _, sent := fb.seen[pausePath]; sent {
		t.Error("refused request must not be sent")
	}
	vc := pauseCall(nil, "2000002")
	vc.allowUnpaired = true
	if err := invoke(context.Background(), fb.do(), d, vc, &strings.Builder{}, true); err != nil {
		t.Fatalf("--allow-unpaired: %v", err)
	}
	status := verbCall{entity: "pause_internet", op: "getDevices", area: "pause-internet", verb: "status", child: "2000002", selfSvc: "1000001", selfPid: "1000002"}
	if err := invoke(context.Background(), fb.do(), d, status, &strings.Builder{}, true); err != nil {
		t.Fatalf("status on an unpaired child must not be guarded: %v", err)
	}
	if fb.seen["/parental-control/frisco/v6/device/pause"].method != "GET" {
		t.Errorf("status should GET the devices route")
	}
}

// Structural refusals happen before any request: a missing --child, an enum miss, and a
// --child that is a name rather than a service id.
func TestInvokeRefusesBeforeSending(t *testing.T) {
	fb := newFakeBackend(t)
	d, _ := descriptor.Default()
	cases := []struct {
		name string
		vc   verbCall
		want string
	}{
		{"missing child", pauseCall(nil, ""), "needs --child"},
		{"enum miss", pauseCall(map[string]any{"for": "45m"}, "2000001"), "30m|1h|2h|4h|until-morning"},
		{"name as child", pauseCall(nil, "Alex"), "SERVICE-ID"},
		{"unknown child", pauseCall(nil, "9999999"), "not a member"},
	}
	for _, c := range cases {
		err := invoke(context.Background(), fb.do(), d, c.vc, &strings.Builder{}, true)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want error containing %q, got %v", c.name, c.want, err)
		}
	}
	if _, sent := fb.seen[pausePath]; sent {
		t.Error("no refused invocation may reach the backend")
	}
}

// --dry-run prints the exact request plus the resolved target, and sends nothing.
func TestInvokeDryRunShowsResolvedTarget(t *testing.T) {
	fb := newFakeBackend(t) // only the account read should hit it
	d, _ := descriptor.Default()
	rc := &runContext{D: d, G: &Globals{}}
	// dumpRequest builds the request the way the real client would, then returns its dump.
	dump := dumpRequest(rc, "TOK")
	// The engine still needs the account read; route it to the fake backend.
	do := func(ctx context.Context, method, path string, body []byte, headers map[string]string) (*client.Response, error) {
		if strings.Contains(path, "userprofiles") {
			return fb.do()(ctx, method, path, body, headers)
		}
		return dump(ctx, method, path, body, headers)
	}
	vc := pauseCall(map[string]any{"for": "2h"}, "2000001")
	vc.dryRun = true
	var out strings.Builder
	if err := invoke(context.Background(), do, d, vc, &out, false); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "# target: Alex (service 2000001, profile 3000001, device 4000001, PAIRED)") {
		t.Errorf("dry-run should name the resolved target:\n%s", s)
	}
	if !strings.Contains(s, "POST "+pausePath) || !strings.Contains(s, `"pauseSchedule":"2_hour"`) {
		t.Errorf("dry-run should print the exact request:\n%s", s)
	}
	if _, sent := fb.seen[pausePath]; sent {
		t.Error("dry-run must not send the pause")
	}
}

// A synthetic descriptor exercising the mechanisms pause-internet does not: a select branch
// chosen by an explicitly-given flag, a structured spread, repeatable array expansion, a
// $lookup enrichment, query and header constants, and one_of/requires.
const engineFixture = `{"name":"t","base_url":"https://h","entities":{"account":{"id_field":"","operations":{"getAccountDetails":{"method":"GET","path":"/account/fam/userprofile-management/v8/accounts/userprofiles","headers":["x-fp-identifier-target-serviceid"]}}},"t":{"id_field":"","operations":{
  "listA":{"method":"GET","path":"/a","query":["q","alt"]},
  "listB":{"method":"GET","path":"/b","query":["q","alt"]},
  "cats":{"method":"GET","path":"/cats","headers":["x-fp-identifier-target-serviceid"]},
  "post":{"method":"POST","path":"/p","takes_body":true,"headers":["x-pending-activation"],"query":["q"],
    "cli":{"area":"t","verb":"do","priority":"core","target":"child","summary":"s",
      "body_template":"{\"mode\":{\"blockContent\":\"$blockContent\",\"alertOn\":\"$alertOn\"},\"domains\":[{\"url\":\"$url\",\"status\":\"$status\"}],\"id\":\"$cat\",\"catName\":\"$lookup:t.cats:id=cat:name\",\"fixed\":\"v\"}",
      "constants":{"fixed":"v"},
      "headers":{"x-pending-activation":"false"},
      "query":{"q":"$name"},
      "resolve":["$lookup:t.cats:id=cat:name"],
      "flags":[
        {"name":"mode","type":"enum","enum":["block","alert"],"default":"block","spreads_to":["body:$blockContent","body:$alertOn"],"transform":"mode_block_alert","help":"h"},
        {"name":"url","type":"string","repeatable":true,"required":true,"maps_to":"body:$url","help":"h"},
        {"name":"status","type":"enum","enum":["allow","block"],"default":"block","maps_to":"body:$status","transform":"allow_block_ab","help":"h"},
        {"name":"cat","type":"int","required":true,"maps_to":"body:$cat","help":"h"},
        {"name":"name","type":"string","maps_to":"header:x-name","help":"h"}]}},
  "log":{"method":"GET","path":"/a","query":["q","alt"],
    "cli":{"area":"t","verb":"log","priority":"core","target":"child","summary":"s",
      "select":[{"when":"flag:alt","op":"t.listB"}],
      "flags":[{"name":"alt","type":"string","maps_to":"query:alt","help":"h"},{"name":"q","type":"string","maps_to":"query:q","help":"h"}]}},
  "hist":{"method":"GET","path":"/h","query":["profileId","startDate"],"headers":["x-fp-identifier-target-serviceid"],
    "cli":{"area":"t","verb":"hist","priority":"core","target":"child","summary":"s",
      "query":{"profileId":"$child.profileId"},"resolve":["$child.profileId"],
      "flags":[{"name":"since","type":"string","maps_to":"query:startDate","transform":"iso_micro","help":"h"}]}},
  "chores":{"method":"GET","path":"/c","headers":["timezone","x-fp-identifier-target-serviceid"],
    "cli":{"area":"t","verb":"chores","priority":"core","target":"child","summary":"s",
      "headers":{"timezone":"$local.timezone"},"resolve":["$local.timezone"]}},
  "acct":{"method":"GET","path":"/acct"},
  "stget":{"method":"GET","path":"/stget","headers":["x-fp-identifier-target-serviceid"]},
  "stput":{"method":"PUT","path":"/st","takes_body":true,"headers":["x-fp-identifier-target-serviceid"]},
  "stset":{"method":"POST","path":"/st","takes_body":true,"headers":["x-fp-identifier-target-serviceid"],
    "cli":{"area":"t","verb":"stset","priority":"core","target":"child","summary":"s",
      "body_template":"{\"name\":\"$name\"}",
      "select":[{"when":"exists:$lookup:t.stget::screenTimeLimitId","op":"t.stput","body_template":"{\"name\":\"$name\",\"screenTimeLimitId\":\"$lookup:t.stget::screenTimeLimitId\"}","resolve":["$lookup:t.stget::screenTimeLimitId"]}],
      "flags":[{"name":"name","type":"string","default":"$lookup:t.acct::familyName","maps_to":"body:$name","help":"h"}]}},
  "dash":{"method":"GET","path":"/dash","headers":["x-fp-identifier-target-serviceid"],
    "cli":{"area":"t","verb":"dash","priority":"core","target":"account","summary":"s",
      "flags":[{"name":"child","type":"int","maps_to":"filter:serviceId","help":"h"}]}},
  "stime":{"method":"POST","path":"/stime","takes_body":true,"headers":["x-fp-identifier-target-serviceid"],
    "cli":{"area":"t","verb":"stime","priority":"core","target":"child","summary":"s",
      "body_template":"{\"mon\":\"$mon?\"}",
      "flags":[{"name":"weekdays","type":"int","excludes":["mon"],"maps_to":"body:$mon","help":"h"},{"name":"mon","type":"int","excludes":["weekdays"],"maps_to":"body:$mon","help":"h"}]}},
  "where":{"method":"GET","path":"/w","query":["lat","lon","address"],
    "cli":{"area":"t","verb":"where","priority":"core","target":"account","summary":"s",
      "one_of":[["lat","lon"],["address"]],
      "flags":[{"name":"lat","type":"float","requires":["lon"],"maps_to":"query:lat","help":"h"},{"name":"lon","type":"float","requires":["lat"],"maps_to":"query:lon","help":"h"},{"name":"address","type":"string","maps_to":"query:address","help":"h"}]}}
}}}}`

func engineDescriptor(t *testing.T) *descriptor.Descriptor {
	t.Helper()
	d, err := descriptor.Parse([]byte(engineFixture))
	if err != nil {
		t.Fatalf("fixture must validate: %v", err)
	}
	return d
}

func TestInvokeSpreadRepeatLookupHeadersQuery(t *testing.T) {
	fb := newFakeBackend(t)
	fb.extra["/cats"] = func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"categories":[{"id":10001,"name":"Social"},{"id":10003,"name":"Games","categoryId":1001}]}`))
	}
	d := engineDescriptor(t)
	vc := verbCall{entity: "t", op: "post", area: "t", verb: "do", child: "2000001", selfSvc: "1000001", selfPid: "1000002",
		given: map[string]any{"mode": "alert", "url": []string{"a.com", "b.com"}, "cat": int64(10003), "name": "n"}}
	if err := invoke(context.Background(), fb.do(), d, vc, &strings.Builder{}, true); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	req := fb.seen["/p"]
	var body map[string]any
	if err := json.Unmarshal([]byte(req.body), &body); err != nil {
		t.Fatalf("body: %v (%s)", err, req.body)
	}
	mode := body["mode"].(map[string]any)
	if mode["blockContent"] != false || mode["alertOn"] != true {
		t.Errorf("spread wrong: %v", mode)
	}
	domains := body["domains"].([]any)
	if len(domains) != 2 || domains[0].(map[string]any)["url"] != "a.com" || domains[1].(map[string]any)["url"] != "b.com" || domains[1].(map[string]any)["status"] != "b" {
		t.Errorf("repeatable expansion wrong: %v", domains)
	}
	if body["id"] != float64(10003) || body["catName"] != "Games" || body["fixed"] != "v" {
		t.Errorf("lookup/constant wrong: %s", req.body)
	}
	if req.headers.Get("x-pending-activation") != "false" || req.headers.Get("x-name") != "n" {
		t.Errorf("header constant/flag wrong: %v", req.headers)
	}
	if fb.seen["/cats"].headers.Get("x-fp-identifier-target-serviceid") != "2000001" {
		t.Errorf("lookup must carry the target header")
	}
}

// A lookup miss names the record and how to list valid ids, and sends nothing.
func TestInvokeLookupMiss(t *testing.T) {
	fb := newFakeBackend(t)
	fb.extra["/cats"] = func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"categories":[]}`)) }
	d := engineDescriptor(t)
	vc := verbCall{entity: "t", op: "post", area: "t", verb: "do", child: "2000001", selfSvc: "1000001", selfPid: "1000002",
		given: map[string]any{"url": []string{"a.com"}, "cat": int64(99999)}}
	err := invoke(context.Background(), fb.do(), d, vc, &strings.Builder{}, true)
	if err == nil || !strings.Contains(err.Error(), "99999") || !strings.Contains(err.Error(), "t cats") {
		t.Fatalf("want a lookup-miss error naming the id and the listing op, got %v", err)
	}
	if _, sent := fb.seen["/p"]; sent {
		t.Error("nothing may be sent after a lookup miss")
	}
}

// select flag:alt routes to the branch op only when --alt was explicitly given.
func TestInvokeSelectByFlag(t *testing.T) {
	fb := newFakeBackend(t)
	d := engineDescriptor(t)
	base := verbCall{entity: "t", op: "log", area: "t", verb: "log", child: "2000001", selfSvc: "1000001", selfPid: "1000002", given: map[string]any{"q": "x"}}
	if err := invoke(context.Background(), fb.do(), d, base, &strings.Builder{}, true); err != nil {
		t.Fatal(err)
	}
	if _, ok := fb.seen["/a"]; !ok {
		t.Errorf("without --alt the base op must be called")
	}
	withAlt := base
	withAlt.given = map[string]any{"q": "x", "alt": "y"}
	if err := invoke(context.Background(), fb.do(), d, withAlt, &strings.Builder{}, true); err != nil {
		t.Fatal(err)
	}
	if r, ok := fb.seen["/b"]; !ok || !strings.Contains(r.query, "alt=y") {
		t.Errorf("with --alt the branch op must be called with the param: %+v", fb.seen["/b"])
	}
}

// one_of and requires are enforced on explicitly given flags before any request.
func TestInvokeOneOfAndRequires(t *testing.T) {
	fb := newFakeBackend(t)
	d := engineDescriptor(t)
	mk := func(given map[string]any) verbCall {
		return verbCall{entity: "t", op: "where", area: "t", verb: "where", selfSvc: "1000001", selfPid: "1000002", given: given}
	}
	for _, c := range []struct {
		name  string
		given map[string]any
		want  string
	}{
		{"none given", map[string]any{}, "exactly one of"},
		{"both groups", map[string]any{"lat": 1.0, "lon": 2.0, "address": "x"}, "exactly one of"},
		{"lat without lon", map[string]any{"lat": 1.0}, "requires --lon"},
	} {
		err := invoke(context.Background(), fb.do(), d, mk(c.given), &strings.Builder{}, true)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want %q, got %v", c.name, c.want, err)
		}
	}
	if _, sent := fb.seen["/w"]; sent {
		t.Error("a refused invocation must not be sent")
	}
	if err := invoke(context.Background(), fb.do(), d, mk(map[string]any{"address": "1 Main St"}), &strings.Builder{}, true); err != nil {
		t.Fatalf("a complete group must pass: %v", err)
	}
	if !strings.Contains(fb.seen["/w"].query, "address=1+Main+St") {
		t.Errorf("query = %q", fb.seen["/w"].query)
	}
}

func childCall(op, verb string, given map[string]any) verbCall {
	return verbCall{entity: "t", op: op, area: "t", verb: verb, child: "2000001", selfSvc: "1000001", selfPid: "1000002", given: given}
}

// A resolver variable as a query value: location history / calls list send the child's
// profileId that the account read resolved, next to the flag-mapped, transformed date.
func TestInvokeResolverVarInQuery(t *testing.T) {
	fb := newFakeBackend(t)
	d := engineDescriptor(t)
	if err := invoke(context.Background(), fb.do(), d, childCall("hist", "hist", map[string]any{"since": "2026-09-01"}), &strings.Builder{}, true); err != nil {
		t.Fatal(err)
	}
	q := fb.seen["/h"].query
	if !strings.Contains(q, "profileId=3000001") || !strings.Contains(q, "startDate=2026-09-01T00%3A00%3A00.000000Z") {
		t.Errorf("query = %q", q)
	}
}

// A resolver variable as a header value: a contextual timezone header the op declares is
// filled from the local zone with no user flag.
func TestInvokeResolverVarInHeader(t *testing.T) {
	fb := newFakeBackend(t)
	d := engineDescriptor(t)
	if err := invoke(context.Background(), fb.do(), d, childCall("chores", "chores", nil), &strings.Builder{}, true); err != nil {
		t.Fatal(err)
	}
	if tz := fb.seen["/c"].headers.Get("timezone"); tz == "" || tz == "$local.timezone" {
		t.Errorf("timezone header = %q, want the local zone", tz)
	}
}

// Unkeyed singleton lookups: a flag default read from the current record, and an exists:
// condition that picks PUT (with the id filled) when the child already has a limit, POST
// (no id) when it does not.
func TestInvokeUnkeyedLookupDefaultAndExists(t *testing.T) {
	fb := newFakeBackend(t)
	fb.extra["/acct"] = func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"familyName":"Rivera"}`)) }
	d := engineDescriptor(t)
	// no existing limit -> base op POST, no id, name defaulted from the account read
	fb.extra["/stget"] = func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) }
	if err := invoke(context.Background(), fb.do(), d, childCall("stset", "stset", nil), &strings.Builder{}, true); err != nil {
		t.Fatal(err)
	}
	if r := fb.seen["/st"]; r.method != "POST" || !strings.Contains(r.body, `"name":"Rivera"`) || strings.Contains(r.body, "screenTimeLimitId") {
		t.Errorf("create branch wrong: %+v", r)
	}
	// existing limit -> branch op PUT with the id resolved from the singleton read
	delete(fb.seen, "/st")
	fb.extra["/stget"] = func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"screenTimeLimitId":55,"weeklyLimits":{}}`))
	}
	if err := invoke(context.Background(), fb.do(), d, childCall("stset", "stset", map[string]any{"name": "n2"}), &strings.Builder{}, true); err != nil {
		t.Fatal(err)
	}
	if r := fb.seen["/st"]; r.method != "PUT" || !strings.Contains(r.body, `"screenTimeLimitId":55`) || !strings.Contains(r.body, `"name":"n2"`) {
		t.Errorf("update branch wrong: %+v", r)
	}
}

// A filter: flag on an ACCOUNT verb never touches the request (the caller's own service id
// stays in the target header) and filters the response client-side.
func TestInvokeFilterFlagOnAccountVerb(t *testing.T) {
	fb := newFakeBackend(t)
	fb.extra["/dash"] = func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"members":[{"serviceId":2000001,"name":"A"},{"serviceId":2000002,"name":"S"}]}`))
	}
	d := engineDescriptor(t)
	vc := verbCall{entity: "t", op: "dash", area: "t", verb: "dash", child: "2000001", selfSvc: "1000001", selfPid: "1000002", given: map[string]any{"child": int64(2000001)}}
	var out strings.Builder
	if err := invoke(context.Background(), fb.do(), d, vc, &out, true); err != nil {
		t.Fatal(err)
	}
	if got := fb.seen["/dash"].headers.Get("x-fp-identifier-target-serviceid"); got != "1000001" {
		t.Errorf("an account verb must keep the caller's service id in the target header, got %q", got)
	}
	if !strings.Contains(out.String(), `"A"`) || strings.Contains(out.String(), `"S"`) {
		t.Errorf("response should be filtered to serviceId 2000001:\n%s", out.String())
	}
}

// Two flags that exclude each other cannot both be given; either alone works and feeds
// the one body var they share.
func TestInvokeExcludesAndSharedVar(t *testing.T) {
	fb := newFakeBackend(t)
	d := engineDescriptor(t)
	err := invoke(context.Background(), fb.do(), d, childCall("stime", "stime", map[string]any{"weekdays": int64(60), "mon": int64(30)}), &strings.Builder{}, true)
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("want an excludes error, got %v", err)
	}
	if _, sent := fb.seen["/stime"]; sent {
		t.Error("nothing may be sent after an excludes refusal")
	}
	if err := invoke(context.Background(), fb.do(), d, childCall("stime", "stime", map[string]any{"mon": int64(30)}), &strings.Builder{}, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fb.seen["/stime"].body, `"mon":30`) {
		t.Errorf("body = %s", fb.seen["/stime"].body)
	}
}
