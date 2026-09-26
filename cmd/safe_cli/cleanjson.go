package main

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// cleanHumanJSON renders a backend JSON body for a person reading the terminal: it drops
// pure-noise fields (the ~2KB presigned S3 `*PresignedUrl` values the generated read verbs
// dump), and encodes with HTML escaping OFF so `&` shows as `&` rather than `&`. It is
// used only on the human (non --json) path; `--json` and `call` keep the raw body so an
// agent still gets every field. On any parse failure it falls back to the raw body.
func cleanHumanJSON(out io.Writer, body []byte) error {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		_, err := out.Write(ensureNewline(body))
		return err
	}
	v = stripNoise(v)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		_, err := out.Write(ensureNewline(body))
		return err
	}
	_, err := out.Write(ensureNewline(bytes.TrimRight(buf.Bytes(), "\n")))
	return err
}

// stripNoise walks a decoded JSON value and removes fields that are pure UI plumbing a
// parent never needs: any key ending in "PresignedUrl" (the presigned S3 icon URLs), and
// any string value that is plainly a presigned URL (an AWS SigV4 query signature).
func stripNoise(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if strings.HasSuffix(strings.ToLower(k), "presignedurl") {
				delete(t, k)
				continue
			}
			t[k] = stripNoise(child)
		}
		return t
	case []any:
		for i, child := range t {
			t[i] = stripNoise(child)
		}
		return t
	case string:
		if isPresignedURL(t) {
			return "<presigned-url omitted>"
		}
		return t
	default:
		return v
	}
}

func isPresignedURL(s string) bool {
	return len(s) > 200 && (strings.Contains(s, "X-Amz-Signature") || strings.Contains(s, "X-Amz-Credential"))
}
