package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ejc3/safe_cli/internal/client"
	"github.com/ejc3/safe_cli/internal/descriptor"
)

func TestResolveOp(t *testing.T) {
	d, _ := descriptor.Default()
	if _, err := resolveOp(d, "account", "getAccountDetails"); err != nil {
		t.Errorf("account getAccountDetails: %v", err)
	}
	if _, err := resolveOp(d, "content_filter", "getFilterContent"); err != nil {
		t.Errorf("content_filter getFilterContent: %v", err)
	}
	if _, err := resolveOp(d, "nope", "getAccountDetails"); err == nil {
		t.Error("want error for unknown entity")
	}
	if _, err := resolveOp(d, "account", "nope"); err == nil {
		t.Error("want error for unknown op")
	}
}

func TestFillPath(t *testing.T) {
	// placeholder substitution
	if got, err := fillPath("/frisco/v8/devices/{deviceId}/appsSync", "deviceId", "D1", nil); err != nil || got != "/frisco/v8/devices/D1/appsSync" {
		t.Errorf("substitute: %q %v", got, err)
	}
	// placeholder present but no id
	if _, err := fillPath("/x/{deviceId}", "deviceId", "", nil); err == nil {
		t.Error("want error: placeholder but no id")
	}
	// fixed path + id -> query param
	if got, err := fillPath("/frisco/v7/filterContent", "profileId", "P1", nil); err != nil || got != "/frisco/v7/filterContent?profileId=P1" {
		t.Errorf("query append: %q %v", got, err)
	}
	// fixed path that already has a query -> append with '&'
	if got, err := fillPath("/frisco/v7/filterContent?scope=all", "profileId", "P1", nil); err != nil || got != "/frisco/v7/filterContent?scope=all&profileId=P1" {
		t.Errorf("amp append: %q %v", got, err)
	}
	// multi-placeholder path: the leading {id_field} is filled but another remains -> error
	if _, err := fillPath("/groups/{groupId}/members/{memberId}", "groupId", "G1", nil); err == nil {
		t.Error("want error: path still has an unfilled placeholder")
	}
	// no id, fixed path -> unchanged
	if got, err := fillPath("/vsf/account-management/v1/accounts", "accountId", "", nil); err != nil || got != "/vsf/account-management/v1/accounts" {
		t.Errorf("no id: %q %v", got, err)
	}
	// multi-placeholder path: positional id fills {id_field}, --path fills the rest
	if got, err := fillPath("/g/{group-id}/m/{member-id}", "group-id", "G1", map[string]string{"member-id": "M2"}); err != nil || got != "/g/G1/m/M2" {
		t.Errorf("multi-placeholder: %q %v", got, err)
	}
}

// An op that needs no x-fp-identifier header just sends the id_token.
func TestRunCallGET(t *testing.T) {
	d, _ := descriptor.Default()
	var gotPath, gotAuth, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotMethod = r.URL.RequestURI(), r.Header.Get("Authorization"), r.Method
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"account":{"id":"acc1"}}`))
	}))
	defer srv.Close()
	cl := client.New("THE.ID.TOKEN")
	cl.BaseURL = srv.URL

	var out strings.Builder
	if err := runCall(context.Background(), cl.DoH, d, callArgs{entity: "account", op: "getAccountDetails"}, &out, true); err != nil {
		t.Fatalf("runCall: %v", err)
	}
	if gotMethod != "GET" || gotAuth != "THE.ID.TOKEN" {
		t.Errorf("method=%q auth=%q", gotMethod, gotAuth)
	}
	if gotPath != "/account/fam/userprofile-management/v8/accounts/userprofiles" {
		t.Errorf("path = %q", gotPath)
	}
	if !strings.Contains(out.String(), `"account"`) {
		t.Errorf("output = %q", out.String())
	}
}

// A parental-control op sends the x-fp-identifier-target-serviceid header when a service id
// is supplied.
func TestRunCallSendsServiceIDHeader(t *testing.T) {
	d, _ := descriptor.Default()
	var gotSvc string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSvc = r.Header.Get("x-fp-identifier-target-serviceid")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	cl := client.New("T")
	cl.BaseURL = srv.URL
	idh := map[string]string{"x-fp-identifier-target-serviceid": "1234567"}
	if err := runCall(context.Background(), cl.DoH, d, callArgs{entity: "content_filter", op: "getFilterContent", idHeaders: idh}, &strings.Builder{}, true); err != nil {
		t.Fatalf("runCall: %v", err)
	}
	if gotSvc != "1234567" {
		t.Errorf("x-fp-identifier-target-serviceid = %q, want 1234567", gotSvc)
	}
}

// If an op needs a service id and none is supplied, it errors up front with guidance
// rather than sending a request that would 403.
func TestRunCallMissingServiceID(t *testing.T) {
	d, _ := descriptor.Default()
	cl := client.New("T") // no server: must fail before any request
	err := runCall(context.Background(), cl.DoH, d, callArgs{entity: "content_filter", op: "getFilterContent"}, &strings.Builder{}, true)
	if err == nil || !strings.Contains(err.Error(), "service-id") {
		t.Errorf("want a --service-id guidance error, got %v", err)
	}
}

func TestRunCallErrorStatus(t *testing.T) {
	d, _ := descriptor.Default()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":[{"details":"no permissions on this serviceId"}]}`))
	}))
	defer srv.Close()
	cl := client.New("T")
	cl.BaseURL = srv.URL
	idh := map[string]string{"x-fp-identifier-target-serviceid": "1"}
	err := runCall(context.Background(), cl.DoH, d, callArgs{entity: "content_filter", op: "getFilterContent", idHeaders: idh}, &strings.Builder{}, true)
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("want a 403 error, got %v", err)
	}
}

func TestRunCallRejectsBadData(t *testing.T) {
	d, _ := descriptor.Default()
	cl := client.New("T")
	idh := map[string]string{"x-fp-identifier-target-serviceid": "1"}
	if err := runCall(context.Background(), cl.DoH, d, callArgs{entity: "content_filter", op: "setCFCategories", data: "{not json", idHeaders: idh}, &strings.Builder{}, true); err == nil {
		t.Error("want error for invalid --data JSON")
	}
}

func TestBuildBodyMergesUserOverDefault(t *testing.T) {
	b, err := buildBody(map[string]any{"paused": true}, `{"foo":"bar","paused":false}`)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["foo"] != "bar" || m["paused"] != false { // user overrides the default
		t.Errorf("merged = %v, want foo=bar paused=false", m)
	}
}

// identityHeaders fills the service id and app-uuid from the args and the profile id from
// the id_token claims, and emits only header names some op declares.
func TestIdentityHeaders(t *testing.T) {
	// {"custom:identifier-profileid":"7654321"}
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"custom:identifier-profileid":"7654321"}`))
	jwt := "hdr." + payload + ".sig"
	m := identityHeaders(jwt, "1234567", "", "", "APP-UUID-1")
	if m["x-fp-identifier-target-serviceid"] != "1234567" {
		t.Errorf("service id = %q", m["x-fp-identifier-target-serviceid"])
	}
	if m["x-fp-identifier-profileid"] != "7654321" {
		t.Errorf("profile id from claims = %q", m["x-fp-identifier-profileid"])
	}
	if m["x-fp-identifier-app-uuid"] != "APP-UUID-1" {
		t.Errorf("app-uuid = %q", m["x-fp-identifier-app-uuid"])
	}
	// A header name no op declares must never be emitted.
	if _, ok := m["x-fp-identifier-mdn"]; ok {
		t.Error("x-fp-identifier-mdn must not be emitted (no op declares it)")
	}
}

// runCall appends provided query parameters to the request URL — the mechanism ops whose
// required inputs live in Operation.Query (e.g. accessibility_pin validatePin's pin) need
// to be callable at all.
func TestRunCallAppendsQuery(t *testing.T) {
	d, _ := descriptor.Default()
	var gotURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURI = r.URL.RequestURI()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	cl := client.New("T")
	cl.BaseURL = srv.URL
	q := map[string]string{"pin": "1234", "foo": "b ar"}
	if err := runCall(context.Background(), cl.DoH, d, callArgs{entity: "account", op: "getAccountDetails", query: q}, &strings.Builder{}, true); err != nil {
		t.Fatalf("runCall: %v", err)
	}
	if !strings.Contains(gotURI, "pin=1234") || !strings.Contains(gotURI, "foo=b+ar") {
		t.Errorf("query not appended to URL: %q", gotURI)
	}
}

// An op that declares x-trace-transaction-id gets a fresh UUID generated for it — the app
// sends one, and newAppRequest only adds x-transaction-id automatically.
func TestRunCallGeneratesTraceHeader(t *testing.T) {
	d, _ := descriptor.Default()
	var gotTrace, gotSvc string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTrace = r.Header.Get("x-trace-transaction-id")
		gotSvc = r.Header.Get("x-fp-identifier-target-serviceid")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	cl := client.New("T")
	cl.BaseURL = srv.URL
	idh := map[string]string{"x-fp-identifier-target-serviceid": "svc"}
	if err := runCall(context.Background(), cl.DoH, d, callArgs{entity: "device_settings", op: "getDeviceLogs", idHeaders: idh}, &strings.Builder{}, true); err != nil {
		t.Fatalf("runCall: %v", err)
	}
	if gotSvc != "svc" {
		t.Errorf("service id header = %q", gotSvc)
	}
	if len(gotTrace) < 32 || !strings.Contains(gotTrace, "-") {
		t.Errorf("x-trace-transaction-id not generated (got %q)", gotTrace)
	}
}

// runCall refuses an op whose path is a runtime-resolved @Url placeholder rather than
// requesting the literal placeholder text from the production host.
func TestRunCallRejectsDynamicURL(t *testing.T) {
	d, err := descriptor.Parse([]byte(`{"name":"t","base_url":"https://h","entities":{"config":{"id_field":"","operations":{"getX":{"method":"GET","path":"/(dynamic @Url)"}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	// No server: the guard must fire before any request is attempted.
	err = runCall(context.Background(), client.New("T").DoH, d, callArgs{entity: "config", op: "getX"}, &strings.Builder{}, true)
	if err == nil || !strings.Contains(err.Error(), "@Url") {
		t.Errorf("want an @Url rejection error, got %v", err)
	}
}

// A multi-placeholder path is fully constructed from the positional id ({group-id}) plus
// --path name=value for the rest ({member-id}) — otherwise these ops are uncallable.
func TestRunCallFillsNamedPathParams(t *testing.T) {
	d, _ := descriptor.Default()
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	cl := client.New("T")
	cl.BaseURL = srv.URL
	args := callArgs{
		entity: "messaging", op: "deleteGroupMember", id: "G1",
		idHeaders:  map[string]string{"x-fp-identifier-target-serviceid": "svc"},
		pathParams: map[string]string{"member-id": "M2"},
	}
	if err := runCall(context.Background(), cl.DoH, d, args, &strings.Builder{}, true); err != nil {
		t.Fatalf("runCall: %v", err)
	}
	if gotMethod != "DELETE" || gotPath != "/account/fam/group-management/v5/groups/G1/members/M2" {
		t.Errorf("method=%q path=%q", gotMethod, gotPath)
	}
}

// An arbitrary --header a op needs (e.g. timezone, schedule-type) is sent even when no
// dedicated flag covers it.
func TestRunCallSendsUserHeader(t *testing.T) {
	d, _ := descriptor.Default()
	var gotTZ string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTZ = r.Header.Get("timezone")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	cl := client.New("T")
	cl.BaseURL = srv.URL
	args := callArgs{entity: "account", op: "getAccountDetails", userHeaders: map[string]string{"timezone": "UTC"}}
	if err := runCall(context.Background(), cl.DoH, d, args, &strings.Builder{}, true); err != nil {
		t.Fatalf("runCall: %v", err)
	}
	if gotTZ != "UTC" {
		t.Errorf("timezone header = %q, want UTC", gotTZ)
	}
}

// A mutating op (POST) sends its method, the merged JSON body, and the required identity
// header — the write pathways behave the same as reads through the one dispatcher.
func TestRunCallPostsBody(t *testing.T) {
	d, _ := descriptor.Default()
	var gotMethod, gotBody, gotSvc string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotMethod, gotBody, gotSvc = r.Method, string(b), r.Header.Get("x-fp-identifier-target-serviceid")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	cl := client.New("T")
	cl.BaseURL = srv.URL
	args := callArgs{
		entity: "content_filter", op: "setCFCategories", data: `{"category":"social","allowed":false}`,
		idHeaders: map[string]string{"x-fp-identifier-target-serviceid": "svc"},
	}
	if err := runCall(context.Background(), cl.DoH, d, args, &strings.Builder{}, true); err != nil {
		t.Fatalf("runCall: %v", err)
	}
	if gotMethod != "POST" || gotSvc != "svc" || !strings.Contains(gotBody, `"category":"social"`) {
		t.Errorf("method=%q svc=%q body=%q", gotMethod, gotSvc, gotBody)
	}
}
