// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// InputChecksReader answers the open findings this caller may see, for one
// population when the arguments name one.
type InputChecksReader func(ctx context.Context, args InputChecksArgs) (json.RawMessage, error)

// InputChecksArgs narrows list_input_checks the way its route's query does.
type InputChecksArgs struct {
	ScopeKind string `json:"scope_kind,omitempty"`
	ScopeID   string `json:"scope_id,omitempty"`
}

// RegisterInputChecksTool joins list_input_checks to the surface.
func RegisterInputChecksTool(r *Registry, read InputChecksReader) {
	r.Register(listInputChecks{read: read})
}

type listInputChecks struct {
	read InputChecksReader
}

func (t listInputChecks) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "list_input_checks", Title: "What the forecast's inputs still need",
		Version:       toolVersionV1,
		Description:   listInputChecksCopy.render(),
		Instead:       listInputChecksCopy.Instead,
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute, Unlisted: true,
		OpenAPIOp: "listInputChecks",
		InputSchema: schema(`{"type":"object","properties":{
			"scope_kind":{"type":"string","enum":["workspace","team","owner"],"description":"Only the findings on the deals of this population. Omit for every finding you can see."},
			"scope_id":{"type":"string","format":"uuid","description":"The team or owner, for a team or owner scope_kind"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[InputChecksResult](),
	}
}

func (t listInputChecks) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args InputChecksArgs
	if len(bytes.TrimSpace(in)) > 0 {
		if err := decodeArgs(in, &args); err != nil {
			return nil, err
		}
	}
	noteDerivedContent(ctx)
	return t.read(ctx, args)
}

// InputChecksResult is what list_input_checks answers with.
type InputChecksResult struct {
	// Data are the open findings this caller can see, most material first.
	Data []InputCheckResult `json:"data"`
	// Groups are the same findings by type, counted over the whole list.
	Groups []InputCheckGroupResult `json:"groups"`
	// Totals count every finding, and each deal once.
	Totals InputCheckTotalsResult `json:"totals"`
}

// InputCheckDestination is where a finding is reviewed, and whether this
// caller may answer it there ("answer") or only read it ("read").
type InputCheckDestination struct {
	Screen string `json:"screen"`
	Mode   string `json:"mode"`
}

// InputCheckGroupResult is the findings of one type. Deals are counted apart
// from findings, because one deal can carry several.
type InputCheckGroupResult struct {
	Type          string `json:"type"`
	Findings      int    `json:"findings"`
	Deals         int    `json:"deals"`
	OtherFindings int    `json:"other_findings"`
	// Exposure is one amount per currency, never summed across currencies;
	// a deal counts once, at its largest amount in the group.
	Exposure    []InputCheckExposureResult `json:"exposure"`
	Destination InputCheckDestination      `json:"destination"`
}

// InputCheckExposureResult is a group's money in one currency.
type InputCheckExposureResult struct {
	Currency    string `json:"currency"`
	AmountMinor int64  `json:"amount_minor"`
}

// InputCheckTotalsResult counts a whole list.
type InputCheckTotalsResult struct {
	Findings      int `json:"findings"`
	Deals         int `json:"deals"`
	OtherFindings int `json:"other_findings"`
}

// InputCheckResult is one finding.
type InputCheckResult struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	SubjectKind string `json:"subject_kind"`
	SubjectID   string `json:"subject_id"`
	Severity    string `json:"severity"`
	// AffectedMinor is the money in question. Absent means it cannot be said,
	// which is not the same as nothing being at stake.
	AffectedMinor *int64 `json:"affected_minor,omitempty"`
	Currency      string `json:"currency,omitempty"`
	// Claim and Observed hold structured values whose keys depend on Type.
	//
	// Raw rather than a map: the key set is the exception TYPE's, not this
	// struct's. A schema generator has nothing to describe a free-form
	// object with. Passing the stored bytes through also keeps this from
	// becoming a second place that knows what each type stores.
	Claim       json.RawMessage `json:"claim"`
	Observed    json.RawMessage `json:"observed"`
	FirstSeenAt string          `json:"first_seen_at"`
	// LastSeenAt is the most recent run that still found it. Something seen for
	// weeks is a different problem from something that appeared last night.
	LastSeenAt string `json:"last_seen_at"`
	// InputsChanged is true when the subject, or a stage or forecast call,
	// changed after the run that found this. Quote such a finding as out of
	// date. Null when that is not known.
	InputsChanged *bool                  `json:"inputs_changed"`
	Destination   *InputCheckDestination `json:"destination,omitempty"`
}
