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
	// The engine keeps the account read real and dumps only the final request.
	vc := pauseCall(map[string]any{"for": "2h"}, "2000001")
	vc.dryRun = true
	vc.dump = dump
	var out strings.Builder
	if err := invoke(context.Background(), fb.do(), d, vc, &out, false); err != nil {
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
  "listA":{"method":"GET","path":"/a","query":["q","alt","cat"]},
  "listB":{"method":"GET","path":"/b","query":["q","alt"]},
  "cats":{"method":"GET","path":"/cats","headers":["x-fp-identifier-target-serviceid"]},
  "post":{"method":"POST","path":"/p","takes_body":true,"headers":["x-pending-activation","x-name"],"query":["q"],
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
        {"name":"name","type":"string","default":"dflt","maps_to":"header:x-name","help":"h"}]}},
  "alerts":{"method":"GET","path":"/al",
    "cli":{"area":"t","verb":"alerts","priority":"core","target":"account","summary":"s",
      "select":[{"when":"child","op":"t.listA","target":"child"}],
      "flags":[{"name":"alt","type":"string","maps_to":"query:alt","help":"h"}]}},
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
  "acct":{"method":"GET","path":"/acct","headers":["(@HeaderMap dynamic)"]},
  "stget":{"method":"GET","path":"/stget","headers":["x-fp-identifier-target-serviceid"]},
  "stput":{"method":"PUT","path":"/st","takes_body":true,"headers":["x-fp-identifier-target-serviceid"]},
  "stset":{"method":"POST","path":"/st","takes_body":true,"headers":["x-fp-identifier-target-serviceid"],
    "cli":{"area":"t","verb":"stset","priority":"core","target":"child","summary":"s",
      "body_template":"{\"name\":\"$name\"}","resolve":["$lookup:t.acct::familyName"],
      "select":[{"when":"exists:$lookup:t.stget::screenTimeLimitId","op":"t.stput","body_template":"{\"name\":\"$name\",\"screenTimeLimitId\":\"$lookup:t.stget::screenTimeLimitId\"}","resolve":["$lookup:t.stget::screenTimeLimitId","$lookup:t.acct::familyName"]}],
      "flags":[{"name":"name","type":"string","default":"$lookup:t.acct::familyName","maps_to":"body:$name","help":"h"}]}},
  "dash":{"method":"GET","path":"/dash","headers":["x-fp-identifier-target-serviceid"],
    "cli":{"area":"t","verb":"dash","priority":"core","target":"account","summary":"s",
      "flags":[{"name":"member","type":"int","maps_to":"filter:serviceId","help":"h"}]}},
  "stime":{"method":"POST","path":"/stime","takes_body":true,"headers":["x-fp-identifier-target-serviceid"],
    "cli":{"area":"t","verb":"stime","priority":"core","target":"child","summary":"s",
      "body_template":"{\"mon\":\"$mon?\"}",
      "flags":[{"name":"weekdays","type":"int","excludes":["mon"],"maps_to":"body:$mon","help":"h"},{"name":"mon","type":"int","excludes":["weekdays"],"maps_to":"body:$mon","help":"h"}]}},
  "hdr":{"method":"GET","path":"/hdr","headers":["x-fp-identifier-target-serviceid"],
    "cli":{"area":"t","verb":"hdr","priority":"core","target":"account","summary":"s",
      "select":[{"when":"child","op":"t.hdrb","target":"child"}],
      "flags":[{"name":"acc","type":"string","maps_to":"header:accept-x","help":"h"},{"name":"accd","type":"string","default":"x","maps_to":"header:x-accd","help":"h"}]}},
  "hdrb":{"method":"GET","path":"/hdrb","headers":["accept-x","x-accd","x-fp-identifier-target-serviceid"]},
  "numq":{"method":"GET","path":"/numq","query":["limit","kind"],
    "cli":{"area":"t","verb":"numq","priority":"core","target":"account","summary":"s",
      "flags":[{"name":"limit","type":"int","default":1000000,"maps_to":"query:limit","help":"h"},{"name":"kind","type":"enum","enum":["a","b"],"repeatable":true,"maps_to":"query:kind","help":"h"}]}},
  "nq":{"method":"POST","path":"/nq","takes_body":true,"query":["mode"],
    "cli":{"area":"t","verb":"nq","priority":"core","target":"account","summary":"s",
      "body_template":"{\"for\":\"$for?\",\"inf\":\"$inf\"}","query":{"mode":"$for"},
      "flags":[{"name":"for","type":"string","default":"30m","maps_to":"body:$for","help":"h"},{"name":"inf","type":"bool","default":false,"maps_to":"body:$inf","nulls":["for"],"help":"h"}]}},
  "kmsi":{"method":"POST","path":"/kmsi","takes_body":true,"inject_caller_app_uuid":true,
    "cli":{"area":"t","verb":"kmsi","priority":"core","target":"self","summary":"s",
      "body_template":"{\"kmsiEnabled\":\"$on\",\"app_uuid\":\"<device-uuid>\",\"triggeredBy\":\"user\"}",
      "constants":{"app_uuid":"<device-uuid>","triggeredBy":"user"},
      "flags":[{"name":"on","type":"bool","default":true,"maps_to":"body:$on","help":"h"}]}},
  "exk":{"method":"GET","path":"/exk","headers":["x-fp-identifier-target-serviceid"],
    "cli":{"area":"t","verb":"exk","priority":"core","target":"child","summary":"s",
      "select":[{"when":"exists:$lookup:t.cats:id=cat:name","op":"t.listA"}],
      "flags":[{"name":"cat","type":"int","maps_to":"filter:role","help":"h"}]}},
  "rq":{"method":"GET","path":"/rq","query":["a"],"required_query":["a"],
    "cli":{"area":"t","verb":"rq","priority":"core","target":"account","summary":"s",
      "flags":[{"name":"a","type":"string","required":true,"maps_to":"query:a","help":"h"}]}},
  "tbl":{"method":"GET","path":"/tbl","headers":["x-fp-identifier-target-serviceid"],
    "cli":{"area":"t","verb":"tbl","priority":"core","target":"child","summary":"s","output":{"table":["name","status"]}}},
  "alo":{"method":"POST","path":"/alo","takes_body":true,
    "cli":{"area":"t","verb":"alo","priority":"core","target":"account","summary":"s",
      "body_template":"{\"on\":\"$on?\",\"name\":\"$name?\"}","at_least_one":["on","name"],
      "flags":[{"name":"on","type":"bool","maps_to":"body:$on","help":"h"},{"name":"name","type":"string","maps_to":"body:$name","help":"h"}]}},
  "where2":{"method":"GET","path":"/w2","query":["lat","lon","address"],
    "cli":{"area":"t","verb":"where2","priority":"core","target":"account","summary":"s",
      "one_of":[["lat","lon"],["address"]],
      "flags":[{"name":"lat","type":"float","maps_to":"query:lat","help":"h"},{"name":"lon","type":"float","maps_to":"query:lon","help":"h"},{"name":"address","type":"string","maps_to":"query:address","help":"h"}]}},
  "rep":{"method":"POST","path":"/rep","takes_body":true,
    "cli":{"area":"t","verb":"rep","priority":"core","target":"account","summary":"s",
      "body_template":"{\"items\":[{\"s\":\"$s\"}]}",
      "flags":[{"name":"s","type":"enum","enum":["allow","block"],"repeatable":true,"required":true,"maps_to":"body:$s","transform":"allow_block_ab","help":"h"}]}},
  "nest":{"method":"POST","path":"/nest","takes_body":true,
    "cli":{"area":"t","verb":"nest","priority":"core","target":"account","summary":"s",
      "body_template":"{\"profiles\":[{\"name\":\"p\",\"domains\":[{\"url\":\"$url\"}]}]}","constants":{"name":"p"},
      "flags":[{"name":"url","type":"string","repeatable":true,"required":true,"maps_to":"body:$url","help":"h"}]}},
  "arr":{"method":"GET","path":"/arr","headers":["x-fp-identifier-target-serviceid"],
    "cli":{"area":"t","verb":"arr","priority":"core","target":"child","summary":"s"}},
  "kdef":{"method":"POST","path":"/kdef","takes_body":true,"headers":["x-fp-identifier-target-serviceid"],
    "cli":{"area":"t","verb":"kdef","priority":"core","target":"child","summary":"s",
      "body_template":"{\"n\":\"$lookup:t.cats:id=cat:name\",\"cat\":\"$cat\"}","resolve":["$lookup:t.cats:id=cat:name"],
      "flags":[{"name":"cat","type":"int","default":10003,"maps_to":"body:$cat","help":"h"}]}},
  "kexd":{"method":"GET","path":"/kexd","query":["cat"],"headers":["x-fp-identifier-target-serviceid"],
    "cli":{"area":"t","verb":"kexd","priority":"core","target":"child","summary":"s",
      "select":[{"when":"exists:$lookup:t.cats:id=cat:name","op":"t.listA"}],
      "flags":[{"name":"cat","type":"int","default":10003,"maps_to":"query:cat","help":"h"}]}},
  "optrep":{"method":"POST","path":"/optrep","takes_body":true,
    "cli":{"area":"t","verb":"optrep","priority":"core","target":"account","summary":"s",
      "body_template":"{\"domains\":[{\"url\":\"$url?\"}]}",
      "flags":[{"name":"url","type":"string","repeatable":true,"maps_to":"body:$url","help":"h"}]}},
  "sub":{"method":"POST","path":"/sub","takes_body":true,"headers":["x-fp-identifier-target-serviceid"],
    "cli":{"area":"t","verb":"sub","priority":"core","target":"child","summary":"s",
      "body_template":"{\"n\":\"$lookup:t.cats/Apps & websites:id=app:name\",\"id\":\"$app\"}","resolve":["$lookup:t.cats/Apps & websites:id=app:name"],
      "flags":[{"name":"app","type":"int","required":true,"maps_to":"body:$app","help":"h"}]}},
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
	// Codex #69-4: the account read wraps its record ({"accounts":[{...}]}); the singleton
	// lookup must find the object that carries the field, not stop at the wrapper.
	fb.extra["/acct"] = func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"accounts":[{"familyName":"Rivera"}]}`))
	}
	d := engineDescriptor(t)
	// no existing limit -> base op POST, no id, name defaulted from the account read
	fb.extra["/stget"] = func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) }
	if err := invoke(context.Background(), fb.do(), d, childCall("stset", "stset", nil), &strings.Builder{}, true); err != nil {
		t.Fatal(err)
	}
	if r := fb.seen["/st"]; r.method != "POST" || !strings.Contains(r.body, `"name":"Rivera"`) || strings.Contains(r.body, "screenTimeLimitId") {
		t.Errorf("create branch wrong: %+v", r)
	}
	// Codex #69-5: the lookup op declares the decompiler's dynamic header map, so the
	// identity headers must be forwarded rather than dropped.
	if got := fb.seen["/acct"].headers.Get("x-fp-identifier-target-serviceid"); got != "2000001" {
		t.Errorf("a dynamic-header lookup must carry the target header, got %q", got)
	}
	// existing limit -> branch op PUT with the id resolved from the singleton read
	delete(fb.seen, "/st")
	reads := 0
	fb.extra["/stget"] = func(w http.ResponseWriter, _ *http.Request) {
		reads++
		_, _ = w.Write([]byte(`{"screenTimeLimitId":55,"weeklyLimits":{}}`))
	}
	if err := invoke(context.Background(), fb.do(), d, childCall("stset", "stset", map[string]any{"name": "n2"}), &strings.Builder{}, true); err != nil {
		t.Fatal(err)
	}
	if r := fb.seen["/st"]; r.method != "PUT" || !strings.Contains(r.body, `"screenTimeLimitId":55`) || !strings.Contains(r.body, `"name":"n2"`) {
		t.Errorf("update branch wrong: %+v", r)
	}
	// Codex #69-6: the exists: condition and the resolve entry name the same lookup; one
	// invocation reads it once so selection and rendering share a snapshot.
	if reads != 1 {
		t.Errorf("the singleton was read %d times in one invocation, want 1", reads)
	}
}

// Codex #69-3: a "$flag" reference in the verb's query map takes the flag's descriptor
// default when the flag is not given (the flag's primary destination is a header here).
func TestInvokeQueryTemplateUsesFlagDefault(t *testing.T) {
	fb := newFakeBackend(t)
	fb.extra["/cats"] = func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"categories":[{"id":10003,"name":"Games"}]}`))
	}
	d := engineDescriptor(t)
	vc := childCall("post", "do", map[string]any{"url": []string{"a.com"}, "cat": int64(10003)})
	if err := invoke(context.Background(), fb.do(), d, vc, &strings.Builder{}, true); err != nil {
		t.Fatal(err)
	}
	if r := fb.seen["/p"]; !strings.Contains(r.query, "q=dflt") || r.headers.Get("x-name") != "dflt" {
		t.Errorf("default must reach both destinations: query=%q x-name=%q", r.query, r.headers.Get("x-name"))
	}
}

// Codex #70-1 (engine side): a flag whose query param only the --child branch's op
// declares is refused, naming the selector, when given without --child — never dropped.
func TestInvokeBranchOnlyQueryFlagRefusedNotDropped(t *testing.T) {
	fb := newFakeBackend(t)
	d := engineDescriptor(t)
	noChild := verbCall{entity: "t", op: "alerts", area: "t", verb: "alerts", selfSvc: "1000001", selfPid: "1000002", given: map[string]any{"alt": "y"}}
	err := invoke(context.Background(), fb.do(), d, noChild, &strings.Builder{}, true)
	if err == nil || !strings.Contains(err.Error(), "--alt") || !strings.Contains(err.Error(), "--child") {
		t.Fatalf("want a refusal naming --alt and --child, got %v", err)
	}
	if _, sent := fb.seen["/al"]; sent {
		t.Error("the base op must not be called with the flag silently dropped")
	}
	withChild := noChild
	withChild.child = "2000001"
	if err := invoke(context.Background(), fb.do(), d, withChild, &strings.Builder{}, true); err != nil {
		t.Fatal(err)
	}
	if r, ok := fb.seen["/a"]; !ok || !strings.Contains(r.query, "alt=y") {
		t.Errorf("with --child the branch op must receive the param: %+v", fb.seen["/a"])
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
	vc := verbCall{entity: "t", op: "dash", area: "t", verb: "dash", child: "2000001", selfSvc: "1000001", selfPid: "1000002", given: map[string]any{"member": int64(2000001)}}
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

// Closure batch (verification sweep, engine):
//   - a header flag the chosen op does not declare is refused when given (naming the selector)
//     and dropped when it is only a default — the twin of the query guard;
//   - a bool given as false is absent for every presence rule (select flag:, excludes,
//     requires, one_of, at_least_one), as it already was for nulls;
//   - structural refusals (enum, at_least_one) happen before the account read;
//   - an int default renders as an integer in query/path/header, a repeatable enum is checked
//     per element, nulls also clears the flag's effective value, and one lookup op is read
//     once per invocation however many fields it feeds.
func TestInvokeHeaderGuardTwin(t *testing.T) {
	fb := newFakeBackend(t)
	d := engineDescriptor(t)
	noChild := verbCall{entity: "t", op: "hdr", area: "t", verb: "hdr", selfSvc: "1000001", selfPid: "1000002", given: map[string]any{"acc": "y"}}
	err := invoke(context.Background(), fb.do(), d, noChild, &strings.Builder{}, true)
	if err == nil || !strings.Contains(err.Error(), "--acc") || !strings.Contains(err.Error(), "--child") {
		t.Fatalf("an explicitly given header flag the chosen op does not take must be refused naming the selector, got %v", err)
	}
	if _, sent := fb.seen["/hdr"]; sent {
		t.Error("nothing may be sent after the refusal")
	}
	noChild.given = nil
	if err := invoke(context.Background(), fb.do(), d, noChild, &strings.Builder{}, true); err != nil {
		t.Fatal(err)
	}
	if r := fb.seen["/hdr"]; r.headers.Get("x-accd") != "" {
		t.Errorf("a defaulted header for another branch's op must be dropped on the base op, got %v", r.headers)
	}
	if err := invoke(context.Background(), fb.do(), d, childCall("hdr", "hdr", map[string]any{"acc": "y"}), &strings.Builder{}, true); err != nil {
		t.Fatal(err)
	}
	if r := fb.seen["/hdrb"]; r.headers.Get("accept-x") != "y" || r.headers.Get("x-accd") != "x" {
		t.Errorf("with --child the branch op must receive both headers: %v", r.headers)
	}
}

func TestInvokeFalseBoolIsAbsentForPresenceRules(t *testing.T) {
	fb := newFakeBackend(t)
	d := engineDescriptor(t)
	// select flag:alt with --alt=false selects the base op (switch semantics).
	base := verbCall{entity: "t", op: "log", area: "t", verb: "log", child: "2000001", selfSvc: "1000001", selfPid: "1000002", given: map[string]any{"q": "x", "alt": false}}
	if err := invoke(context.Background(), fb.do(), d, base, &strings.Builder{}, true); err != nil {
		t.Fatal(err)
	}
	if _, ok := fb.seen["/b"]; ok {
		t.Error("--alt=false must not select the alt branch")
	}
}

func TestInvokeStructuralRefusalsBeforeAccountRead(t *testing.T) {
	fb := newFakeBackend(t)
	d, _ := descriptor.Default()
	err := invoke(context.Background(), fb.do(), d, pauseCall(map[string]any{"for": "45m"}, "2000001"), &strings.Builder{}, true)
	if err == nil || !strings.Contains(err.Error(), "30m|1h|2h|4h|until-morning") {
		t.Fatalf("want the enum refusal, got %v", err)
	}
	for path := range fb.seen {
		if strings.Contains(path, "userprofiles") {
			t.Errorf("an enum miss must be refused before the account read (saw %s)", path)
		}
	}
}

func TestInvokeIntDefaultAndRepeatableEnumInQuery(t *testing.T) {
	fb := newFakeBackend(t)
	d := engineDescriptor(t)
	vc := verbCall{entity: "t", op: "numq", area: "t", verb: "numq", selfSvc: "1000001", selfPid: "1000002", given: map[string]any{"kind": []string{"a", "b"}}}
	if err := invoke(context.Background(), fb.do(), d, vc, &strings.Builder{}, true); err != nil {
		t.Fatalf("a repeatable enum must be checked per element: %v", err)
	}
	q := fb.seen["/numq"].query
	// list values join with commas (the betaProviders=RCS,GIZMO convention of renderQuery)
	if !strings.Contains(q, "limit=1000000") || strings.Contains(q, "e%2B06") || !strings.Contains(q, "kind=a%2Cb") {
		t.Errorf("query = %q (want an integer limit and the comma-joined kinds)", q)
	}
	vc.given = map[string]any{"kind": []string{"a", "zzz"}}
	if err := invoke(context.Background(), fb.do(), d, vc, &strings.Builder{}, true); err == nil || !strings.Contains(err.Error(), "zzz") {
		t.Errorf("a bad element of a repeatable enum must be refused naming it, got %v", err)
	}
}

func TestInvokeNullsClearsEffectiveValue(t *testing.T) {
	fb := newFakeBackend(t)
	d := engineDescriptor(t)
	vc := verbCall{entity: "t", op: "nq", area: "t", verb: "nq", selfSvc: "1000001", selfPid: "1000002", given: map[string]any{"inf": true}}
	if err := invoke(context.Background(), fb.do(), d, vc, &strings.Builder{}, true); err != nil {
		t.Fatal(err)
	}
	r := fb.seen["/nq"]
	if strings.Contains(r.body, `"for"`) || strings.Contains(r.query, "mode=") {
		t.Errorf("a nulled flag must vanish from the body AND the query map: body=%s query=%q", r.body, r.query)
	}
}

func TestInvokeOneReadPerLookupOp(t *testing.T) {
	fb := newFakeBackend(t)
	reads := 0
	fb.extra["/cats"] = func(w http.ResponseWriter, _ *http.Request) {
		reads++
		_, _ = w.Write([]byte(`{"categories":[{"id":"GAM","categoryId":1001,"subCategories":[{"id":10003,"name":"8 Ball"}]}]}`))
	}
	d := engineDescriptor(t)
	if err := invoke(context.Background(), fb.do(), d, childCall("post", "do", map[string]any{"url": []string{"a.com"}, "cat": int64(10003)}), &strings.Builder{}, true); err != nil {
		t.Fatal(err)
	}
	if reads != 1 {
		t.Errorf("three lookups on one op must read it once, read %d times", reads)
	}
}

// A bool given as false is absent for nulls too: --indefinite=false --for 1h is a timed pause.
func TestInvokeFalseBoolDoesNotNull(t *testing.T) {
	fb := newFakeBackend(t)
	d, _ := descriptor.Default()
	if err := invoke(context.Background(), fb.do(), d, pauseCall(map[string]any{"indefinite": false, "for": "1h"}, "2000001"), &strings.Builder{}, true); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if b := fb.seen[pausePath].body; !strings.Contains(b, `"pauseSchedule":"1_hour"`) || !strings.Contains(b, `"untilIUnpause":false`) {
		t.Errorf("--indefinite=false --for 1h must send a timed pause: %s", b)
	}
}

// An exists: condition keyed by an optional flag that was not given is simply not met: the
// base op runs and no lookup is attempted (Codex #70 round 7, engine side).
func TestInvokeExistsKeyedByAbsentFlagIsNotMet(t *testing.T) {
	fb := newFakeBackend(t)
	d := engineDescriptor(t)
	if err := invoke(context.Background(), fb.do(), d, childCall("exk", "exk", nil), &strings.Builder{}, true); err != nil {
		t.Fatalf("an absent key flag must not be an error: %v", err)
	}
	if _, base := fb.seen["/exk"]; !base {
		t.Error("the base op must run when the exists: key flag is absent")
	}
	if _, looked := fb.seen["/cats"]; looked {
		t.Error("no lookup may be attempted without its key")
	}
}

// A lookup scoped to a top-level subtree ("entity.op/field") finds records only there: an
// id that exists in another part of the read is a miss, never a silent cross-over
// (Codex #72: apps block must not reach the content categories).
func TestInvokeLookupSubtreeScopesTheSearch(t *testing.T) {
	fb := newFakeBackend(t)
	fb.extra["/cats"] = func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"categories":[{"id":"MORE","categoryId":4014,"subCategories":[{"id":30033,"name":"Downloads"}]}],"Apps & websites":[{"id":"GAM","categoryId":1006,"subCategories":[{"id":10061,"name":"8 Ball Pool"}]}]}`))
	}
	d := engineDescriptor(t)
	if err := invoke(context.Background(), fb.do(), d, childCall("sub", "sub", map[string]any{"app": int64(10061)}), &strings.Builder{}, true); err != nil {
		t.Fatal(err)
	}
	if b := fb.seen["/sub"].body; !strings.Contains(b, `"n":"8 Ball Pool"`) {
		t.Errorf("body = %s", b)
	}
	delete(fb.seen, "/sub")
	err := invoke(context.Background(), fb.do(), d, childCall("sub", "sub", map[string]any{"app": int64(30033)}), &strings.Builder{}, true)
	if err == nil || !strings.Contains(err.Error(), "30033") {
		t.Fatalf("an id outside the subtree must be a miss naming it, got %v", err)
	}
	// The hint names the op an agent can call, never the subtree (Codex #69 round 7).
	if hint := err.Error()[strings.LastIndex(err.Error(), "run `"):]; !strings.Contains(hint, "safe_cli call t cats`") || strings.Contains(hint, "cats/") {
		t.Errorf("the miss hint must name the plain op: %v", err)
	}
	if _, sent := fb.seen["/sub"]; sent {
		t.Error("nothing may be sent after a miss")
	}
}

// Codex #69 round 6:
//   - a keyed lookup (in resolve or an exists: condition) whose key flag has a default uses
//     that default when the flag is not given, as the schema promises;
//   - an optional repeatable flag that is not given expands its singleton array to [], not
//     to one element with the leaf removed.
func TestInvokeLookupKeyUsesFlagDefault(t *testing.T) {
	fb := newFakeBackend(t)
	fb.extra["/cats"] = func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"categories":[{"id":"GAM","categoryId":1001,"subCategories":[{"id":10003,"name":"8 Ball"}]}]}`))
	}
	d := engineDescriptor(t)
	if err := invoke(context.Background(), fb.do(), d, childCall("kdef", "kdef", nil), &strings.Builder{}, true); err != nil {
		t.Fatalf("a defaulted key must feed the lookup: %v", err)
	}
	if b := fb.seen["/kdef"].body; !strings.Contains(b, `"n":"8 Ball"`) || !strings.Contains(b, `"cat":10003`) {
		t.Errorf("body = %s", b)
	}
	if err := invoke(context.Background(), fb.do(), d, childCall("kexd", "kexd", nil), &strings.Builder{}, true); err != nil {
		t.Fatal(err)
	}
	if _, branch := fb.seen["/a"]; !branch {
		t.Error("an exists: condition keyed by a defaulted flag must be evaluated with the default (branch expected)")
	}
}

func TestInvokeAbsentRepeatableExpandsToEmptyArray(t *testing.T) {
	fb := newFakeBackend(t)
	d := engineDescriptor(t)
	vc := verbCall{entity: "t", op: "optrep", area: "t", verb: "optrep", selfSvc: "1000001", selfPid: "1000002"}
	if err := invoke(context.Background(), fb.do(), d, vc, &strings.Builder{}, true); err != nil {
		t.Fatal(err)
	}
	if b := fb.seen["/optrep"].body; !strings.Contains(b, `"domains":[]`) {
		t.Errorf("an absent repeatable flag must expand to an empty array, got %s", b)
	}
}

// Codex #69 round 5:
func TestInvokeRepeatableScalarTransformPerElement(t *testing.T) {
	fb := newFakeBackend(t)
	d := engineDescriptor(t)
	vc := verbCall{entity: "t", op: "rep", area: "t", verb: "rep", selfSvc: "1000001", selfPid: "1000002", given: map[string]any{"s": []string{"allow", "block"}}}
	if err := invoke(context.Background(), fb.do(), d, vc, &strings.Builder{}, true); err != nil {
		t.Fatalf("a scalar transform must apply to each repeated value: %v", err)
	}
	if b := fb.seen["/rep"].body; !strings.Contains(b, `{"s":"a"}`) || !strings.Contains(b, `{"s":"b"}`) {
		t.Errorf("body = %s", b)
	}
}

func TestInvokeRepeatExpandsNearestArray(t *testing.T) {
	fb := newFakeBackend(t)
	d := engineDescriptor(t)
	vc := verbCall{entity: "t", op: "nest", area: "t", verb: "nest", selfSvc: "1000001", selfPid: "1000002", given: map[string]any{"url": []string{"a.com", "b.com"}}}
	if err := invoke(context.Background(), fb.do(), d, vc, &strings.Builder{}, true); err != nil {
		t.Fatal(err)
	}
	b := fb.seen["/nest"].body
	if strings.Count(b, `"name":"p"`) != 1 || strings.Count(b, `"url"`) != 2 {
		t.Errorf("want ONE profile with two domains, got %s", b)
	}
}

func TestInvokeArrayResponseKeepsMeta(t *testing.T) {
	fb := newFakeBackend(t)
	fb.extra["/arr"] = func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`[{"a":1},{"a":2}]`)) }
	d := engineDescriptor(t)
	var out strings.Builder
	if err := invoke(context.Background(), fb.do(), d, childCall("arr", "arr", nil), &out, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "_meta") || !strings.Contains(out.String(), `"a": 2`) && !strings.Contains(out.String(), `"a":2`) {
		t.Errorf("--json must keep the array and add _meta: %s", out.String())
	}
}

// Codex #69 round 4:
//   - a required query param that renders empty (explicit "" or a lookup miss) is refused
//     before sending, as `call` already does;
//   - output.table renders one row per object of a listing response;
//   - an explicit --flag=false counts as provided for requires/excludes/one_of/at_least_one
//     (a value-style bool: --objectionable-alerts=false is a setting), while select flag:
//     and nulls keep switch semantics (false is absent);
//   - members of an unselected one_of alternative are refused, not silently mixed in;
//   - the caller's session uuid for body injection is separate from the header uuid.
func TestInvokeRefusesEmptyRequiredQuery(t *testing.T) {
	fb := newFakeBackend(t)
	d := engineDescriptor(t)
	vc := verbCall{entity: "t", op: "rq", area: "t", verb: "rq", selfSvc: "1000001", selfPid: "1000002", given: map[string]any{"a": ""}}
	err := invoke(context.Background(), fb.do(), d, vc, &strings.Builder{}, true)
	if err == nil || !strings.Contains(err.Error(), "a") {
		t.Fatalf("an empty required query value must be refused naming it, got %v", err)
	}
	if _, sent := fb.seen["/rq"]; sent {
		t.Error("nothing may be sent with a required query param missing")
	}
}

func TestInvokeTableRendersEveryRow(t *testing.T) {
	fb := newFakeBackend(t)
	fb.extra["/tbl"] = func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"devices":[{"name":"A","status":"Paused"},{"name":"B","status":"Unpaused"}]}`))
	}
	d := engineDescriptor(t)
	var out strings.Builder
	if err := invoke(context.Background(), fb.do(), d, childCall("tbl", "tbl", nil), &out, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Paused") || !strings.Contains(out.String(), "Unpaused") || !strings.Contains(out.String(), "B") {
		t.Errorf("every object of the listing must be a row:\n%s", out.String())
	}
}

func TestInvokeExplicitFalseCountsForAtLeastOne(t *testing.T) {
	fb := newFakeBackend(t)
	d := engineDescriptor(t)
	vc := verbCall{entity: "t", op: "alo", area: "t", verb: "alo", selfSvc: "1000001", selfPid: "1000002", given: map[string]any{"on": false}}
	if err := invoke(context.Background(), fb.do(), d, vc, &strings.Builder{}, true); err != nil {
		t.Fatalf("--on=false is a provided setting: %v", err)
	}
	if b := fb.seen["/alo"].body; !strings.Contains(b, `"on":false`) {
		t.Errorf("the explicit false must reach the body: %s", b)
	}
	vc.given = map[string]any{}
	if err := invoke(context.Background(), fb.do(), d, vc, &strings.Builder{}, true); err == nil || !strings.Contains(err.Error(), "at least one") {
		t.Errorf("nothing given must still be refused, got %v", err)
	}
}

func TestInvokeOneOfRefusesMembersOfOtherGroups(t *testing.T) {
	fb := newFakeBackend(t)
	d := engineDescriptor(t)
	vc := verbCall{entity: "t", op: "where2", area: "t", verb: "where2", selfSvc: "1000001", selfPid: "1000002", given: map[string]any{"address": "x", "lat": 1.0}}
	err := invoke(context.Background(), fb.do(), d, vc, &strings.Builder{}, true)
	if err == nil || !strings.Contains(err.Error(), "--lat") {
		t.Fatalf("a member of an unselected alternative must be refused naming it, got %v", err)
	}
	if _, sent := fb.seen["/w2"]; sent {
		t.Error("nothing may be sent with mixed alternatives")
	}
}

// Codex #69 round 3:
//   - --dry-run with no request dumper must never fall through to the live sender;
//   - a JSON null body still gets _meta under --json instead of a nil-map panic;
//   - an op marked inject_caller_app_uuid gets the caller's stored app-uuid in its body, as
//     `call` already does.
func TestInvokeDryRunNeverSendsWithoutADumper(t *testing.T) {
	fb := newFakeBackend(t)
	d, _ := descriptor.Default()
	vc := pauseCall(map[string]any{"for": "1h"}, "2000001")
	vc.dryRun = true // and vc.dump left nil
	err := invoke(context.Background(), fb.do(), d, vc, &strings.Builder{}, true)
	if err == nil {
		t.Fatal("a dry run without a dumper must be refused, not sent")
	}
	if _, sent := fb.seen[pausePath]; sent {
		t.Error("the live pause was sent under --dry-run")
	}
}

func TestInvokeJSONNullBodyGetsMeta(t *testing.T) {
	fb := newFakeBackend(t)
	fb.extra["/c"] = func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`null`)) }
	d := engineDescriptor(t)
	var out strings.Builder
	if err := invoke(context.Background(), fb.do(), d, childCall("chores", "chores", nil), &out, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "_meta") {
		t.Errorf("--json output must carry _meta even for a null body: %s", out.String())
	}
}

func TestInvokeInjectsCallerAppUUID(t *testing.T) {
	fb := newFakeBackend(t)
	d := engineDescriptor(t)
	// The header uuid (install fallback) is not the session uuid: without a session uuid
	// (a token set imported without app_uuid) the verb is refused — a generated verb has no
	// --data override to supply it, and the placeholder must never go out.
	vc := verbCall{entity: "t", op: "kmsi", area: "t", verb: "kmsi", selfSvc: "1000001", selfPid: "1000002", appUUID: "99999999-9999-9999-9999-999999999999"}
	err := invoke(context.Background(), fb.do(), d, vc, &strings.Builder{}, true)
	if err == nil || !strings.Contains(err.Error(), "app_uuid") {
		t.Fatalf("without a session uuid the verb must be refused naming app_uuid, got %v", err)
	}
	if _, sent := fb.seen["/kmsi"]; sent {
		t.Error("nothing may be sent with the placeholder unfilled")
	}
	vc.sessionUUID = "11111111-2222-3333-4444-555555555555"
	if err := invoke(context.Background(), fb.do(), d, vc, &strings.Builder{}, true); err != nil {
		t.Fatal(err)
	}
	if b := fb.seen["/kmsi"].body; !strings.Contains(b, `"app_uuid":"11111111-2222-3333-4444-555555555555"`) || strings.Contains(b, "<device-uuid>") {
		t.Errorf("the caller's session uuid must replace the placeholder: %s", b)
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
