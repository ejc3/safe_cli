package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	daKey           = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	daAppUUID       = "c9ce8abc-2e84-3e8e-81bd-07557dd60015"
	otpPath         = "/auth/frisco/frisco-iam-device-auth/v7/user/auth/otp"
	otpValidatePath = "/auth/frisco/frisco-iam-device-auth/v7/user/auth/otp/validate"
	exchangePath    = "/auth/frisco/frisco-iam-device-auth/v7/user/auth/token"
)

func TestRequestDeviceOTP(t *testing.T) {
	var gotPath, gotBody, gotSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotSig = r.Header.Get("x-signature")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"state":"OTP_SENT","statusCode":200}`))
	}))
	defer srv.Close()

	c := New("")
	c.BaseURL = srv.URL
	resp, err := c.RequestDeviceOTP(context.Background(), otpPath, "5551234567", daKey, daAppUUID)
	if err != nil {
		t.Fatalf("RequestDeviceOTP: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d", resp.Status)
	}
	if gotPath != otpPath {
		t.Errorf("path = %q, want %q", gotPath, otpPath)
	}
	if gotBody != `{"mdn":"5551234567"}` {
		t.Errorf("body = %q", gotBody)
	}
	if len(gotSig) != 64 {
		t.Errorf("x-signature len = %d, want a signed request", len(gotSig))
	}
}

func TestValidateDeviceOTP(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`{"state":"AM_LOGIN_PAGE","tokens":[{"token_type":"login_recom_token","id_token":"RECOM.JWT.VALUE","expires_in":1800}]}`))
	}))
	defer srv.Close()

	c := New("")
	c.BaseURL = srv.URL
	res, err := c.ValidateDeviceOTP(context.Background(), otpValidatePath, "5551234567", "071480", daKey, daAppUUID)
	if err != nil {
		t.Fatalf("ValidateDeviceOTP: %v", err)
	}
	if gotPath != otpValidatePath {
		t.Errorf("path = %q, want %q", gotPath, otpValidatePath)
	}
	var body map[string]string
	if err := json.Unmarshal([]byte(gotBody), &body); err != nil || body["mdn"] != "5551234567" || body["otp"] != "071480" {
		t.Errorf("body = %q", gotBody)
	}
	if res.State != "AM_LOGIN_PAGE" {
		t.Errorf("state = %q", res.State)
	}
	if res.LoginRecomToken != "RECOM.JWT.VALUE" || res.ExpiresIn != 1800 {
		t.Errorf("token = %q exp=%d", res.LoginRecomToken, res.ExpiresIn)
	}
}

func TestValidateDeviceOTPNoToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"state":"INVALID_OTP","tokens":[]}`))
	}))
	defer srv.Close()

	c := New("")
	c.BaseURL = srv.URL
	if _, err := c.ValidateDeviceOTP(context.Background(), otpValidatePath, "5551234567", "000000", daKey, daAppUUID); err == nil {
		t.Fatal("want error when no login_recom_token is returned")
	}
}

func TestExchangeCode(t *testing.T) {
	var gotPath, gotAuth, gotSig string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotSig = r.Header.Get("x-signature")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tokens":[
			{"frisco_token_type":"online","id_token":"ID1","access_token":"AC1","refresh_token":"RT_ONLINE","expires_in":1800},
			{"frisco_token_type":"offline","id_token":"ID2","refresh_token":"RT_OFFLINE","expires_in":86400}
		]}`))
	}))
	defer srv.Close()

	c := New("")
	c.BaseURL = srv.URL
	ts, err := c.ExchangeCode(context.Background(), CodeExchange{
		Path: exchangePath, Code: "AUTHCODE", Verifier: "VERIFIER",
		ClientID: "CID", RedirectURI: "vsfapp://x/signin", AppUUID: daAppUUID, Token: "RECOM",
	})
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if gotPath != exchangePath {
		t.Errorf("path = %q, want %q", gotPath, exchangePath)
	}
	// The code exchange is neither authenticated nor signed.
	if gotAuth != "" {
		t.Errorf("Authorization = %q, want none on the token exchange", gotAuth)
	}
	if gotSig != "" {
		t.Errorf("x-signature = %q, want none on the token exchange", gotSig)
	}
	// Confirmed-live camelCase body: grantType, codeVerifier, and the recom token that
	// fuses the two factors — with no snake_case leftovers from the old best-guess body.
	if gotBody["grantType"] != "authorization_code" || gotBody["code"] != "AUTHCODE" ||
		gotBody["codeVerifier"] != "VERIFIER" || gotBody["token"] != "RECOM" ||
		gotBody["identityProvider"] != "vz-am-provider" || gotBody["friscoTokenType"] != "online" {
		t.Errorf("body missing expected camelCase fields: %v", gotBody)
	}
	if _, snake := gotBody["code_verifier"]; snake {
		t.Errorf("body must be camelCase, found snake_case code_verifier: %v", gotBody)
	}
	if len(ts.Tokens) != 2 {
		t.Fatalf("tokens = %d, want 2 (online + offline)", len(ts.Tokens))
	}
	var offline string
	for _, tk := range ts.Tokens {
		if tk.FriscoTokenType == "offline" {
			offline = tk.RefreshToken
		}
	}
	if offline != "RT_OFFLINE" {
		t.Errorf("offline refresh_token = %q, want RT_OFFLINE", offline)
	}
}

func TestExchangeCodeRequiresCodeAndVerifier(t *testing.T) {
	c := New("")
	if _, err := c.ExchangeCode(context.Background(), CodeExchange{Path: exchangePath, Verifier: "V", Token: "R"}); err == nil {
		t.Error("want error without a code")
	}
	if _, err := c.ExchangeCode(context.Background(), CodeExchange{Path: exchangePath, Code: "C", Token: "R"}); err == nil {
		t.Error("want error without a verifier")
	}
	// The loginRecommendation token is required too (it fuses the device + web factors).
	if _, err := c.ExchangeCode(context.Background(), CodeExchange{Path: exchangePath, Code: "C", Verifier: "V"}); err == nil {
		t.Error("want error without the loginRecommendation token")
	}
}

// A set with only an online refresh_token (no offline entry) is not durable-login
// usable, so it must be rejected — not accepted just because *a* refresh_token exists.
func TestExchangeCodeRequiresOfflineRefreshToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tokens":[{"frisco_token_type":"online","id_token":"ID1","refresh_token":"RT_ONLINE","expires_in":1800}]}`))
	}))
	defer srv.Close()
	c := New("")
	c.BaseURL = srv.URL
	if _, err := c.ExchangeCode(context.Background(), CodeExchange{Path: exchangePath, Code: "C", Verifier: "V", Token: "R"}); err == nil {
		t.Error("want error when the response has no OFFLINE refresh_token, even if an online one exists")
	}
}

// The device-OTP request returns 2xx for both success and failure and distinguishes them
// by "state"; only OTP_SENT actually dispatched a code. A non-sent state (e.g.
// OTP_DISPATCH_FAILED / OTP_ATTEMPTS_EXCEEDED) must be surfaced as an error rather than
// reported as "SMS sent". This fails against the old code, which returned no error for a
// 200 regardless of state.
func TestRequestDeviceOTPFailsWhenNotSent(t *testing.T) {
	for _, state := range []string{"OTP_DISPATCH_FAILED", "OTP_ATTEMPTS_EXCEEDED", "OTP_NOT_FOUND", ""} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"state":"` + state + `","statusCode":200}`))
		}))
		c := New("")
		c.BaseURL = srv.URL
		_, err := c.RequestDeviceOTP(context.Background(), otpPath, "5551234567", daKey, daAppUUID)
		if err == nil {
			t.Errorf("state=%q: want an error (no code was sent), got nil", state)
		} else if !strings.Contains(err.Error(), state) && state != "" {
			t.Errorf("state=%q: error %q should name the state", state, err)
		}
		srv.Close()
	}
}

// A 200 with an empty token array (or an error envelope) must not be returned as a
// successful token set. postTokenRequest now rejects "no tokens" as a centralized
// invariant; the callers (ExchangeCode/Refresh) also reject it downstream, so this is a
// regression guard on that invariant rather than a fix for a caller-visible gap.
func TestExchangeCodeRejectsEmptyTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tokens":[]}`))
	}))
	defer srv.Close()
	c := New("")
	c.BaseURL = srv.URL
	if _, err := c.ExchangeCode(context.Background(), CodeExchange{Path: exchangePath, Code: "C", Verifier: "V", Token: "R"}); err == nil {
		t.Error("want error when the token response contains no tokens")
	}
}
