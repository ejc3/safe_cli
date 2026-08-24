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
	Query     []string `name:"query" short:"q" help:"Query parameter as name=value (repeatable), for operations that declare query params (see 'safe_cli describe <entity>')."`
	Path      []string `name:"path" short:"p" help:"Path placeholder as name=value (repeatable), for paths with more than one {name} segment."`
	Header    []string `name:"header" short:"H" help:"Extra request header as name=value (repeatable), for headers a op declares that no flag covers (e.g. timezone, schedule-type, If-None-Match)."`
	Confirm   bool     `name:"confirm" help:"Required to run a catastrophic, effectively irreversible operation (deleting a user/device/subscription, wiping messages)."`
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
	query, err := parseQuery(c.Query)
	if err != nil {
		return err
	}
	pathParams, err := parseKV("path", c.Path)
	if err != nil {
		return err
	}
	userHeaders, err := parseKV("header", c.Header)
	if err != nil {
		return err
	}
	args := callArgs{
		entity:      c.Entity,
		op:          c.Op,
		id:          c.ID,
		data:        c.Data,
		idHeaders:   identityHeaders(idt, c.ServiceID, c.ProfileID, c.DeviceID, appUUID),
		query:       query,
		pathParams:  pathParams,
		userHeaders: userHeaders,
		confirm:     c.Confirm,
	}
	return runCall(context.Background(), authedRequest(rc, st, ts), rc.D, args, rc.Out, rc.G.JSON)
}

// callArgs bundles everything a single call needs beyond the descriptor: which op, the
// positional id and JSON body, and the maps that fill headers / query / path placeholders.
type callArgs struct {
	entity, op, id, data string
	idHeaders            map[string]string // x-fp-identifier-* values keyed by header name
	query                url.Values        // extra query params (ordered, multi-value)
	pathParams           map[string]string // {name} path placeholder values
	userHeaders          map[string]string // arbitrary extra request headers
	confirm              bool              // set by --confirm; required for destructive ops
}

// parseQuery turns repeated name=value flags into url.Values, preserving duplicate keys
// (some ops send the same query key more than once, e.g. productType=VSF&productType=GIZMO).
func parseQuery(pairs []string) (url.Values, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	q := url.Values{}
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("--query %q must be name=value", p)
		}
		q.Add(k, v)
	}
	return q, nil
}

// parseKV turns repeated name=value flags into a map (nil for none); flag names the flag
// for the error message.
func parseKV(flag string, pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	m := make(map[string]string, len(pairs))
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("--%s %q must be name=value", flag, p)
		}
		m[k] = v
	}
	return m, nil
}

// doFunc sends an HTTP request (with the given extra request-identity headers) and returns
// the response. Production wires it to an id_token-authenticated client that refreshes on
// 401 (authedRequest); tests inject a direct client.
type doFunc func(ctx context.Context, method, path string, body []byte, headers map[string]string) (*client.Response, error)

// runCall resolves the operation, fills the path and query, attaches the headers the
// descriptor declares for it (identity values, a generated trace id, or user-supplied
// ones), sends via do, and writes the response. Parental-control endpoints are plain
// id_token-authed (confirmed from the decompiled TokenAwareInterceptor — Authorization:
// <id_token>, no SigV4); each declares which x-fp-identifier-* headers it needs (the
// child's service id is the near-universal one).
func runCall(ctx context.Context, do doFunc, d *descriptor.Descriptor, a callArgs, out io.Writer, asJSON bool) error {
	o, err := resolveOp(d, a.entity, a.op)
	if err != nil {
		return err
	}
	if o.Destructive && !a.confirm {
		return fmt.Errorf("%s %s is catastrophic and effectively irreversible (%s) — re-run with --confirm to proceed", a.entity, a.op, o.Method)
	}
	if strings.Contains(o.Path, "@") {
		return fmt.Errorf("%s %s has a runtime-resolved (@Url) path the CLI cannot construct: %q", a.entity, a.op, o.Path)
	}
	ent, _ := d.Entity(a.entity)
	path, err := fillPath(o.Path, ent.IDField, a.id, a.pathParams)
	if err != nil {
		return err
	}
	path = appendQuery(path, a.query)
	body, err := buildBody(o.Body, a.data)
	if err != nil {
		return err
	}
	// Guide the caller when an op is known (from the decompiled interface) to require a body
	// but none was supplied — show the example payload when we have one, rather than sending
	// an empty request.
	if o.TakesBody && len(body) == 0 {
		if o.BodyExample != "" {
			return fmt.Errorf("%s %s needs a JSON body — pass --data. Example: %s", a.entity, a.op, o.BodyExample)
		}
		return fmt.Errorf("%s %s needs a JSON body — pass --data (payload shape not yet extracted for this op)", a.entity, a.op)
	}
	headers := make(map[string]string)
	var missingSvc []string
	for _, h := range o.Headers {
		switch {
		case a.idHeaders[h] != "":
			headers[h] = a.idHeaders[h]
		case a.userHeaders[h] != "":
			headers[h] = a.userHeaders[h]
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
		return fmt.Errorf("%s %s needs %s — pass --service-id with the TARGET (child's) service id (parental-control acts on a child, not the parent's own service)", a.entity, a.op, strings.Join(missingSvc, ", "))
	}
	// Any extra --header values not already placed (e.g. a header the op doesn't declare)
	// are still sent, so the caller can always reproduce the exact request.
	for k, v := range a.userHeaders {
		if _, ok := headers[k]; !ok {
			headers[k] = v
		}
	}
	resp, err := do(ctx, o.Method, path, body, headers)
	if err != nil {
		return err
	}
	return writeAPIResponse(out, asJSON, resp)
}

// appendQuery appends query parameters onto path, choosing ? or & as needed. url.Values
// keeps every value for a repeated key and encodes deterministically (keys sorted).
func appendQuery(path string, query url.Values) string {
	if len(query) == 0 {
		return path
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + query.Encode()
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
func fillPath(path, idField, id string, pathParams map[string]string) (string, error) {
	filled := path
	// The positional id fills the entity's own {idField}; --path name=value can supply it
	// too (and any other placeholder).
	idPlaceholder := idField != "" && strings.Contains(path, "{"+idField+"}")
	if idPlaceholder {
		v := id
		if v == "" {
			v = pathParams[idField]
		}
		if v == "" {
			return "", fmt.Errorf("this operation needs a %s (path %s)", idField, path)
		}
		filled = strings.ReplaceAll(filled, "{"+idField+"}", v)
	}
	for k, v := range pathParams {
		filled = strings.ReplaceAll(filled, "{"+k+"}", v)
	}
	if strings.Contains(filled, "{") {
		return "", fmt.Errorf("path %s has unfilled placeholder(s) — pass --path name=value for each {name}", filled)
	}
	// A fixed path (no {idField} placeholder) plus a positional id: the collection-endpoint
	// heuristic appends ?idField=id.
	if !idPlaceholder && id != "" {
		if idField == "" {
			return "", fmt.Errorf("operation takes no id, but %q was given", id)
		}
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		return path + sep + url.QueryEscape(idField) + "=" + url.QueryEscape(id), nil
	}
	return filled, nil
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
