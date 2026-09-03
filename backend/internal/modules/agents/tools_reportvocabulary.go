// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The report plan vocabulary, reachable by a caller that cannot read an MCP
// resource.
//
// margince://schema/reports is a resource, and a resource is reachable by an MCP
// client that reads resources and by nobody else. Two of the three callers this
// surface actually has are not that:
//
//   - A client that lists TOOLS only — the majority — reads run_report naming a
//     document it has no call to open.
//   - A Surface-B run has no document seam AT ALL. The runner offers a step no
//     resource read, and giving it one was measured and made things worse, so the
//     gap is a decision rather than an oversight. That makes this tool the only
//     vocabulary route the runner has, which is the reason it exists.
//
// Without a door, both of those recover only by refusal — honest, and enough to
// get there in principle. What that costs was already measured on the QUERY
// vocabulary, whose own tool file records the incident: refusals that were each
// correct, and a caller that reasoned from them to a false statement about the
// product. The shape transfers; the incident is not this vocabulary's and is not
// retold here.
//
// NOT a second copy: the handler renders the resource's own document, over the
// same catalog, so the tool and the resource cannot drift.

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// DescribeReportVocabularyResult carries the published report vocabulary.
//
// One member, holding the document VERBATIM rather than a Go mirror of it. A
// struct here would be a hand-maintained restatement of what the engine's
// prebuilt catalog derives — the copy this whole move exists to remove, and the
// same reason describe_query_vocabulary answers with a raw document.
type DescribeReportVocabularyResult struct {
	Vocabulary json.RawMessage `json:"vocabulary"`
}

// ReportVocabularyReader answers the report plan vocabulary.
//
// A port rather than the catalog itself, so the composition root can wrap the
// answer the way it wraps run_report's: in an overlay workspace run_report is
// refused, and a vocabulary served there would teach a caller a name list
// nothing can execute. ReportVocabularyResource satisfies it, which is what
// keeps the tool and the resource one document.
type ReportVocabularyReader interface {
	ReportVocabularyDocument(ctx context.Context) (json.RawMessage, error)
}

// RegisterReportVocabularyTool adds describe_report_vocabulary.
//
// Unconditional, like the resource beside it and for the same reason:
// RegisterReportTool is unconditional too — run_report is registered on every
// build and refuses at CALL time under an overlay system of record — so there is
// no installation that serves the tool and lacks the vocabulary it refuses
// against. A conditionally-absent door would be a tool naming a document that a
// build could omit, which is the asymmetry the pointer gates fail.
func RegisterReportVocabularyTool(r *Registry, read ReportVocabularyReader) {
	r.Register(describeReportVocabulary{read: read})
}

type describeReportVocabulary struct {
	read ReportVocabularyReader
}

func (t describeReportVocabulary) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "describe_report_vocabulary", Title: "Describe the report vocabulary",
		Version:       toolVersionV1,
		Description:   describeReportVocabularyCopy.render(),
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		// No arguments. The document is the same for every caller, so a
		// `report` filter would only let one narrow what it already receives,
		// at the cost of a name it could spell wrong — which is the failure
		// this tool exists to end.
		InputSchema: schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		// The vocabulary's shape is the published document's own, and this
		// surface deliberately does not restate it.
		OutputSchema: schemaFor[DescribeReportVocabularyResult](),
	}
}

func (t describeReportVocabulary) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	// The arguments are decoded even though there are none to READ, because
	// decodeArgs is what enforces the schema: without it a call carrying
	// `{"report":"deals-by-stage"}` is accepted silently and the caller then
	// reads the whole catalog believing it asked about one report. The declared
	// `additionalProperties: false` is a promise this keeps.
	//
	// An ABSENT payload skips it, because for THIS tool that is the normal
	// call: it takes no arguments, so a client sending none is correct.
	if err := decodeNoArguments(in); err != nil {
		return nil, err
	}
	// The resource's own composition, not a second rendering of the catalog.
	body, err := t.read.ReportVocabularyDocument(ctx)
	if err != nil {
		return nil, err
	}
	return json.Marshal(DescribeReportVocabularyResult{Vocabulary: body})
}
