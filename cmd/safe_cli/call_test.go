package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ejc3/safe_cli/internal/client"
	"github.com/ejc3/safe_cli/internal/descriptor"
)

func TestResolveOp(t *testing.T) {
	d, _ := descriptor.Default()
	if _, err := resolveOp(d, "account", "info"); err != nil {
		t.Errorf("account info: %v", err)
	}
	if _, err := resolveOp(d, "web_filter", "block_site"); err != nil { // an action
		t.Errorf("web_filter block_site (action): %v", err)
	}
	if _, err := resolveOp(d, "nope", "info"); err == nil {
		t.Error("want error for unknown entity")
	}
	if _, err := resolveOp(d, "account", "nope"); err == nil {
		t.Error("want error for unknown op")
	}
}

func TestFillPath(t *testing.T) {
	// placeholder substitution
	if got, err := fillPath("/frisco/v8/devices/{deviceId}/appsSync", "deviceId", "D1"); err != nil || got != "/frisco/v8/devices/D1/appsSync" {
		t.Errorf("substitute: %q %v", got, err)
	}
	// placeholder present but no id
	if _, err := fillPath("/x/{deviceId}", "deviceId", ""); err == nil {
		t.Error("want error: placeholder but no id")
	}
	// fixed path + id -> query param
	if got, err := fillPath("/frisco/v7/filterContent", "profileId", "P1"); err != nil || got != "/frisco/v7/filterContent?profileId=P1" {
		t.Errorf("query append: %q %v", got, err)
	}
	// fixed path + existing query -> &
	if got, _ := fillPath("/x?a=b", "profileId", "P1"); got != "/x?a=b&profileId=P1" {
		t.Errorf("query &: %q", got)
	}
	// no id, fixed path -> unchanged
	if got, err := fillPath("/vsf/account-management/v1/accounts", "accountId", ""); err != nil || got != "/vsf/account-management/v1/accounts" {
		t.Errorf("no id: %q %v", got, err)
	}
	// unrelated unfilled placeholder
	if _, err := fillPath("/x/{other}", "profileId", ""); err == nil {
		t.Error("want error for unfilled placeholder")
	}
}

func TestRunCallGET(t *testing.T) {
	d, _ := descriptor.Default()
	var gotPath, gotAuth, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accounts":[{"id":"acc1"}]}`))
	}))
	defer srv.Close()
	cl := client.New("THE.ID.TOKEN")
	cl.BaseURL = srv.URL

	var out strings.Builder
	if err := runCall(context.Background(), cl, d, "account", "info", "", "", &out, true); err != nil {
		t.Fatalf("runCall: %v", err)
	}
	if gotMethod != "GET" || gotAuth != "THE.ID.TOKEN" {
		t.Errorf("method=%q auth=%q", gotMethod, gotAuth)
	}
	if gotPath != "/vsf/account-management/v1/accounts" {
		t.Errorf("path = %q", gotPath)
	}
	if !strings.Contains(out.String(), `"accounts"`) {
		t.Errorf("output = %q", out.String())
	}
}

func TestRunCallAppendsIDQuery(t *testing.T) {
	d, _ := descriptor.Default()
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	cl := client.New("T")
	cl.BaseURL = srv.URL
	if err := runCall(context.Background(), cl, d, "web_filter", "get", "PROF1", "", &strings.Builder{}, true); err != nil {
		t.Fatalf("runCall: %v", err)
	}
	if !strings.Contains(gotPath, "profileId=PROF1") {
		t.Errorf("path %q missing profileId query", gotPath)
	}
}

func TestRunCallErrorStatus(t *testing.T) {
	d, _ := descriptor.Default()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"nope"}`))
	}))
	defer srv.Close()
	cl := client.New("T")
	cl.BaseURL = srv.URL
	err := runCall(context.Background(), cl, d, "account", "info", "", "", &strings.Builder{}, true)
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("want a 403 error, got %v", err)
	}
}

func TestRunCallRejectsBadData(t *testing.T) {
	d, _ := descriptor.Default()
	cl := client.New("T")
	if err := runCall(context.Background(), cl, d, "web_filter", "block_site", "P1", "{not json", &strings.Builder{}, true); err == nil {
		t.Error("want error for invalid --data JSON")
	}
}
