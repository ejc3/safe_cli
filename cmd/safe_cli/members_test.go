package main

import "testing"

// parseMembers must flatten the nested account-details shape (accounts[].userprofiles[].
// services[]) into one addressable row per service, with the role and pairing an agent
// keys off. Synthetic values only — no real account data.
func TestParseMembers(t *testing.T) {
	body := []byte(`{"accounts":[{"accountId":111,"familyName":"Test","userprofiles":[
		{"userProfileId":201,"profileName":"Parent","services":[
			{"serviceId":9001,"userProfileId":201,"roleName":"GUARDIAN","deviceId":8001,"pairingStatus":"PAIRED","planName":"Family Plus"}]},
		{"userProfileId":202,"profileName":"Kid","services":[
			{"serviceId":9002,"userProfileId":202,"roleName":"DEPENDENT","deviceId":8002,"pairingStatus":"UNPAIRED","planName":"Family Plus"}]},
		{"userProfileId":203,"profileName":"NoService","services":[]}
	]}]}`)
	ms, err := parseMembers(body)
	if err != nil {
		t.Fatalf("parseMembers: %v", err)
	}
	if len(ms) != 2 {
		t.Fatalf("got %d members, want 2 (the service-less profile yields no row)", len(ms))
	}
	kid := ms[1]
	if kid.Name != "Kid" || kid.Role != "DEPENDENT" || kid.ServiceID != 9002 ||
		kid.ProfileID != 202 || kid.DeviceID != 8002 || kid.Pairing != "UNPAIRED" {
		t.Errorf("dependent row = %+v", kid)
	}
	if ms[0].Role != "GUARDIAN" || ms[0].ServiceID != 9001 {
		t.Errorf("guardian row = %+v", ms[0])
	}
}

// A profile whose service omits its own userProfileId falls back to the profile's id, so
// the ProfileID column is always populated.
func TestParseMembersProfileIDFallback(t *testing.T) {
	body := []byte(`{"accounts":[{"userprofiles":[
		{"userProfileId":777,"profileName":"K","services":[{"serviceId":9,"roleName":"DEPENDENT"}]}]}]}`)
	ms, err := parseMembers(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 1 || ms[0].ProfileID != 777 {
		t.Errorf("want ProfileID fallback to 777, got %+v", ms)
	}
}
