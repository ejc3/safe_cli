package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ejc3/safe_cli/internal/descriptor"
)

// account is the parsed getAccountDetails read that `members` and the generated verbs
// share (docs/CLI-DESIGN.md §2): the family's members and the account id. It is the one
// read a verb makes to resolve --child into the profileId/deviceId/pairing an op's body
// needs, so the user never types them.
type account struct {
	ID      int64    `json:"account_id"`
	Members []member `json:"members"`
}

// accountCache holds the read for the life of the process: a CLI invocation is one
// command, and every verb that needs the family resolves it from the same response.
var accountCache *account

// fetchAccount performs the account read (getAccountDetails targeted with the caller's
// own service id — any family service id returns the whole account) once per process.
// do is the authenticated request function; selfServiceID comes from the id_token.
func fetchAccount(ctx context.Context, do doFunc, d *descriptor.Descriptor, selfServiceID, appUUID string) (*account, error) {
	if accountCache != nil {
		return accountCache, nil
	}
	op, err := resolveOp(d, "account", "getAccountDetails")
	if err != nil {
		return nil, err
	}
	headers := map[string]string{"x-fp-identifier-target-serviceid": selfServiceID}
	if appUUID != "" {
		headers["x-fp-identifier-app-uuid"] = appUUID
	}
	resp, err := do(ctx, op.Method, op.Path, nil, headers)
	if err != nil {
		return nil, err
	}
	if resp.Status >= 400 {
		return nil, fmt.Errorf("HTTP %d fetching account details: %s", resp.Status, strings.TrimSpace(string(resp.Body)))
	}
	a, err := parseAccount(resp.Body)
	if err != nil {
		return nil, err
	}
	accountCache = a
	return a, nil
}

// parseAccount flattens the account-details response into the account id and one member
// row per (profile, service), sorted the way `members` prints them (see sortMembers). It
// reads defensively — a member missing a service simply yields no row — so a partial
// response never errors the whole listing.
func parseAccount(body []byte) (*account, error) {
	var doc struct {
		Accounts []struct {
			AccountID    int64 `json:"accountId"`
			UserProfiles []struct {
				UserProfileID int64  `json:"userProfileId"`
				ProfileName   string `json:"profileName"`
				Services      []struct {
					ServiceID     int64  `json:"serviceId"`
					UserProfileID int64  `json:"userProfileId"`
					RoleName      string `json:"roleName"`
					DeviceID      int64  `json:"deviceId"`
					PairingStatus string `json:"pairingStatus"`
					PlanName      string `json:"planName"`
				} `json:"services"`
			} `json:"userprofiles"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse account details: %w", err)
	}
	a := &account{}
	for _, acc := range doc.Accounts {
		if a.ID == 0 {
			a.ID = acc.AccountID
		}
		for _, p := range acc.UserProfiles {
			for _, s := range p.Services {
				pid := s.UserProfileID
				if pid == 0 {
					pid = p.UserProfileID
				}
				a.Members = append(a.Members, member{
					Name:      p.ProfileName,
					Role:      roleLabel(s.RoleName),
					IsChild:   isChildRole(s.RoleName),
					ServiceID: s.ServiceID,
					ProfileID: pid,
					DeviceID:  s.DeviceID,
					Pairing:   s.PairingStatus,
					Plan:      s.PlanName,
				})
			}
		}
	}
	sortMembers(a.Members)
	return a, nil
}

// roleLabel renders the API's roleName as the word a parent uses: GUARDIAN -> guardian,
// DEPENDENT -> child (the baseline probe had to infer "dependent = child"; now nothing is
// inferred). Any other value passes through lowercased so a new role is still visible.
func roleLabel(apiRole string) string {
	switch strings.ToUpper(apiRole) {
	case "DEPENDENT":
		return "child"
	case "GUARDIAN":
		return "guardian"
	}
	return strings.ToLower(apiRole)
}

func isChildRole(apiRole string) bool { return strings.EqualFold(apiRole, "DEPENDENT") }

// sortMembers fixes the order `members` prints and `members --help` states: guardians
// first, then children with PAIRED before UNPAIRED (the actionable ones first), then by
// name — so "the first child" is well-defined, and a paused-on-unpaired mistake is less
// likely (docs/CLI-DESIGN.md §9, D3).
func sortMembers(ms []member) {
	rank := func(m member) int {
		switch {
		case !m.IsChild:
			return 0
		case strings.EqualFold(m.Pairing, "PAIRED"):
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(ms, func(i, j int) bool {
		ri, rj := rank(ms[i]), rank(ms[j])
		if ri != rj {
			return ri < rj
		}
		return strings.ToLower(ms[i].Name) < strings.ToLower(ms[j].Name)
	})
}

// resolveTarget finds the member a verb acts on by the SERVICE-ID `members` prints
// (--child). A miss lists the family so the caller can pick the right id; it never
// guesses by name (name lookup is `members --find`, a separate step the agent chains).
func (a *account) resolveTarget(childFlag string) (member, error) {
	svc, err := strconv.ParseInt(strings.TrimSpace(childFlag), 10, 64)
	if err != nil {
		return member{}, fmt.Errorf("--child %q must be a SERVICE-ID (run `safe_cli members`; to find one by name, `safe_cli members --find <text>`)", childFlag)
	}
	for _, m := range a.Members {
		if m.ServiceID == svc {
			if !m.IsChild {
				return member{}, fmt.Errorf("--child %d is %s, a guardian, not a child: child-scoped verbs act on a managed child (run `safe_cli members --role child`)", svc, m.Name)
			}
			return m, nil
		}
	}
	ids := make([]string, 0, len(a.Members))
	for _, m := range a.Members {
		ids = append(ids, fmt.Sprintf("%d (%s, %s)", m.ServiceID, m.Name, m.Role))
	}
	return member{}, fmt.Errorf("--child %d is not a member of this account; members: %s", svc, strings.Join(ids, "; "))
}

// paired reports whether the member's device is PAIRED — the precondition every
// device-scoped verb checks (a pause on an UNPAIRED device is a no-op the API accepts).
func (m member) paired() bool { return strings.EqualFold(m.Pairing, "PAIRED") }
