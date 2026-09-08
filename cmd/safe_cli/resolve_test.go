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

// syntheticAccount mirrors the real getAccountDetails shape (accounts[].accountId,
// userprofiles[].services[]) with synthetic ids: one guardian, one PAIRED child, one
// UNPAIRED child — listed in the API's own (unsorted) order to exercise sortMembers.
const syntheticAccount = `{"accounts":[{"accountId":7000001,"userprofiles":[
  {"userProfileId":3000002,"profileName":"Sam","services":[{"serviceId":2000002,"userProfileId":3000002,"roleName":"DEPENDENT","deviceId":4000002,"pairingStatus":"UNPAIRED"}]},
  {"userProfileId":1000002,"profileName":"You","services":[{"serviceId":1000001,"userProfileId":1000002,"roleName":"GUARDIAN","deviceId":1000003}]},
  {"userProfileId":3000001,"profileName":"Alex","services":[{"serviceId":2000001,"userProfileId":3000001,"roleName":"DEPENDENT","deviceId":4000001,"pairingStatus":"PAIRED"}]}
]}]}`

func TestParseAccountRolesSortAndIDs(t *testing.T) {
	a, err := parseAccount([]byte(syntheticAccount))
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != 7000001 {
		t.Errorf("account id = %d, want 7000001", a.ID)
	}
	// Order: guardian, PAIRED child, UNPAIRED child (D3: the actionable child comes first).
	var order []string
	for _, m := range a.Members {
		order = append(order, m.Name+"/"+m.Role+"/"+m.Pairing)
	}
	want := []string{"You/guardian/", "Alex/child/PAIRED", "Sam/child/UNPAIRED"}
	if strings.Join(order, " ") != strings.Join(want, " ") {
		t.Errorf("order = %v, want %v", order, want)
	}
	// ROLE is the parent's word, not the API's; is_child is explicit.
	if a.Members[1].Role != "child" || !a.Members[1].IsChild || a.Members[0].IsChild {
		t.Errorf("role labels wrong: %+v", a.Members)
	}
	if a.Members[1].ProfileID != 3000001 || a.Members[1].DeviceID != 4000001 {
		t.Errorf("child ids wrong: %+v", a.Members[1])
	}
}

func TestResolveTarget(t *testing.T) {
	a, _ := parseAccount([]byte(syntheticAccount))
	m, err := a.resolveTarget("2000001")
	if err != nil || m.Name != "Alex" || m.ProfileID != 3000001 || m.DeviceID != 4000001 || !m.paired() {
		t.Fatalf("resolve 2000001 = %+v, %v", m, err)
	}
	if m, err := a.resolveTarget(" 2000002 "); err != nil || m.Name != "Sam" || m.paired() {
		t.Errorf("resolve unpaired child = %+v, %v", m, err)
	}
	// A name is not a target: the error points at members / --find, never guesses.
	if _, err := a.resolveTarget("Alex"); err == nil || !strings.Contains(err.Error(), "SERVICE-ID") || !strings.Contains(err.Error(), "--find") {
		t.Errorf("name as --child should be refused with guidance, got %v", err)
	}
	// An unknown id lists the family.
	if _, err := a.resolveTarget("9999999"); err == nil || !strings.Contains(err.Error(), "2000001") || !strings.Contains(err.Error(), "Alex") {
		t.Errorf("unknown id should list members, got %v", err)
	}
}

// fetchAccount makes exactly one getAccountDetails read per process, targeted with the
// caller's own service id, and caches it — a second call must not hit the backend.
func TestFetchAccountReadsOnceAndCaches(t *testing.T) {
	accountCache = nil
	t.Cleanup(func() { accountCache = nil })
	d, _ := descriptor.Default()
	hits := 0
	var gotSvc, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		gotSvc, gotPath = r.Header.Get("x-fp-identifier-target-serviceid"), r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(syntheticAccount))
	}))
	defer srv.Close()
	cl := client.New("T")
	cl.BaseURL = srv.URL
	a, err := fetchAccount(context.Background(), cl.DoH, d, "1000001", "")
	if err != nil {
		t.Fatal(err)
	}
	if gotSvc != "1000001" || !strings.Contains(gotPath, "userprofiles") {
		t.Errorf("targeted svc=%q path=%q", gotSvc, gotPath)
	}
	if _, err := fetchAccount(context.Background(), cl.DoH, d, "1000001", ""); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Errorf("account read should be cached: %d hits", hits)
	}
	if len(a.Members) != 3 {
		t.Errorf("members = %d", len(a.Members))
	}
}

// A backend error on the account read is reported, not swallowed into an empty family.
func TestFetchAccountReportsHTTPError(t *testing.T) {
	accountCache = nil
	t.Cleanup(func() { accountCache = nil })
	d, _ := descriptor.Default()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":[{"details":"no permissions"}]}`))
	}))
	defer srv.Close()
	cl := client.New("T")
	cl.BaseURL = srv.URL
	if _, err := fetchAccount(context.Background(), cl.DoH, d, "1000001", ""); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("want a 403 error, got %v", err)
	}
}
