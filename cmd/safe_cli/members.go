package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ejc3/safe_cli/internal/outfmt"
	"github.com/ejc3/safe_cli/internal/tokenstore"
)

// membersCmd lists the family members on the account with the identifiers an agent needs to
// target them — the service id (the near-universal --service-id for parental-control ops),
// the profile and device ids, the role (GUARDIAN vs DEPENDENT), and pairing status. It is
// the intended first call: it takes no arguments (it targets the account with the logged-in
// user's own service id, read from the token) and answers "what can I act on, and how?".
type membersCmd struct{}

// member is one family member's addressable identity, flattened from the nested account
// details so an agent can read the fields it passes to `call` directly off each row.
type member struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	ServiceID int64  `json:"service_id"`
	ProfileID int64  `json:"profile_id"`
	DeviceID  int64  `json:"device_id,omitempty"`
	Pairing   string `json:"pairing,omitempty"`
	Plan      string `json:"plan,omitempty"`
}

func (c *membersCmd) Run(rc *runContext) error {
	st, ts, err := loadTokens()
	if err != nil {
		return err
	}
	idt, ok := ts.IDToken()
	if !ok {
		return fmt.Errorf("no id_token in the stored tokens; run `safe_cli auth login`")
	}
	// Any family service id returns the whole account, so target it with the logged-in
	// user's own service id (from the token) — that is why this command needs no flags.
	sid := tokenstore.Claims(idt)["custom:identifier-serviceid"]
	if sid == "" {
		return fmt.Errorf("no service id in the stored token; run `safe_cli auth login`")
	}
	appUUID, _ := resolveAppUUID(ts)
	op, err := resolveOp(rc.D, "account", "getAccountDetails")
	if err != nil {
		return err
	}
	headers := map[string]string{"x-fp-identifier-target-serviceid": sid}
	if appUUID != "" {
		headers["x-fp-identifier-app-uuid"] = appUUID
	}
	do := authedRequest(rc, st, ts)
	resp, err := do(context.Background(), op.Method, op.Path, nil, headers)
	if err != nil {
		return err
	}
	if resp.Status >= 400 {
		return fmt.Errorf("HTTP %d fetching account details: %s", resp.Status, resp.Body)
	}
	members, err := parseMembers(resp.Body)
	if err != nil {
		return err
	}
	if rc.G.JSON {
		return outfmt.JSON(rc.Out, members)
	}
	rows := make([][]string, 0, len(members))
	for _, m := range members {
		dev := ""
		if m.DeviceID != 0 {
			dev = fmt.Sprintf("%d", m.DeviceID)
		}
		rows = append(rows, []string{
			m.Name, m.Role, fmt.Sprintf("%d", m.ServiceID),
			fmt.Sprintf("%d", m.ProfileID), dev, m.Pairing,
		})
	}
	if err := outfmt.Table(rc.Out, []string{"NAME", "ROLE", "SERVICE-ID", "PROFILE-ID", "DEVICE-ID", "PAIRING"}, rows); err != nil {
		return err
	}
	_, err = fmt.Fprintln(rc.Out, "\nPass SERVICE-ID as --service-id to child-scoped ops (a DEPENDENT is a managed child). "+
		"Device-scoped ops (pause, contacts) need a paired device.")
	return err
}

// parseMembers flattens the account-details response into one row per (profile, service):
// the fields an agent addresses a member by. It reads defensively — a member missing a
// service simply yields no row — so a partial response never errors the whole listing.
func parseMembers(body []byte) ([]member, error) {
	var doc struct {
		Accounts []struct {
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
	var out []member
	for _, a := range doc.Accounts {
		for _, p := range a.UserProfiles {
			for _, s := range p.Services {
				pid := s.UserProfileID
				if pid == 0 {
					pid = p.UserProfileID
				}
				out = append(out, member{
					Name:      p.ProfileName,
					Role:      s.RoleName,
					ServiceID: s.ServiceID,
					ProfileID: pid,
					DeviceID:  s.DeviceID,
					Pairing:   s.PairingStatus,
					Plan:      s.PlanName,
				})
			}
		}
	}
	return out, nil
}
