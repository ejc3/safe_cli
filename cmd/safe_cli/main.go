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
		body := "-"
		if op.BodyExample != "" {
			body = "yes"
		}
		return []string{name, op.Method, op.Path, joinOr(op.Query), joinOr(op.Headers), body, confirmed(op.Confirmed)}
	}
	for _, k := range e.OperationNames() {
		rows = append(rows, row(k, e.Operations[k]))
	}
	for _, k := range e.ActionNames() {
		rows = append(rows, row(k+" (action)", e.Actions[k]))
	}
	return outfmt.Table(rc.Out, []string{"OP", "METHOD", "PATH", "QUERY", "HEADERS", "BODY", "CONFIRMED"}, rows)
}

// joinOr renders a descriptor string list for the table, or "-" when empty.
func joinOr(xs []string) string {
	if len(xs) == 0 {
		return "-"
	}
	return strings.Join(xs, ",")
}

func confirmed(b bool) string {
	if b {
		return "yes"
	}
	return "no (static)"
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
