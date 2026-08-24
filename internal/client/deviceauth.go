package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// RequestDeviceOTP triggers the SafePath line-verification SMS to mdn (factor 1,
// signed frisco call — docs/PROCESS.md §9 step 1). path is the descriptor's
// auth.endpoints.otp_request. A 200 with {"state":"OTP_SENT"} means the code was sent.
func (c *Client) RequestDeviceOTP(ctx context.Context, path, mdn, key, appUUID string) (*Response, error) {
	body, err := json.Marshal(map[string]string{"mdn": mdn})
	if err != nil {
		return nil, err
	}
	return c.SignedDo(ctx, "POST", path, body, key, appUUID)
}

// DeviceOTPResult is the parsed validate response.
type DeviceOTPResult struct {
	State           string // e.g. "AM_LOGIN_PAGE" — hand off to the hosted account login next
	LoginRecomToken string // the login_recom_token used to seed the account login
	ExpiresIn       int
	Raw             []byte
}

type tokenEnvelope struct {
	State  string `json:"state"`
	Tokens []struct {
		TokenType string `json:"token_type"`
		IDToken   string `json:"id_token"`
		ExpiresIn int    `json:"expires_in"`
	} `json:"tokens"`
}

// ValidateDeviceOTP submits the SMS code and returns the login_recom_token, which the
// assisted hosted-account login (steps 3-5) consumes. path is the descriptor's
// auth.endpoints.otp_validate. The backend answered 207 in the capture; 200 is accepted.
func (c *Client) ValidateDeviceOTP(ctx context.Context, path, mdn, otp, key, appUUID string) (*DeviceOTPResult, error) {
	body, err := json.Marshal(map[string]string{"mdn": mdn, "otp": otp})
	if err != nil {
		return nil, err
	}
	resp, err := c.SignedDo(ctx, "POST", path, body, key, appUUID)
	if err != nil {
		return nil, err
	}
	if resp.Status != http.StatusOK && resp.Status != http.StatusMultiStatus {
		return nil, fmt.Errorf("validate otp: status %d: %s", resp.Status, resp.Body)
	}
	var env tokenEnvelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, fmt.Errorf("parse validate response: %w", err)
	}
	res := &DeviceOTPResult{State: env.State, Raw: resp.Body}
	for _, tk := range env.Tokens {
		if tk.TokenType == "login_recom_token" {
			res.LoginRecomToken = tk.IDToken
			res.ExpiresIn = tk.ExpiresIn
			break
		}
	}
	if res.LoginRecomToken == "" {
		return nil, fmt.Errorf("no login_recom_token in validate response (state=%q)", env.State)
	}
	return res, nil
}
