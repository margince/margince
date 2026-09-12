// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// describe_record_fields — the write vocabulary as a tool, beside the
// margince://schema/record-fields resource.
//
// Both doors for the reason describe_report_vocabulary exists beside
// margince://schema/reports: a resource is reachable by a client that reads
// resources and by nobody else, and most callers on this surface list TOOLS
// only. Without a door they reach the field names by refusal alone — correct,
// and measured: a tools-only run writing a company, a contact and a
// relationship from one business card spent three turns being told the names it
// had guessed, one per record type, before it wrote anything.
//
// NOT a second copy: the handler renders the resource's own document, over the
// same contract shapes, so the tool and the resource cannot drift.

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// DescribeRecordFieldsResult carries the published write vocabulary.
//
// One member, holding the document VERBATIM rather than a Go mirror of it. A
// struct here would be a hand-maintained restatement of what the contract
// shapes derive — the copy this document exists to remove.
type DescribeRecordFieldsResult struct {
	Fields json.RawMessage `json:"fields"`
}

// RegisterRecordFieldsTool adds the write-vocabulary reader to the surface.
func RegisterRecordFieldsTool(r *Registry, read RecordFieldsReader) {
	r.Register(describeRecordFields{read: read})
}

type describeRecordFields struct {
	read RecordFieldsReader
}

func (t describeRecordFields) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "describe_record_fields", Title: "Describe the record write vocabulary",
		Version:     toolVersionV1,
		Description: describeRecordFieldsCopy.render(),
		// ScopeRead, where the RESOURCE beside it is ScopeWrite, and the
		// asymmetry is forced rather than chosen. A tool's read-only-ness is
		// DERIVED from this field: a write scope here would have the surface
		// splice an `idempotency_key` into the schema and route the call
		// through the replay machinery, offering retry safety for a tool that
		// changes nothing and records nothing to replay.
		//
		// It costs no disclosure. The document is composed from the contract
		// shapes alone — identical in every workspace, naming no row and no
		// workspace's own columns — so a read-only passport served it learns
		// what crm.yaml already says in public. The resource's write scope is
		// about not ADVERTISING the write vocabulary in a catalogue a read-only
		// client browses; this door is one such a client must ask for by name.
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		// No arguments. The document is composed from the contract and is the
		// same for every caller, so a record_type filter would only let one
		// narrow what it already receives, at the cost of a name it could spell
		// wrong — which is the failure this tool exists to end.
		InputSchema: schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		// The vocabulary's shape is the published document's own, and this
		// surface deliberately does not restate it.
		OutputSchema: schemaFor[DescribeRecordFieldsResult](),
	}
}

func (t describeRecordFields) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	// Decoded even with nothing to read, because decodeNoArguments is what
	// enforces the declared additionalProperties: false — without it a call
	// carrying `{"record_type":"contact"}` is accepted silently and the caller
	// then reads every record type believing it asked about one. An ABSENT
	// payload skips it, because for THIS tool that is the normal call.
	if err := decodeNoArguments(in); err != nil {
		return nil, err
	}
	// The resource's own composition, not a second rendering of the shapes.
	body, err := t.read.RecordFieldsDocument(ctx)
	if err != nil {
		return nil, err
	}
	return json.Marshal(DescribeRecordFieldsResult{Fields: body})
}
