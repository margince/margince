// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The analytics query vocabulary, reachable by a caller that reads TOOLS and
// not resources.
//
// margince://schema/analytics is a resource, and a resource is reachable by an
// MCP client that reads resources and by nobody else. A tools-only client —
// the majority — reads run_analytics_query naming a document it has no call to
// open, and the Surface-B runner is offered no resource step at all. Both
// recover only by refusal, and what that loop costs was measured on the query
// vocabulary, whose own tool file records the incident. The shape transfers;
// it is not retold here.
//
// NOT a second copy: the handler serves the resource's own composition,
// derived per caller, so the tool and the resource cannot drift.

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// DescribeAnalyticsVocabularyResult carries the published analytics vocabulary.
//
// One member, holding the document VERBATIM. It is a string rather than a JSON
// mirror of the schema because the resource publishes prose — a Go struct here
// would be a hand-maintained restatement of what the engine derives per
// caller, which is the copy this whole move exists to remove.
type DescribeAnalyticsVocabularyResult struct {
	Vocabulary string `json:"vocabulary"`
}

// AnalyticsVocabularyReader answers the analytics vocabulary for the calling
// principal.
//
// A port rather than the schema itself, so the composition root can wrap the
// answer the way it wraps the analytics runner's: in an overlay workspace the
// query verb is refused, and a vocabulary served there would teach a caller a
// language nothing here can execute.
type AnalyticsVocabularyReader interface {
	AnalyticsVocabularyDocument(ctx context.Context) (string, error)
}

// RegisterAnalyticsVocabularyTool adds describe_analytics_vocabulary.
//
// Unconditional, like the verb it describes: run_analytics_query is registered
// on every build and refuses at CALL time under an overlay system of record,
// so there is no installation that serves the verb and lacks the vocabulary it
// refuses against.
func RegisterAnalyticsVocabularyTool(r *Registry, read AnalyticsVocabularyReader) {
	r.Register(describeAnalyticsVocabulary{read: read})
}

type describeAnalyticsVocabulary struct {
	read AnalyticsVocabularyReader
}

func (t describeAnalyticsVocabulary) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "describe_analytics_vocabulary", Title: "Describe the analytics vocabulary",
		Version:       toolVersionV1,
		Description:   describeAnalyticsVocabularyCopy.render(),
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		// No arguments. The document is composed for the calling principal, so
		// a population filter would only let a caller narrow what it already
		// receives, at the cost of a name it could spell wrong — which is the
		// failure this tool exists to end.
		InputSchema: schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		// The vocabulary's shape is the published document's own, and this
		// surface deliberately does not restate it.
		OutputSchema: schemaFor[DescribeAnalyticsVocabularyResult](),
	}
}

func (t describeAnalyticsVocabulary) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	// The arguments are decoded even though there are none to READ, because
	// decoding is what enforces the schema: without it a call carrying
	// `{"entity":"pipeline-current"}` is accepted silently and the caller then
	// reads the whole vocabulary believing it asked about one population. The
	// declared `additionalProperties: false` is a promise this keeps. An
	// ABSENT payload skips it, because for THIS tool that is the normal call.
	if err := decodeNoArguments(in); err != nil {
		return nil, err
	}
	// The resource's own composition, not a second rendering of the schema.
	body, err := t.read.AnalyticsVocabularyDocument(ctx)
	if err != nil {
		return nil, err
	}
	return json.Marshal(DescribeAnalyticsVocabularyResult{Vocabulary: body})
}
