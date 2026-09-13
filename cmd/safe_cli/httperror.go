package main

import (
	"fmt"
	"strings"
)

// httpError turns a non-2xx backend response into an error that tells a blind caller (an
// agent) what to do next, instead of dumping raw JSON. The raw body is kept for debugging,
// but the leading guidance is keyed on the status so "this feature/device isn't on my
// account" is distinguishable from "I built the request wrong" (issue #75). Most 4xx seen in
// the navigation test were 403s for ops the account is not provisioned for.
func httpError(status int, body []byte) error {
	raw := strings.TrimSpace(string(body))
	var hint string
	switch status {
	case 401:
		hint = "the backend rejected the credentials. The stored session may be stale — try `safe_cli auth refresh` — or the op needs a token this CLI does not hold."
	case 403:
		hint = "the backend refused this. Usually the target's account or device is not provisioned for this feature, or the op needs a child/device token you do not hold. " +
			"Check `safe_cli describe <entity>` (ops known to be unavailable are marked ✗ with the reason) and confirm --service-id points at the intended child."
	case 404:
		hint = "the resource does not exist for this target. An id (event, device, member) may be wrong or the feature is not set up for this child; list it first with the matching read op."
	case 400:
		hint = "the backend rejected the request shape. Check the required query/body for this op with `safe_cli describe <entity>` (required params are marked with *)."
	default:
		if status >= 500 {
			hint = "the backend failed (server-side); retry later."
		}
	}
	if hint == "" {
		return fmt.Errorf("HTTP %d: %s", status, raw)
	}
	return fmt.Errorf("HTTP %d: %s\n(%s)", status, raw, hint)
}
