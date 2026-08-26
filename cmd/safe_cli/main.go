// Command safe_cli is a comprehensive, gog/GAM-style CLI for Verizon Family
// (Smith Micro SafePath). It is unofficial and intended for administering your
// own family account. The command surface and data model are generated from a
// protocol descriptor (internal/descriptor); network operations are added in
// later phases behind the same descriptor. See README.md and docs/FINDINGS.md.
package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/ejc3/safe_cli/internal/descriptor"
	"github.com/ejc3/safe_cli/internal/outfmt"
)

var version = "0.1.0-dev"

// Globals are flags shared by every subcommand.
type Globals struct {
	JSON bool `name:"json" help:"Machine-readable JSON output (stdout-as-API)."`
}

// runContext is bound into each command's Run method by kong.
type runContext struct {
	D   *descriptor.Descriptor
	G   *Globals
	Out io.Writer
}

// CLI is the top-level command grammar.
type CLI struct {
	Globals

	Version  versionCmd  `cmd:"" help:"Print the CLI version and the descriptor it targets."`
	Entities entitiesCmd `cmd:"" help:"List the entities in the SafePath data model."`
	Describe describeCmd `cmd:"" help:"Show the operations and actions for one entity."`
	Members  membersCmd  `cmd:"" help:"List family members with their service/profile/device ids (the targets for --service-id). Start here."`
	Auth     authCmd     `cmd:"" help:"Manage authentication (import/status/logout)."`
	Raw      rawCmd      `cmd:"" help:"Call any backend path with the stored id_token."`
	Call     callCmd     `cmd:"" help:"Invoke a descriptor operation/action on an entity (verb entity id)."`
}

type versionCmd struct{}

func (c *versionCmd) Run(rc *runContext) error {
	if rc.G.JSON {
		return outfmt.JSON(rc.Out, map[string]string{
			"version":     version,
			"descriptor":  rc.D.Name,
			"app_package": rc.D.AppPackage,
			"app_version": rc.D.AppVersion,
			"base_url":    rc.D.BaseURL,
		})
	}
	if _, err := fmt.Fprintf(rc.Out, "safe_cli %s\n", version); err != nil {
		return err
	}
	_, err := fmt.Fprintf(rc.Out, "descriptor: %s (targets %s %s at %s)\n",
		rc.D.Name, rc.D.AppPackage, rc.D.AppVersion, rc.D.BaseURL)
	return err
}

type entitiesCmd struct{}

func (c *entitiesCmd) Run(rc *runContext) error {
	if rc.G.JSON {
		return outfmt.JSON(rc.Out, rc.D.EntityNames())
	}
	var rows [][]string
	for _, name := range rc.D.EntityNames() {
		e, _ := rc.D.Entity(name)
		rows = append(rows, []string{name, fmt.Sprintf("%d", len(e.Operations)+len(e.Actions)), e.Summary})
	}
	return outfmt.Table(rc.Out, []string{"ENTITY", "OPS", "SUMMARY"}, rows)
}

type describeCmd struct {
	Entity string `arg:"" help:"Entity name (see 'safe_cli entities')."`
}

func (c *describeCmd) Run(rc *runContext) error {
	e, ok := rc.D.Entity(c.Entity)
	if !ok {
		return fmt.Errorf("unknown entity %q; run 'safe_cli entities'", c.Entity)
	}
	if rc.G.JSON {
		return outfmt.JSON(rc.Out, e)
	}
	if _, err := fmt.Fprintf(rc.Out, "%s — %s\n", c.Entity, e.Summary); err != nil {
		return err
	}
	if e.IDField != "" {
		if _, err := fmt.Fprintf(rc.Out, "  id: %s\n", e.IDField); err != nil {
			return err
		}
	}
	var rows [][]string
	row := func(name string, op descriptor.Operation) []string {
		if op.Destructive {
			name = "⚠ " + name // catastrophic: requires --confirm
		}
		return []string{name, op.Method, opFlags(op, e.IDField), op.Description}
	}
	for _, k := range e.OperationNames() {
		rows = append(rows, row(k, e.Operations[k]))
	}
	for _, k := range e.ActionNames() {
		rows = append(rows, row(k+" (action)", e.Actions[k]))
	}
	if err := outfmt.Table(rc.Out, []string{"OP", "METHOD", "FLAGS", "WHAT IT DOES"}, rows); err != nil {
		return err
	}
	_, err := fmt.Fprintln(rc.Out, "\nFLAGS: svc=--service-id (child)  body=--data  query=NAMES (--query name=value)  "+
		"header=NAMES (--header name=value)  path=NAMES (--path name=value)  "+
		"multipart=upload (not constructible)  confirm=destructive, needs --confirm. Full paths: --json.")
	return err
}

// placeholderRe matches {name} segments in a descriptor path.
var placeholderRe = regexp.MustCompile(`\{([^}]+)\}`)

// autoHeaders are the request headers the CLI fills itself (from --service-id and the other
// identity flags, or a generated id) — so they are never listed as names the agent must pass.
var autoHeaders = map[string]bool{
	"x-fp-identifier-target-serviceid": true, // --service-id
	"x-fp-identifier-profileid":        true, // --profile-id / id_token claim
	"x-fp-identifier-deviceid":         true, // --device-id / id_token claim
	"x-fp-identifier-app-uuid":         true, // resolved from the install
	"x-trace-transaction-id":           true, // generated per call
	"x-transaction-id":                 true, // generated per call
}

// opFlags summarizes, in a few space-separated tokens, exactly what a caller must supply to
// invoke op — so an agent can construct the call straight from `describe` without dropping to
// --json or the repo. Name-bearing args (query/header/path) list their exact declared names,
// e.g. `query=newPin` or `header=If-None-Match`; idField is the entity's own id placeholder,
// filled by the positional id argument rather than --path.
func opFlags(op descriptor.Operation, idField string) string {
	var f []string
	for _, h := range op.Headers {
		if strings.Contains(h, "serviceid") {
			f = append(f, "svc")
			break
		}
	}
	switch {
	case op.Multipart:
		f = append(f, "multipart")
	case op.TakesBody:
		f = append(f, "body")
	}
	if ph := extraPlaceholders(op.Path, idField); len(ph) > 0 {
		f = append(f, "path="+strings.Join(ph, ","))
	}
	if len(op.Query) > 0 {
		f = append(f, "query="+strings.Join(op.Query, ","))
	}
	if h := headerNames(op); len(h) > 0 {
		f = append(f, "header="+strings.Join(h, ","))
	}
	if op.Destructive {
		f = append(f, "confirm")
	}
	if len(f) == 0 {
		return "-"
	}
	return strings.Join(f, " ")
}

// extraPlaceholders returns the {name} path segments other than the entity's own id field —
// the ones an agent must fill with `--path name=value` (the id field is the positional id).
func extraPlaceholders(path, idField string) []string {
	var out []string
	for _, m := range placeholderRe.FindAllStringSubmatch(path, -1) {
		if m[1] != idField {
			out = append(out, m[1])
		}
	}
	return out
}

// headerNames returns the header NAMES an agent must pass via `--header name=value`: every
// header the op declares except the identity/trace headers the CLI fills itself (autoHeaders,
// and any serviceid header shown as svc) and the decompiler's dynamic-@HeaderMap artifacts,
// which are not real header names.
func headerNames(op descriptor.Operation) []string {
	var out []string
	for _, h := range op.Headers {
		if autoHeaders[h] || strings.Contains(h, "serviceid") {
			continue
		}
		if strings.HasPrefix(h, "(") || strings.Contains(h, "@HeaderMap") || strings.Contains(h, "dynamic") {
			continue // decompiler placeholder for an arbitrary header map, not a name
		}
		out = append(out, h)
	}
	return out
}

func main() {
	d, err := descriptor.Default()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "safe_cli:", err)
		os.Exit(1)
	}
	var cli CLI
	ctx := kong.Parse(&cli,
		kong.Name("safe_cli"),
		kong.Description("Unofficial CLI for Verizon Family (Smith Micro SafePath) — administer your own family account."),
		kong.UsageOnError(),
	)
	rc := &runContext{D: d, G: &cli.Globals, Out: os.Stdout}
	ctx.FatalIfErrorf(ctx.Run(rc))
}
