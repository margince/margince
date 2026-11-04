// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// toolChecksJSON is what each verb's admin call was asked by the gates on its
// path, measured by TestTheToolRoleMatrixIsPublished and drift-gated there.
//
//go:embed toolchecks.json
var toolChecksJSON []byte

// ToolChecksFile is the generated table's shape.
type ToolChecksFile struct {
	Note  string                   `json:"note"`
	Verbs map[string]ToolChecksRow `json:"verbs"`
}

// ToolChecksRow is one verb's measured checks. Complete is false when the
// admin call stopped for a reason that is not a role. Only the checks before
// that stop are then known, and each one is still needed on that path.
type ToolChecksRow struct {
	Checks   []string `json:"checks"`
	Complete bool     `json:"complete"`
}

var requiredChecks = mustReadToolChecks(toolChecksJSON)

// EmbeddedToolChecks is the table this binary embeds, for the census that
// holds it to the served surface.
func EmbeddedToolChecks() (ToolChecksFile, error) {
	var file ToolChecksFile
	err := json.Unmarshal(toolChecksJSON, &file)
	return file, err
}

func mustReadToolChecks(raw []byte) map[string][]auth.Check {
	var file ToolChecksFile
	if err := json.Unmarshal(raw, &file); err != nil {
		//craft:ignore panic-in-domain the table is embedded at build time, and TestTheToolChecksTableReads reads it in every unit run
		panic(fmt.Sprintf("crmagents: toolchecks.json does not read: %v", err))
	}
	out := make(map[string][]auth.Check, len(file.Verbs))
	for verb, row := range file.Verbs {
		for _, name := range row.Checks {
			check, err := auth.ParseCheck(name)
			if err != nil {
				//craft:ignore panic-in-domain the table is embedded at build time, and TestTheToolChecksTableReads reads it in every unit run
				panic(fmt.Sprintf("crmagents: toolchecks.json, %s: %v", verb, err))
			}
			out[verb] = append(out[verb], check)
		}
	}
	return out
}

// grantedTo is spec as far as an agent's grants reach. A tool whose recorded
// checks the caller lacks is not offered, and a router offers only the actions
// whose verbs the caller holds. A verb with no recorded row stays offered: the
// gate still decides each call. Humans and the system are not narrowed, since
// their authority is enforced at the store and never listed.
func (r *Registry) grantedTo(ctx context.Context, spec mcp.ToolSpec) (mcp.ToolSpec, bool) {
	p, ok := principal.Actor(ctx)
	if !ok || p.Type != principal.PrincipalAgent {
		return spec, true
	}
	if len(spec.Actions) == 0 {
		return spec, r.holdsChecks(p, spec.Name)
	}
	kept := slices.DeleteFunc(slices.Clone(spec.Actions), func(a mcp.ToolAction) bool { return !r.holdsChecks(p, a.Verb) })
	if len(kept) == len(spec.Actions) {
		return spec, true
	}
	if len(kept) == 0 {
		return spec, false
	}
	spec.Actions = kept
	if spec.ActionField == "" {
		return spec, true
	}
	schema, err := narrowedChoices(spec.InputSchema, spec.ActionField, kept)
	if err != nil {
		// Offering the whole enum would advertise actions the caller cannot run.
		slog.Warn("router schema could not be narrowed, so it is not offered", "tool", spec.Name, "err", err)
		return spec, false
	}
	spec.InputSchema = schema
	return spec, true
}

func (r *Registry) holdsChecks(p principal.Principal, verb string) bool {
	for _, check := range r.required[verb] {
		if !check.HeldBy(p) {
			return false
		}
	}
	return true
}

// narrowedChoices is a router's schema with its route field admitting only
// the kept actions.
func narrowedChoices(schema json.RawMessage, field string, kept []mcp.ToolAction) (json.RawMessage, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(schema, &doc); err != nil {
		return nil, err
	}
	var properties map[string]map[string]json.RawMessage
	if err := json.Unmarshal(doc["properties"], &properties); err != nil {
		return nil, err
	}
	choice, ok := properties[field]
	if !ok {
		return nil, fmt.Errorf("the schema has no %s property", field)
	}
	words := make([]string, len(kept))
	for i, a := range kept {
		words[i] = a.Action
	}
	slices.Sort(words)
	var err error
	if choice[schemaEnum], err = json.Marshal(words); err != nil {
		return nil, err
	}
	if doc["properties"], err = json.Marshal(properties); err != nil {
		return nil, err
	}
	return json.Marshal(doc)
}
