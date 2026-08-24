package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/ejc3/safe_cli/internal/client"
	"github.com/ejc3/safe_cli/internal/descriptor"
)

// callCmd invokes any entity operation or action from the descriptor against the
// backend using the stored id_token, e.g. `safe_cli call web_filter print <profileId>`.
// It is the generic engine behind the data model; the descriptor is the source of the
// method, path, and id placeholder.
type callCmd struct {
	Entity string `arg:"" help:"Entity (see 'safe_cli entities')."`
	Op     string `arg:"" help:"Operation or action (see 'safe_cli describe <entity>')."`
	ID     string `arg:"" optional:"" help:"Resource id to fill the path placeholder."`
	Data   string `name:"data" help:"JSON request body (for create/update/action operations)."`
}

func (c *callCmd) Run(rc *runContext) error {
	_, ts, err := loadTokens()
	if err != nil {
		return err
	}
	idToken, ok := ts.IDToken()
	if !ok {
		return fmt.Errorf("no id_token in the stored tokens; run `safe_cli auth login`")
	}
	cl := client.New(idToken)
	if rc.D.BaseURL != "" {
		cl.BaseURL = rc.D.BaseURL
	}
	return runCall(context.Background(), cl, rc.D, c.Entity, c.Op, c.ID, c.Data, rc.Out, rc.G.JSON)
}

// runCall resolves the operation, fills the path, sends the request, and writes the
// response. It is the testable core (the caller supplies an authenticated client).
func runCall(ctx context.Context, cl *client.Client, d *descriptor.Descriptor, entity, op, id, data string, out io.Writer, asJSON bool) error {
	o, err := resolveOp(d, entity, op)
	if err != nil {
		return err
	}
	// Parental-control operations require AWS SigV4 (Cognito temp creds), not the
	// id_token (docs/PROCESS.md §10). Reject them explicitly until that client exists,
	// rather than sending an id_token that the backend answers with 403.
	if strings.Contains(o.Path, "/parental-control/") {
		return fmt.Errorf("%s %s targets a parental-control endpoint, which needs AWS SigV4 (Cognito) — not yet implemented; see docs/PROCESS.md §10", entity, op)
	}
	ent, _ := d.Entity(entity)
	path, err := fillPath(o.Path, ent.IDField, id)
	if err != nil {
		return err
	}
	body, err := buildBody(o.Body, data)
	if err != nil {
		return err
	}
	resp, err := cl.Do(ctx, o.Method, path, body)
	if err != nil {
		return err
	}
	return writeAPIResponse(out, asJSON, resp)
}

// buildBody starts from the operation's descriptor default body (e.g. {"paused":true}
// for `device pause`) and merges any user --data over it (user keys win). With no
// defaults, --data is used raw; with neither, the body is nil.
func buildBody(defaults map[string]any, data string) ([]byte, error) {
	if len(defaults) == 0 {
		if data == "" {
			return nil, nil
		}
		if !json.Valid([]byte(data)) {
			return nil, fmt.Errorf("--data is not valid JSON")
		}
		return []byte(data), nil
	}
	merged := make(map[string]any, len(defaults))
	for k, v := range defaults {
		merged[k] = v
	}
	if data != "" {
		var user map[string]any
		if err := json.Unmarshal([]byte(data), &user); err != nil {
			return nil, fmt.Errorf("--data must be a JSON object to merge with the operation's default body: %w", err)
		}
		for k, v := range user {
			merged[k] = v
		}
	}
	return json.Marshal(merged)
}

// resolveOp finds an operation or action named op on entity.
func resolveOp(d *descriptor.Descriptor, entity, op string) (descriptor.Operation, error) {
	ent, ok := d.Entity(entity)
	if !ok {
		return descriptor.Operation{}, fmt.Errorf("unknown entity %q; run `safe_cli entities`", entity)
	}
	if o, ok := ent.Operations[op]; ok {
		return o, nil
	}
	if o, ok := ent.Actions[op]; ok {
		return o, nil
	}
	return descriptor.Operation{}, fmt.Errorf("entity %q has no operation or action %q; run `safe_cli describe %s`", entity, op, entity)
}

// fillPath places id into path. If the path has the {idField} placeholder it is
// substituted; otherwise (these collection endpoints mostly have fixed paths) a given
// id is appended as a query param `?idField=id` — a heuristic the live capture confirms
// (the app may instead pass it as a header for some endpoints).
func fillPath(path, idField, id string) (string, error) {
	ph := "{" + idField + "}"
	switch {
	case idField != "" && strings.Contains(path, ph):
		if id == "" {
			return "", fmt.Errorf("this operation needs a %s (path %s)", idField, path)
		}
		filled := strings.ReplaceAll(path, ph, id)
		if strings.Contains(filled, "{") {
			return "", fmt.Errorf("path has other unfilled placeholders: %s", filled)
		}
		return filled, nil
	case strings.Contains(path, "{"):
		return "", fmt.Errorf("path has unfilled placeholders: %s", path)
	case id != "":
		if idField == "" {
			return "", fmt.Errorf("operation takes no id, but %q was given", id)
		}
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		return path + sep + url.QueryEscape(idField) + "=" + url.QueryEscape(id), nil
	default:
		return path, nil
	}
}

// writeAPIResponse turns a non-2xx into an error; otherwise writes the JSON body (raw
// under --json, indented otherwise).
func writeAPIResponse(out io.Writer, asJSON bool, resp *client.Response) error {
	if resp.Status >= 400 {
		return fmt.Errorf("HTTP %d: %s", resp.Status, strings.TrimSpace(string(resp.Body)))
	}
	if asJSON {
		_, err := out.Write(ensureNewline(resp.Body))
		return err
	}
	var pretty bytes.Buffer
	if json.Indent(&pretty, resp.Body, "", "  ") == nil {
		_, err := out.Write(ensureNewline(pretty.Bytes()))
		return err
	}
	_, err := out.Write(ensureNewline(resp.Body))
	return err
}

func ensureNewline(b []byte) []byte {
	if len(b) == 0 || b[len(b)-1] == '\n' {
		return b
	}
	return append(b, '\n')
}
