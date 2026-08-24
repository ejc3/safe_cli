// Package descriptor loads the machine-readable protocol descriptor that maps
// the entire SafePath (Verizon Family) data model — entities, operations, and
// the auth/attestation contract — onto the discovered REST backend.
//
// The descriptor is the single source of truth from which the CLI's command
// surface and data model are generated (the gog/GAM model). Endpoint paths were
// harvested by static analysis; each carries a `confirmed` flag that a dynamic
// capture flips to true. See docs/FINDINGS.md.
package descriptor

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
)

//go:embed verizon_family.json
var embedded []byte

// Operation is one HTTP call: a CRUD verb on an entity or a named action. Headers and
// Query list the request-identity headers (e.g. x-fp-identifier-target-serviceid) and
// query-parameter names the decompiled Retrofit interface declares for this call; the
// CLI fills them from flags/token claims. Placeholders are {name} path segments.
// Headers drives runCall today; Query and Placeholders are carried from the Retrofit
// interfaces but not yet consumed (fillPath handles only the single {id_field} segment,
// so multi-placeholder paths are not yet callable — a documented follow-up).
type Operation struct {
	Method       string         `json:"method"`
	Path         string         `json:"path"`
	Body         map[string]any `json:"body,omitempty"`
	Headers      []string       `json:"headers,omitempty"`
	Query        []string       `json:"query,omitempty"`
	Placeholders []string       `json:"placeholders,omitempty"`
	Confirmed    bool           `json:"confirmed"`
	// Destructive marks a catastrophic, effectively irreversible op (deleting a user,
	// device, or subscription; wiping messages). `call` refuses these without --confirm.
	Destructive bool `json:"destructive,omitempty"`
}

// Entity is one type in the data model, with its CRUD operations and actions. Tier marks
// peripheral/secondary surfaces ("p2") — e.g. Driving Insights, which is the one feature
// backed by its own AWS/S3 credential path rather than the shared id_token API.
type Entity struct {
	Summary    string               `json:"summary"`
	IDField    string               `json:"id_field"`
	Tier       string               `json:"tier,omitempty"`
	Operations map[string]Operation `json:"operations"`
	Actions    map[string]Operation `json:"actions,omitempty"`
}

// Attestation records the answer to the project's "deciding question".
type Attestation struct {
	Required   bool   `json:"required"`
	Confidence string `json:"confidence"`
	Note       string `json:"note"`
}

// Auth describes how the client authenticates to the backend.
type Auth struct {
	Style       string            `json:"style"`
	ClientID    string            `json:"client_id"`
	RedirectURI string            `json:"redirect_uri"`
	Scope       string            `json:"scope"`
	Header      string            `json:"header"`
	Scheme      string            `json:"scheme"`
	Endpoints   map[string]string `json:"endpoints"`
	Attestation Attestation       `json:"attestation"`
}

// Descriptor is the whole protocol contract.
type Descriptor struct {
	Name       string            `json:"name"`
	AppPackage string            `json:"app_package"`
	AppVersion string            `json:"app_version"`
	BaseURL    string            `json:"base_url"`
	Auth       Auth              `json:"auth"`
	Entities   map[string]Entity `json:"entities"`
}

// Default returns the descriptor embedded in the binary.
func Default() (*Descriptor, error) { return Parse(embedded) }

// Parse validates and decodes a descriptor from JSON.
func Parse(b []byte) (*Descriptor, error) {
	var d Descriptor
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("parse descriptor: %w", err)
	}
	if d.BaseURL == "" {
		return nil, fmt.Errorf("descriptor %q has an empty base_url", d.Name)
	}
	if len(d.Entities) == 0 {
		return nil, fmt.Errorf("descriptor %q declares no entities", d.Name)
	}
	return &d, nil
}

// EntityNames returns the entity keys in stable, sorted order.
func (d *Descriptor) EntityNames() []string {
	names := make([]string, 0, len(d.Entities))
	for k := range d.Entities {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// Entity looks up one entity by name.
func (d *Descriptor) Entity(name string) (Entity, bool) {
	e, ok := d.Entities[name]
	return e, ok
}

// OperationNames returns an entity's operation keys, sorted.
func (e Entity) OperationNames() []string {
	names := make([]string, 0, len(e.Operations))
	for k := range e.Operations {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// ActionNames returns an entity's action keys, sorted.
func (e Entity) ActionNames() []string {
	names := make([]string, 0, len(e.Actions))
	for k := range e.Actions {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
