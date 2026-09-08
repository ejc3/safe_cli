package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/ejc3/safe_cli/internal/outfmt"
	"github.com/ejc3/safe_cli/internal/tokenstore"
)

// membersCmd lists the family members on the account with the identifiers an agent needs to
// target them — the SERVICE-ID (the target for child-scoped ops), the profile and device
// ids, the role, and pairing status. It is the intended first call: it takes no target (it
// reads the account with the logged-in user's own service id, from the token) and answers
// "what can I act on, and how?". --find and --role are the one lookup convenience
// (docs/CLI-DESIGN.md §8): finding a child's id by name is this separate step, never
// fuzzy matching inside a verb.
type membersCmd struct {
	Find string `name:"find" help:"Only members whose name contains this text (case-insensitive)."`
	Role string `name:"role" help:"Only members with this role: child|guardian (dependent is accepted for child)." enum:",child,guardian,dependent" default:""`
}

// member is one family member's addressable identity, flattened from the nested account
// details so an agent can read the fields it passes to a verb directly off each row.
// Role is the parent's word (child|guardian), not the API's DEPENDENT|GUARDIAN, and
// IsChild is explicit so nothing has to be inferred (docs/CLI-DESIGN.md §9).
type member struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	IsChild   bool   `json:"is_child"`
	ServiceID int64  `json:"service_id"`
	ProfileID int64  `json:"profile_id"`
	DeviceID  int64  `json:"device_id,omitempty"`
	Pairing   string `json:"pairing,omitempty"`
	Plan      string `json:"plan,omitempty"`
}

// membersOrder is the sort `members` prints, stated in its help and footer so "the first
// child" is well-defined: guardians, then children with PAIRED before UNPAIRED, then by name.
const membersOrder = "guardians first, then children with PAIRED before UNPAIRED, then by name"

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
	// user's own service id (from the token) — that is why this command needs no target.
	sid := tokenstore.Claims(idt)["custom:identifier-serviceid"]
	if sid == "" {
		return fmt.Errorf("no service id in the stored token; run `safe_cli auth login`")
	}
	appUUID, _ := resolveAppUUID(ts)
	a, err := fetchAccount(context.Background(), authedRequest(rc, st, ts), rc.D, sid, appUUID)
	if err != nil {
		return err
	}
	members := filterMembers(a.Members, c.Find, c.Role)
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
			m.Name, m.Role, m.Pairing, fmt.Sprintf("%d", m.ServiceID),
			fmt.Sprintf("%d", m.ProfileID), dev,
		})
	}
	if err := outfmt.Table(rc.Out, []string{"NAME", "ROLE", "PAIRING", "SERVICE-ID", "PROFILE-ID", "DEVICE-ID"}, rows); err != nil {
		return err
	}
	_, err = fmt.Fprintf(rc.Out, "\nOrder: %s. Pass SERVICE-ID as --service-id to child-scoped ops. "+
		"Device-scoped ops (pause, contacts, websites) need a PAIRED device.\n", membersOrder)
	return err
}

// filterMembers applies --find (case-insensitive substring on the name) and --role
// (child|guardian; dependent means child). Both empty returns the input unchanged.
func filterMembers(ms []member, find, role string) []member {
	if find == "" && role == "" {
		return ms
	}
	role = strings.ToLower(role)
	if role == "dependent" {
		role = "child"
	}
	var out []member
	for _, m := range ms {
		if find != "" && !strings.Contains(strings.ToLower(m.Name), strings.ToLower(find)) {
			continue
		}
		if role != "" && m.Role != role {
			continue
		}
		out = append(out, m)
	}
	return out
}
