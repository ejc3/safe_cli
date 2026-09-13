package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// renderTemplate builds a request body from a descriptor `cli.body_template`
// (docs/CLI-DESIGN.md §4). The template is strict JSON in which a string leaf "$name"
// is a variable and "$name?" an optional one. Every "$name" is replaced by vars[name],
// keeping its type (a number stays a number, a bool a bool); a "$name?" whose variable is
// absent or nil removes the property (or array element) entirely — that is how
// `pause-internet pause --indefinite` sends untilIUnpause=true with NO pauseSchedule,
// matching the wire-verified indefinite body. A required "$name" with no value is an
// error naming it, so a missing resolution can never silently ship an empty field.
// Literal leaves pass through untouched (they were declared constants at Parse).
func renderTemplate(tpl string, vars map[string]any) ([]byte, error) {
	return renderTemplateRepeat(tpl, vars, nil)
}

// renderTemplateRepeat is renderTemplate with repeatable-flag expansion: repeat names the
// variables whose value is a list, and a SINGLE-element array whose element references one
// of them is rendered once per value, with that value bound in place of the list and the
// element's other variables shared — `"domains":[{"status":"$status","url":"$url"}]` with
// `--url a.com --url b.com` becomes two domain objects (docs/CLI-DESIGN.md §4).
func renderTemplateRepeat(tpl string, vars map[string]any, repeat map[string]bool) ([]byte, error) {
	var root any
	if err := json.Unmarshal([]byte(tpl), &root); err != nil {
		return nil, fmt.Errorf("body_template is not valid JSON: %w", err)
	}
	out, _, err := renderNode(root, vars, repeat)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

// renderNode returns the rendered node and whether it should be kept (false = an optional
// variable with no value, so the caller drops the property/element).
func renderNode(n any, vars map[string]any, repeat map[string]bool) (any, bool, error) {
	switch t := n.(type) {
	case map[string]any:
		m := make(map[string]any, len(t))
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys) // deterministic error order
		for _, k := range keys {
			v, keep, err := renderNode(t[k], vars, repeat)
			if err != nil {
				return nil, false, err
			}
			if keep {
				m[k] = v
			}
		}
		return m, true, nil
	case []any:
		if len(t) == 1 {
			if name := repeatVarIn(t[0], repeat); name != "" {
				return expandRepeat(t[0], name, vars, repeat)
			}
		}
		arr := make([]any, 0, len(t))
		for _, el := range t {
			v, keep, err := renderNode(el, vars, repeat)
			if err != nil {
				return nil, false, err
			}
			if keep {
				arr = append(arr, v)
			}
		}
		return arr, true, nil
	case string:
		if !strings.HasPrefix(t, "$") {
			return t, true, nil
		}
		optional := strings.HasSuffix(t, "?")
		name := strings.TrimSuffix(strings.TrimPrefix(t, "$"), "?")
		v, ok := vars[name]
		if !ok || v == nil {
			if optional {
				return nil, false, nil
			}
			return nil, false, fmt.Errorf("body variable $%s has no value", name)
		}
		return v, true, nil
	default:
		return n, true, nil
	}
}

// repeatVarIn returns the name of the first repeatable variable referenced anywhere in
// the node, or "" — the signal that a single-element array is an expansion template.
func repeatVarIn(n any, repeat map[string]bool) string {
	switch t := n.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if name := repeatVarIn(t[k], repeat); name != "" {
				return name
			}
		}
	case []any:
		// A nested array is its own expansion site: the NEAREST enclosing singleton array
		// expands on the variable, so an outer array is left alone ({"profiles":[{"domains":
		// [{"url":"$url"}]}]} yields one profile with several domains).
		return ""
	case string:
		if strings.HasPrefix(t, "$") {
			name := strings.TrimSuffix(strings.TrimPrefix(t, "$"), "?")
			if repeat[name] {
				return name
			}
		}
	}
	return ""
}

// expandRepeat renders the element template once per value of the repeatable variable
// name, binding each value as a scalar in a copy of vars. A missing or empty list renders
// an empty array (the flag was required or nulled; omission is the caller's decision).
func expandRepeat(elem any, name string, vars map[string]any, repeat map[string]bool) (any, bool, error) {
	items, err := asList(vars[name])
	if err != nil {
		return nil, false, fmt.Errorf("repeatable variable $%s: %w", name, err)
	}
	arr := make([]any, 0, len(items))
	for _, item := range items {
		scoped := make(map[string]any, len(vars))
		for k, v := range vars {
			scoped[k] = v
		}
		scoped[name] = item
		// The bound value is a scalar now; do not re-expand on it.
		inner := make(map[string]bool, len(repeat))
		for k, v := range repeat {
			if k != name {
				inner[k] = v
			}
		}
		v, keep, err := renderNode(elem, scoped, inner)
		if err != nil {
			return nil, false, err
		}
		if keep {
			arr = append(arr, v)
		}
	}
	return arr, true, nil
}

func asList(v any) ([]any, error) {
	switch t := v.(type) {
	case nil:
		return nil, nil
	case []any:
		return t, nil
	case []string:
		out := make([]any, len(t))
		for i, s := range t {
			out[i] = s
		}
		return out, nil
	default:
		return []any{t}, nil // a single value expands to one element
	}
}

// renderQuery builds query parameters from a `cli.query` map: a literal value is sent
// as-is, a "$flag" value is the flag's value (omitted when the flag has no value). Flag
// values mapped with maps_to "query:<name>" are merged in by the caller the same way.
func renderQuery(q map[string]string, vars map[string]any) (url.Values, error) {
	if len(q) == 0 {
		return nil, nil
	}
	out := url.Values{}
	for name, spec := range q {
		if !strings.HasPrefix(spec, "$") {
			out.Set(name, spec)
			continue
		}
		v, ok := vars[strings.TrimPrefix(spec, "$")]
		if !ok || v == nil {
			continue
		}
		s, err := queryString(v)
		if err != nil {
			return nil, fmt.Errorf("query %s: %w", name, err)
		}
		out.Set(name, s)
	}
	return out, nil
}

// queryString renders a flag value for a query parameter: scalars as their text, a list
// joined with commas (the API's convention for multi-valued params like betaProviders).
func queryString(v any) (string, error) {
	v = normalizeNum(v) // an int default arrives as a JSON float64; render 1000000, not 1e+06
	switch t := v.(type) {
	case string:
		return t, nil
	case bool:
		return fmt.Sprintf("%t", t), nil
	case int, int64, float64:
		return fmt.Sprintf("%v", t), nil
	case []string:
		return strings.Join(t, ","), nil
	case []any:
		parts := make([]string, 0, len(t))
		for _, el := range t {
			s, err := queryString(el)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		}
		return strings.Join(parts, ","), nil
	default:
		return "", fmt.Errorf("unsupported query value type %T", v)
	}
}

// normalizeNum prints a JSON float that is a whole number without a decimal part so
// 10003 (float64 from encoding/json) compares equal to the flag text "10003" and an int
// default renders as 1000000, not 1e+06.
func normalizeNum(v any) any {
	if f, ok := v.(float64); ok && f == float64(int64(f)) {
		return int64(f)
	}
	return v
}
