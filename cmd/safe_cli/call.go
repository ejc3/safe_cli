package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"github.com/ejc3/safe_cli/internal/client"
	"github.com/ejc3/safe_cli/internal/descriptor"
	"github.com/ejc3/safe_cli/internal/tokenstore"
)

// callCmd invokes any entity operation or action from the descriptor against the
// backend using the stored id_token, e.g. `safe_cli call web_filter print <profileId>`.
// It is the generic engine behind the data model; the descriptor is the source of the
// method, path, and id placeholder.
type callCmd struct {
	Entity    string   `arg:"" help:"Entity (see 'safe_cli entities')."`
	Op        string   `arg:"" help:"Operation or action (see 'safe_cli describe <entity>')."`
	ID        string   `arg:"" optional:"" help:"Resource id to fill a {placeholder} in the path."`
	Data      string   `name:"data" help:"JSON request body (for create/update/action operations)."`
	ServiceID string   `name:"service-id" help:"Target (child's) service id for the x-fp-identifier-target-serviceid header."`
	ProfileID string   `name:"profile-id" help:"Target profile id (defaults to the id_token's own profile)."`
	DeviceID  string   `name:"device-id" help:"Target device id (defaults to the id_token's own device)."`
	Query     []string `name:"query" short:"q" help:"Query parameter as key=value (repeatable), for operations that declare query params (see 'safe_cli describe <entity>')."`
}

func (c *callCmd) Run(rc *runContext) error {
	st, ts, err := loadTokens()
	if err != nil {
		return err
	}
	idt, ok := ts.IDToken()
	if !ok {
		return fmt.Errorf("no id_token in the stored tokens; run `safe_cli auth login`")
	}
	// Best-effort: only the two services_hub ops need app-uuid, so a missing device id
	// must not block every other call — identityHeaders just omits the header.
	appUUID, _ := resolveAppUUID(ts)
	hdrs := identityHeaders(idt, c.ServiceID, c.ProfileID, c.DeviceID, appUUID)
	query, err := parseKV(c.Query)
	if err != nil {
		return err
	}
	return runCall(context.Background(), authedRequest(rc, st, ts), rc.D, c.Entity, c.Op, c.ID, c.Data, hdrs, query, rc.Out, rc.G.JSON)
}

// parseKV turns repeated key=value flags into a map (nil for none).
func parseKV(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	m := make(map[string]string, len(pairs))
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("--query %q must be key=value", p)
		}
		m[k] = v
	}
	return m, nil
}

// doFunc sends an HTTP request (with the given extra request-identity headers) and returns
// the response. Production wires it to an id_token-authenticated client that refreshes on
// 401 (authedRequest); tests inject a direct client.
type doFunc func(ctx context.Context, method, path string, body []byte, headers map[string]string) (*client.Response, error)

// runCall resolves the operation, fills the path, attaches the request-identity headers the
// descriptor declares for it (from idHeaders), sends via do, and writes the response.
// Parental-control endpoints are plain id_token-authed (confirmed from the decompiled
// TokenAwareInterceptor — Authorization: <id_token>, no SigV4); each declares which
// x-fp-identifier-* headers it needs (the child's service id is the near-universal one).
func runCall(ctx context.Context, do doFunc, d *descriptor.Descriptor, entity, op, id, data string, idHeaders, query map[string]string, out io.Writer, asJSON bool) error {
	o, err := resolveOp(d, entity, op)
	if err != nil {
		return err
	}
	if strings.Contains(o.Path, "@") {
		return fmt.Errorf("%s %s has a runtime-resolved (@Url) path the CLI cannot construct: %q", entity, op, o.Path)
	}
	ent, _ := d.Entity(entity)
	path, err := fillPath(o.Path, ent.IDField, id)
	if err != nil {
		return err
	}
	path = appendQuery(path, query)
	body, err := buildBody(o.Body, data)
	if err != nil {
		return err
	}
	headers := make(map[string]string)
	var missingSvc []string
	for _, h := range o.Headers {
		switch {
		case idHeaders[h] != "":
			headers[h] = idHeaders[h]
		case h == "x-trace-transaction-id":
			// The app supplies a fresh UUID here; newAppRequest only adds x-transaction-id.
			tid, terr := client.TraceID()
			if terr != nil {
				return terr
			}
			headers[h] = tid
		case strings.Contains(h, "serviceid"):
			missingSvc = append(missingSvc, h)
		}
	}
	if len(missingSvc) > 0 {
		return fmt.Errorf("%s %s needs %s — pass --service-id with the TARGET (child's) service id (parental-control acts on a child, not the parent's own service)", entity, op, strings.Join(missingSvc, ", "))
	}
	resp, err := do(ctx, o.Method, path, body, headers)
	if err != nil {
		return err
	}
	return writeAPIResponse(out, asJSON, resp)
}

// appendQuery appends query parameters (sorted, for deterministic URLs) onto path,
// choosing ? or & as needed.
func appendQuery(path string, query map[string]string) string {
	if len(query) == 0 {
		return path
	}
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	var b strings.Builder
	b.WriteString(path)
	for _, k := range keys {
		b.WriteString(sep)
		b.WriteString(url.QueryEscape(k))
		b.WriteByte('=')
		b.WriteString(url.QueryEscape(query[k]))
		sep = "&"
	}
	return b.String()
}

// identityHeaders builds the x-fp-identifier-* header VALUES keyed by the wire header
// name each op declares: the target service id from --service-id, the install's app-uuid,
// and profile/device from the flags or, when omitted, the id_token's own claims. runCall
// attaches only the ones each op lists, so only the four names below are ever emitted.
func identityHeaders(idToken, serviceID, profileID, deviceID, appUUID string) map[string]string {
	claims := tokenstore.Claims(idToken)
	pick := func(flag, claim string) string {
		if flag != "" {
			return flag
		}
		return claims[claim]
	}
	m := make(map[string]string)
	if serviceID != "" {
		m["x-fp-identifier-target-serviceid"] = serviceID
	}
	if v := pick(profileID, "custom:identifier-profileid"); v != "" {
		m["x-fp-identifier-profileid"] = v
	}
	if v := pick(deviceID, "custom:identifier-deviceid"); v != "" {
		m["x-fp-identifier-deviceid"] = v
	}
	if appUUID != "" {
		m["x-fp-identifier-app-uuid"] = appUUID
	}
	return m
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
