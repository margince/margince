// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The query vocabulary, reachable by a client that reads TOOLS and not
// resources.
//
// `query_workspace` refuses every name outside the vocabulary — by name, never
// approximated — and its description sends the caller to
// margince://schema/query to learn what the names are. That instruction assumes
// the caller can read an MCP resource, and a large share of clients surface
// tools only. For those, the loop had no exit: a tool naming a document they
// cannot open, then refusing each guess.
//
// It is not hypothetical. On 2026-08-26 a client asked which companies were
// near Cologne, probed four spellings of the geo field, was correctly refused
// four times, and told its user that organizations in this workspace carry no
// address at all — while `address` with `within_radius` sat in the vocabulary
// it had no door to. The refusals were honest and the conclusion was wrong,
// which is the worst pairing available: a caller reasoning carefully from what
// it could see, to a false statement about the product.
//
// So the same document gets a second door. NOT a second copy: the handler
// renders the resource's own composition, over the resolver the validator
// reads, so the tool and the resource cannot drift. A hand-written schema in
// the tool's description was the alternative and is the thing the original
// design correctly refused — it would be a maintained list of exactly what is
// derived, stale the first time a workspace declared a custom field.

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// DescribeQueryVocabularyResult carries the published query vocabulary.
//
// One member, holding the document VERBATIM rather than a Go mirror of it. The
// vocabulary is composed per caller from the field catalog and the live column
// catalog, so a struct here would be a hand-maintained restatement of exactly
// what is derived — the copy this surface refuses to keep, and the same reason
// the grammar was never inlined into query_workspace's input schema.
//
// It lives beside its tool rather than in results.go, where the other result
// types sit: that file is at the 500-line cap, and this type is legible only
// next to the door it comes out of.
type DescribeQueryVocabularyResult struct {
	Vocabulary json.RawMessage `json:"vocabulary"`
}

// VocabularyReader answers the query vocabulary for the calling principal.
//
// The port is declared here and satisfied by search's schema resource, like
// every other cross-module edge on this surface (ADR-0054).
type VocabularyReader interface {
	VocabularyDocument(ctx context.Context) (json.RawMessage, error)
}

// RegisterVocabularyTool adds describe_query_vocabulary, when a reader is
// wired.
//
// A composition that publishes no query vocabulary registers no tool to read
// one, the same way RegisterQueryTool declines a nil runner: a surface that
// advertises a door to nothing is worse than one door fewer.
func RegisterVocabularyTool(r *Registry, read VocabularyReader) {
	if read == nil {
		return
	}
	r.Register(describeQueryVocabulary{read: read})
}

type describeQueryVocabulary struct {
	read VocabularyReader
}

func (t describeQueryVocabulary) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "describe_query_vocabulary", Title: "Describe the query vocabulary",
		Version:       toolVersionV1,
		Description:   describeQueryVocabularyCopy.render(),
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		// No arguments. The document is composed for the calling principal, so
		// there is nothing to ask about: a target filter would only let a
		// caller narrow what they already receive, at the cost of a name they
		// could spell wrong — which is the failure this tool exists to end.
		InputSchema: schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		// The vocabulary's shape is the published document's own, and this
		// surface deliberately does not restate it: that restatement is the
		// hand-maintained copy the whole design avoids.
		OutputSchema: schemaFor[DescribeQueryVocabularyResult](),
	}
}

func (t describeQueryVocabulary) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	// The arguments are decoded even though there are none to READ, because
	// decodeArgs is what enforces the schema: without it a call carrying
	// `{"target":"organisation"}` is accepted silently, and the caller then
	// reads a whole-workspace vocabulary believing they asked about one record
	// type. The declared `additionalProperties: false` is a promise this keeps.
	//
	// An ABSENT payload skips it, because for THIS tool that is the normal
	// call: it takes no arguments, so a client sending none is correct.
	// decodeArgs would answer "the payload is empty; send a JSON object
	// carrying this operation's fields" — advice for a tool that has fields,
	// and a refusal of the one call this tool is designed for.
	if err := decodeNoArguments(in); err != nil {
		return nil, err
	}
	body, err := t.read.VocabularyDocument(ctx)
	if err != nil {
		return nil, err
	}
	return json.Marshal(DescribeQueryVocabularyResult{Vocabulary: body})
}
