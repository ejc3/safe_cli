package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

func TestDoSendsRawIDTokenAndHeaders(t *testing.T) {
	var gotAuth, gotSrc, gotVer, gotTxn, gotUA string
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotSrc = r.Header.Get("x-source-app")
		gotVer = r.Header.Get("x-mobile-app-version")
		gotTxn = r.Header.Get("x-transaction-id")
		gotUA = r.Header.Get("user-agent")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New("THE.ID.TOKEN")
	c.BaseURL = srv.URL
	resp, err := c.Do(context.Background(), "POST", "/x", []byte(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != 200 || string(resp.Body) != `{"ok":true}` {
		t.Fatalf("resp = %d %s", resp.Status, resp.Body)
	}
	// Authorization is the raw id_token — NOT "Bearer ...".
	if gotAuth != "THE.ID.TOKEN" {
		t.Errorf("Authorization = %q, want raw id_token with no Bearer prefix", gotAuth)
	}
	if gotSrc != "AndroidMAPP" || gotVer != AppVersion || gotUA != "okhttp/4.12.0" {
		t.Errorf("headers: src=%q ver=%q ua=%q", gotSrc, gotVer, gotUA)
	}
	// x-transaction-id now matches the app: a decimal integer up to 40 digits.
	if !regexp.MustCompile(`^\d{1,40}$`).MatchString(gotTxn) {
		t.Errorf("x-transaction-id = %q, want a decimal (<=40 digits)", gotTxn)
	}
	if gotBody != `{"a":1}` {
		t.Errorf("body = %q", gotBody)
	}
}

func TestDoRequiresToken(t *testing.T) {
	c := New("")
	if _, err := c.Do(context.Background(), "GET", "/x", nil); err == nil {
		t.Fatal("expected an error when no id_token is set")
	}
}
