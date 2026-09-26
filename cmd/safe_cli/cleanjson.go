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

// emptyRecordBody recognizes a "no matching records" 404 whose body is a genuine data
// payload rather than a pure error — this backend returns e.g.
// {"statusCode":404,"totalCalls":0,"callActivity":null,…,"errors":[…]} for a quiet range.
// If the body is a JSON object carrying real data keys (anything beyond the error envelope),
// it returns the body with the error-envelope keys removed and true; otherwise nil,false so
// the caller reports the 404 as an error as usual.
func emptyRecordBody(body []byte) ([]byte, bool) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, false
	}
	envelope := map[string]bool{"errors": true, "statuscode": true, "status": true, "message": true, "details": true, "resourceidentifier": true, "code": true}
	dataKeys := 0
	for k := range m {
		if !envelope[strings.ToLower(k)] {
			dataKeys++
		}
	}
	if dataKeys == 0 {
		return nil, false // a pure error envelope: a real not-found
	}
	for k := range m {
		if envelope[strings.ToLower(k)] {
			delete(m, k)
		}
	}
	clean, err := json.Marshal(m)
	if err != nil {
		return nil, false
	}
	return clean, true
}

// isDataEmpty reports whether a JSON body carries no actual data — every leaf is null, 0,
// false, "", or an empty array/object. Used to turn a "no records" payload (call/text
// activity of all zeros/nulls) into a one-line "no results" instead of a blob of nulls.
func isDataEmpty(body []byte) bool {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return false
	}
	return valueEmpty(v)
}

func valueEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case bool:
		return !t
	case float64:
		return t == 0
	case string:
		return t == ""
	case []any:
		for _, x := range t {
			if !valueEmpty(x) {
				return false
			}
		}
		return true
	case map[string]any:
		for _, x := range t {
			if !valueEmpty(x) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
