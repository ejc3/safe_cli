// Package outfmt renders CLI output in the two shapes a gog-style tool needs:
// human tables and machine-readable JSON (stdout-as-API).
package outfmt

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// JSON writes v as indented JSON.
func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// Table writes a simple left-aligned, space-padded table. rows whose length
// differs from headers are padded/truncated so a ragged row can never panic.
func Table(w io.Writer, headers []string, rows [][]string) error {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	norm := func(r []string) []string {
		out := make([]string, len(headers))
		for i := range headers {
			if i < len(r) {
				out[i] = r[i]
			}
		}
		return out
	}
	normalized := make([][]string, len(rows))
	for ri, r := range rows {
		nr := norm(r)
		normalized[ri] = nr
		for i, c := range nr {
			if len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}
	writeRow := func(cells []string) error {
		parts := make([]string, len(cells))
		for i, c := range cells {
			parts[i] = c + strings.Repeat(" ", widths[i]-len(c))
		}
		_, err := fmt.Fprintln(w, strings.TrimRight(strings.Join(parts, "  "), " "))
		return err
	}
	if err := writeRow(headers); err != nil {
		return err
	}
	for _, r := range normalized {
		if err := writeRow(r); err != nil {
			return err
		}
	}
	return nil
}
