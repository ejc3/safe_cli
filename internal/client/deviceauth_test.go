package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	daKey     = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	daAppUUID = "c9ce8abc-2e84-3e8e-81bd-07557dd60015"
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
	resp, err := c.RequestDeviceOTP(context.Background(), "5551234567", daKey, daAppUUID)
	if err != nil {
		t.Fatalf("RequestDeviceOTP: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("status = %d", resp.Status)
	}
	if gotPath != pathDeviceOTP {
		t.Errorf("path = %q, want %q", gotPath, pathDeviceOTP)
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
	res, err := c.ValidateDeviceOTP(context.Background(), "5551234567", "071480", daKey, daAppUUID)
	if err != nil {
		t.Fatalf("ValidateDeviceOTP: %v", err)
	}
	if gotPath != pathDeviceOTPValidate {
		t.Errorf("path = %q, want %q", gotPath, pathDeviceOTPValidate)
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
	if _, err := c.ValidateDeviceOTP(context.Background(), "5551234567", "000000", daKey, daAppUUID); err == nil {
		t.Fatal("want error when no login_recom_token is returned")
	}
}
