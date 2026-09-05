// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// AnalyticsQueryRunner executes one typed analytics question. The module owns
// no SQL and no metric arithmetic: the query arrives as the wire shape, the
// engine validates it against the caller's derived schema, and what comes back
// is the contract-shaped answer.
type AnalyticsQueryRunner func(ctx context.Context, query json.RawMessage) (json.RawMessage, error)

// RegisterAnalyticsQueryTool adds run_analytics_query. The runner is always
// wired — the typed engine ships in every composition — so unlike the
// conditional registrations there is no absent-capability branch here.
func RegisterAnalyticsQueryTool(r *Registry, run AnalyticsQueryRunner) {
	r.Register(runAnalyticsQuery{run: run})
}

type runAnalyticsQuery struct {
	run AnalyticsQueryRunner
}

func (t runAnalyticsQuery) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "run_analytics_query", Title: "Run an analytics query", Version: toolVersionV1,
		Description:   runAnalyticsQueryCopy.render(),
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "runAnalyticsQuery",
		// The population/field vocabulary is NOT re-declared here — it is
		// derived per caller and published at margince://schema/analytics; a
		// second copy in this schema would be wrong for somebody.
		InputSchema: schema(`{"type":"object","required":["entity","measures"],"properties":{
			"entity":{"type":"string","description":"A population from margince://schema/analytics. An unknown name is refused with the allowed set."},
			"group_by":{"type":"array","items":{"type":"string"}},
			"measures":{"type":"array","items":{"type":"object","required":["fn"],"properties":{
				"fn":{"enum":["count","count_distinct","sum","avg","min","max","median","p75"]},
				"field":{"type":"string","description":"A measure name. Omit only with fn=count."},
				"as":{"type":"string"}},"additionalProperties":false}},
			"filters":{"type":"array","items":{"type":"object","required":["field","op"],"properties":{
				"field":{"type":"string"},
				"op":{"enum":["eq","ne","lt","lte","gt","gte","is_null","is_not_null"]},
				"value":{}},"additionalProperties":false}},
			"limit":{"type":"integer"},
			"save":{"type":"boolean","description":"Persist the run; the answer then carries a citable run_id."},
			"scope_kind":{"enum":["workspace","team","owner"],"description":"Omit for this seat's own default population."},
			"scope_id":{"type":"string"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[AnalyticsQueryResult](),
	}
}

// AnalyticsQueryResult is the tool's answer shape.
type AnalyticsQueryResult struct {
	Columns []string `json:"columns"`
	// Rows are aggregate rows whose members ARE the columns above; their
	// shape is the plan's, which is why nothing is declared about them here.
	Rows []json.RawMessage `json:"rows"`
	// Withheld says the privacy floor kept something back. A boolean and
	// never a count, like every other withheld flag on this surface.
	Withheld bool `json:"withheld"`
	// TotalSafe says whether a total over these rows may be shown; false once
	// anything is withheld, because total-minus-shown is the subtraction the
	// floor exists to stop.
	TotalSafe     bool   `json:"total_safe"`
	SchemaVersion string `json:"schema_version"`
	// RunID names the saved run, present exactly when save was set.
	RunID *string `json:"run_id,omitempty"`
}

func (t runAnalyticsQuery) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	noteDerivedContent(ctx)
	// Forwarded verbatim: the engine validates the vocabulary, and a second
	// check here would be a second answer to what is askable.
	return t.run(ctx, in)
}
