package descriptor

import "testing"

// From the navigation test: these ops answer 403 to a guardian token on this (phone-child,
// no-Gizmo) account for a correctly-targeted call — a product/device the account lacks, a
// device-originated read, or a Family-Line SPC-gated management op. They must be marked
// unavailable so `describe` shows ✗ and an agent skips them instead of probing (issue #75a).
// Each op's whole (method,path) route is gated, so the route invariant holds.
func TestNavGatedOpsMarkedUnavailable(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	gated := map[string][]string{
		"family_line":        {"getFamilyLines", "getEligibleLines", "getAddress"},
		"real_time_tracking": {"getHistoryEvents"},
		"pairing":            {"getGizmoDevices", "gizmoImportEligibility", "getDeviceStatus", "getMediaBackupStorageStatus", "getDeviceLogs", "getDeviceSettings"},
		"activity_tracking":  {"getActivity", "getDailyActivities"},
		"device_settings":    {"getDeviceLogs", "getDeviceSettings"},
	}
	for ent, ops := range gated {
		for _, op := range ops {
			o, ok := d.Entities[ent].Operations[op]
			if !ok {
				t.Errorf("%s.%s missing", ent, op)
				continue
			}
			if o.Available() {
				t.Errorf("%s.%s must be marked unavailable (403 for a guardian token on this account)", ent, op)
			}
		}
	}
	// getProvisioningStatus (the plain-id_token status route) must STAY available — it works
	// with the guardian's service id; only the SPC-gated management ops are disabled.
	if !d.Entities["family_line"].Operations["getProvisioningStatus"].Available() {
		t.Error("family_line.getProvisioningStatus must stay available (works with the guardian service id)")
	}
}
