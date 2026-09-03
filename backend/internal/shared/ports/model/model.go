// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package model defines the provider-agnostic LLM client seam
// (interfaces.md §4, 03b Layer 3). Model choice is config, not
// architecture: one implementation per provider (Anthropic / OpenAI /
// local vLLM/Ollama), never on the synchronous hot path, with a
// secret-stripper hook on every outbound payload and a local-only
// capability for the sovereign zero-egress profile (P7).
package model

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
)

// ErrEmbeddingsUnsupported reports a chat provider with no embedding
// lane (embeddings are a separate lane, ai-operational-spec §1.1); the
// routing layer binds Embed to a dedicated embedder instead. Port-level
// so consumers in other modules can errors.Is against it without
// importing a provider package.
var ErrEmbeddingsUnsupported = errors.New("model: provider has no embedding lane")

// ErrAttachmentUnsupported reports an adapter that cannot carry a given
// attachment MIME on its wire (a model capability limit, parallel to
// ErrEmbeddingsUnsupported — NOT an apperrors domain sentinel). Callers route
// or surface honestly rather than silently dropping the attachment.
var ErrAttachmentUnsupported = errors.New("model: provider cannot carry this attachment type")

// Attachment is one cross-provider input part. Bytes XOR URI: Bytes for inline
// content, URI for a provider file handle / URL. Name is optional provenance.
type Attachment struct {
	MIME  string
	Bytes []byte
	URI   string
	Name  string
}

// CarriesMIME reports whether mime falls inside a declared carriage set, in the
// spelling Capabilities.AttachmentMIMEs uses: an exact media type
// ("application/pdf"), or a type wildcard ("image/*"). It is the ONE matcher —
// an adapter derives its wire gate from the same set it declares, so what a
// binding says it carries and what it will actually accept cannot drift apart.
func CarriesMIME(declared []string, mime string) bool {
	for _, pattern := range declared {
		if prefix, wildcard := strings.CutSuffix(pattern, "*"); wildcard {
			if strings.HasPrefix(mime, prefix) {
				return true
			}
			continue
		}
		if pattern == mime {
			return true
		}
	}
	return false
}

// IntersectMIMEs is the other half of CarriesMIME: the carriage set two
// declarations both admit, computed over the patterns rather than over their
// spellings.
//
// The spellings are the whole point. Two declarations can describe overlapping
// sets and share no literal — a wire that decodes {image/jpeg, image/png} and
// an operator who wrote `image/*` agree completely, and a literal comparison
// reads that agreement as a contradiction and carries nothing. That is what a
// binding-side permission written as a wildcard has to compose with, so the
// intersection has to understand what CarriesMIME understands.
//
// Never wider than either input: every pattern returned is the narrower of one
// pattern from each side, so anything it admits both sides already admitted.
// That is the safety property the literal comparison used to buy by being
// blunt, and it is now bought by construction instead.
//
// Order follows a, then b within each element of a, so the answer is stable for
// a caller that compares sets by equality.
func IntersectMIMEs(a, b []string) []string {
	kept := make([]string, 0, len(a))
	for _, left := range a {
		for _, right := range b {
			narrow, overlap := narrower(left, right)
			if !overlap || slices.Contains(kept, narrow) {
				continue
			}
			kept = append(kept, narrow)
		}
	}
	// A pattern already covered by a wider one that survived describes nothing
	// the set does not already admit, and two spellings of one set would make
	// equality comparisons depend on which order the inputs arrived in.
	return dropCovered(kept)
}

// narrower reports the pattern admitting exactly what both a and b admit, and
// whether they overlap at all. The four cases are the two pattern kinds
// CarriesMIME reads, squared.
func narrower(a, b string) (string, bool) {
	aPrefix, aWild := strings.CutSuffix(a, "*")
	bPrefix, bWild := strings.CutSuffix(b, "*")
	switch {
	case !aWild && !bWild:
		return a, a == b
	case aWild && !bWild:
		// The exact type is the narrower one, when the wildcard admits it.
		return b, strings.HasPrefix(b, aPrefix)
	case !aWild && bWild:
		return a, strings.HasPrefix(a, bPrefix)
	}
	// Both wildcards: one contains the other, or they are disjoint. "image/*"
	// and "image/x-*" overlap in the second; "image/*" and "audio/*" in
	// neither direction, and share nothing.
	if strings.HasPrefix(bPrefix, aPrefix) {
		return b, true
	}
	if strings.HasPrefix(aPrefix, bPrefix) {
		return a, true
	}
	return "", false
}

// dropCovered removes every pattern another kept pattern already admits.
func dropCovered(patterns []string) []string {
	kept := make([]string, 0, len(patterns))
	for i, pattern := range patterns {
		covered := false
		for j, other := range patterns {
			if i == j || other == pattern {
				continue
			}
			if CarriesMIME([]string{other}, pattern) {
				covered = true
				break
			}
		}
		if !covered {
			kept = append(kept, pattern)
		}
	}
	return kept
}

// Client is the swappable model interface; selection is config.
type Client interface {
	// Complete is a single-shot completion (summaries, draft replies,
	// NL→query-plan compilation).
	Complete(ctx context.Context, req Request) (Response, error)

	// Stream yields tokens incrementally (first-token budget 1.5s).
	Stream(ctx context.Context, req Request) (TokenStream, error)

	// Embed produces vectors for pgvector retrieval / the context graph.
	Embed(ctx context.Context, req EmbedRequest) (Embeddings, error)

	// Caps reports what the provider supports so callers route correctly
	// (cheap/local for capture+classify, premium when quality demands).
	Caps() Capabilities
}

type Request struct {
	Model     string // logical model id; config resolves it to a provider model
	System    string
	Messages  []Message
	Tools     []ToolDef
	MaxTokens int
	// ContextScopes and ContextFingerprint bind a compose-selected company
	// context view to this request. They are routing/cache/trace metadata, not
	// provider wire fields: the compose provider renders the actual values as a
	// delimited user-data message, while the AI router uses these fields to make
	// stale-context cache hits impossible and the call trace inspectable.
	ContextScopes      []string
	ContextFingerprint string
	// ContextBytes and ContextTokensEstimate describe only the final delimited
	// company-context block. They are trace metadata, never provider inputs.
	ContextBytes          int
	ContextTokensEstimate int
	// IncludeCompanyContext opts into a task policy explicitly marked
	// conditional. Compose consumes and clears the flag before routing; it
	// cannot select scopes or bypass a policy-none declaration.
	IncludeCompanyContext bool
	// ResponseSchema, when non-nil, is a JSON Schema the completion must
	// conform to. Providers with schema-constrained decoding enforce it at
	// GENERATION so a weak model cannot emit the wrong shape — Ollama via
	// `format`, vLLM via the OpenAI `response_format` json_schema, and
	// Anthropic via `output_config.format`. Providers without a native mode
	// (the offline fake) ignore it and the caller's parse→validate→retry
	// policy still catches malformed output. It is a shape guardrail, never a
	// substitute for that policy or the domain evidence gate. It is a
	// json.RawMessage (not []byte) so it is carried as a JSON value, never
	// base64-encoded, if a wire embeds it.
	ResponseSchema json.RawMessage
	// SecretStripper runs over the OUTBOUND payload before it leaves the
	// process. Hygiene only — credentials and secrets, not PII
	// pseudonymization (A8 revised); privacy is the location ladder. In
	// the sovereign profile egress is blocked entirely regardless.
	SecretStripper SecretStripper
	// ProviderOptions carries vendor-only knobs, namespaced by provider key
	// (e.g. {"openai":{"reasoning_effort":"low"}}). An adapter reads only its
	// own namespace and ignores the rest; an unknown namespace is a no-op. This
	// is how a native adapter gets reasoning/thinking/cache-control without
	// widening this interface per vendor.
	ProviderOptions map[string]json.RawMessage
	// Attachments are typed cross-provider input parts (image/pdf/audio). Each
	// capable adapter maps them to its wire; one that cannot carry a given MIME
	// returns ErrAttachmentUnsupported (never a silent drop).
	Attachments []Attachment
}

type Message struct {
	Role    string // "user" | "assistant"
	Content string
}

// ToolDef is a native tool-use declaration passed through to providers
// that support it.
type ToolDef struct {
	Name        string
	Description string
	InputSchema []byte // JSON Schema
}

type Response struct {
	Text string
	// InputTokens is the TOTAL prompt tokens billed, cache reads AND cache
	// writes INCLUDED — every adapter must normalize to this. OpenAI and
	// Gemini already report an inclusive total on the wire; Anthropic reports
	// input_tokens EXCLUSIVE of both cache buckets, so its adapter adds
	// CachedTokens and CacheWriteTokens back in. This keeps InputTokens a
	// true prompt-cost figure on every provider (a pricer values it as one
	// number, never re-deriving it from the itemized buckets below).
	InputTokens int
	// OutputTokens is the TOTAL billed output, reasoning/thinking tokens
	// INCLUDED — every adapter must normalize to this (Gemini reports them
	// separately; its adapter adds them back), so tokens_in+tokens_out is
	// true spend on every provider and the budget bands can't be leaked past
	// by thinking-heavy calls.
	OutputTokens int
	// CachedTokens / ReasoningTokens are the itemized usage a native provider
	// returns (prompt-cache reads, reasoning/thinking tokens). ReasoningTokens
	// is a breakdown WITHIN OutputTokens, never additive to it; an adapter
	// with no such figure leaves them 0.
	CachedTokens    int
	ReasoningTokens int
	// CacheWriteTokens is cache-creation (write) tokens, disjoint from
	// CachedTokens (which is the cache-READ subset) — both are already
	// counted inside InputTokens above, so this is a breakdown, never
	// additive on its own. 0 when the provider reports none.
	CacheWriteTokens int
	// ProviderMetadata carries vendor-only outputs namespaced by provider key
	// (e.g. {"openai":{"response_id":"…"}} for session logging).
	ProviderMetadata map[string]json.RawMessage
	// ServedModel is the provider-reported identity of the model that actually
	// answered — read off the wire response, never fabricated by an adapter. It
	// is empty when the provider reports none (the routing layer then falls
	// back to the configured tier binding, which may differ from what actually
	// served if the vendor silently substitutes a model).
	ServedModel string
}

// TokenStream delivers incremental completion tokens; Close releases the
// underlying connection.
type TokenStream interface {
	// Next returns the next chunk; ok is false when the stream is done.
	Next(ctx context.Context) (chunk string, ok bool, err error)
	Close() error
}

type EmbedRequest struct {
	Model  string
	Inputs []string
	// Dimensions, when > 0, asks the embedder to emit vectors of exactly this
	// width — the retrieval store's fixed column width. Cloud embedders whose
	// native width differs (OpenAI text-embedding-3, Gemini gemini-embedding-001)
	// honor it via their truncation parameter; an embedder already at the width
	// (a local bge-m3, the fake) ignores it. 0 means "provider default".
	Dimensions int
}

type Embeddings struct {
	Vectors [][]float32
	Dims    int
}

// SecretStripper removes credentials/secrets (API keys, tokens,
// passwords) from a model-bound payload. Conformance-tested: secrets
// never appear in an outbound payload.
type SecretStripper interface {
	Strip(ctx context.Context, payload []byte) (stripped []byte, report StripReport, err error)
}

// StripReport says what was removed, for the audit trail.
type StripReport struct {
	Findings int
	Kinds    []string
}

type Capabilities struct {
	Streaming bool
	EmbedDims int
	// LocalOnly is true for local inference — the P7 sovereignty and
	// zero-egress path.
	LocalOnly bool
	// AttachmentMIMEs is the closed set of media types this client carries on
	// its wire, in CarriesMIME's spelling ("image/*", "application/pdf"). Empty
	// means the wire carries no attachment parts at all, which is a legitimate
	// binding rather than a broken one.
	//
	// It is DECLARED rather than discovered because refusing at send time
	// answers the wrong question: a caller holding a document learns "this
	// binding is text-only" only by attempting the call, and that attempt is
	// indistinguishable, in the operator's own call trace, from a model that
	// failed. A caller that can read this picks its input lane first and
	// leaves no failed attempt behind for a configuration that is merely
	// text-only.
	AttachmentMIMEs []string
}

// Lister is implemented by an adapter whose vendor publishes what it
// currently serves.
//
// Deliberately NOT part of Client. A vendor list endpoint is a different
// question from inference, not every adapter has one to answer, and widening
// Client would make five adapters carry a method two of them can only refuse.
// A caller type-asserts, exactly as it does for any other optional capability,
// and treats a client that does not implement it as "this vendor does not
// publish a list" rather than as a failure.
//
// It answers AVAILABILITY, never price. A broker (OpenRouter) publishes
// per-model prices on the same wire endpoint this reads; the native vendors
// put theirs on an HTML page. Either way this interface drops it: the price
// sheet is its own effective-dated record and stays the authority on cost for
// anything reached through a stored binding, so a second price arriving by
// this route would be two answers that drift the first time either moves. A
// model listed here that the sheet cannot price is bindable and reports
// UNPRICED — that is honest.
//
// `ai`'s unauthenticated, unbound OpenRouter read (asked by provider name,
// never through a Lister) folds a vendor's own price in anyway: it has no
// stored binding to protect and no sheet row to contradict yet, so its price
// rides beside the sheet's, always labelled PROPOSED and never confused with
// a recorded rate.
type Lister interface {
	// ListModels reports what the vendor serves, newest first where the vendor
	// dates its models and in the vendor's own order where it does not.
	ListModels(ctx context.Context) ([]Info, error)
}

// Info is one model a vendor says it serves.
//
// Three fields and no fourth. The vendors disagree about everything else they
// publish — context windows, modalities, deprecation dates, owners — and a
// field only some of them fill is one the caller cannot rely on. What every
// list endpoint agrees on is an id, and what a picker needs is that id plus
// enough to sort and label it.
//
// A caller that needs more than this asks a wider question than Lister
// answers: `ai.AvailableModel` embeds Info and adds the price and rank-score
// fields the one unauthenticated broker read (OpenRouter, by provider name)
// can honestly state. That type lives beside its own caller rather than
// widening this one, because every OTHER vendor's Lister still cannot fill
// those fields — and a Lister that returned them anyway would have nothing
// but a zero value to put there, which reads as an answer rather than as
// silence.
type Info struct {
	// ID is the string a binding names, exactly as the vendor spells it.
	ID string
	// DisplayName is the vendor's own human label, empty where it publishes
	// none. A caller shows the ID when this is empty rather than inventing a
	// prettier form of the id — the id is what an operator greps for.
	DisplayName string
	// Lane is what the vendor says the model is FOR, empty where it does not
	// say. Only Gemini states it (supportedGenerationMethods); the rest publish
	// one undifferentiated list. Empty means unknown, NOT chat: a caller that
	// needs the distinction filters on a stated lane rather than assuming one,
	// because binding an embedder to a chat tier produces a call that cannot
	// succeed.
	Lane string
}

// The lanes an Info can state. Spelled here rather than imported from the
// ai module because the port cannot depend on a module, and duplicated
// deliberately: these are the wire's own two words, and the ai module's Lane
// type is what maps them onto its price sheet.
const (
	LaneChat       = "chat"
	LaneEmbeddings = "embeddings"
)
