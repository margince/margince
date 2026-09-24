// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// openAICompatClient is the shared OpenAI-wire transport (/v1/chat/completions,
// /v1/embeddings, response_format json_schema, SSE). Reused by the local vLLM
// binding (apiKey empty, localOnly true) and the cloud openai_compatible binding
// (Bearer key, localOnly false). The trust posture is the caller's choice of
// provider name, never a field on this struct (spec §3.2/§3.6).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

type openAICompatClient struct {
	http         *http.Client
	baseURL      string
	apiKey       string // "" ⇒ send no Authorization header (local vLLM)
	localOnly    bool   // Caps().LocalOnly — the sovereign-eligibility bit
	defaultModel string
	// attachmentMIMEs is what THIS binding carries, translated from the routing
	// config's `input:` (inputmodality.go). Every other adapter answers that from
	// a constant because its wire decides; here the served model decides, and
	// only the operator can see which model that is. Nil ⇒ text-only, the default
	// for an undeclared binding.
	//
	// One field, two uses — Caps() advertises it and sendChat enforces it — so a
	// binding cannot advertise a media type its own wire then refuses.
	attachmentMIMEs []string
	// routing carries the broker's upstream-selection preferences, nil when the
	// binding names none (and always nil on the local vLLM wire, which fronts
	// one host). Held on the client rather than read per request because it is
	// a property of the BINDING an operator configured, not of any one call.
	routing *OpenRouterRouting
}

// openAICompatSchemaName labels the structured-output schema; OpenAI's
// response_format requires a name, and the value is otherwise opaque.
const openAICompatSchemaName = "structured_output"

type openAICompatChatWire struct {
	Model string `json:"model"`
	// Messages is this adapter's own message type rather than the wireMessage
	// Ollama shares: only here can a turn carry attachment parts, and widening
	// the shared type would change Ollama's wire to buy nothing.
	Messages  []openAICompatMessage `json:"messages"`
	Tools     []ollamaToolWire      `json:"tools,omitempty"`
	MaxTokens int                   `json:"max_tokens,omitempty"`
	Stream    bool                  `json:"stream"`
	// ResponseFormat carries the OpenAI-compatible json_schema structured
	// output (vLLM guided decoding); set only when the request asks for a
	// schema, so ordinary free-text calls are unchanged.
	ResponseFormat *openAICompatResponseFormat `json:"response_format,omitempty"`
	// Provider and Reasoning are broker controls, omitted entirely when the
	// binding names no preference — an absent object leaves the broker's own
	// behaviour untouched, while an object of zero values would silently turn
	// off its load balancing.
	Provider  *openAICompatProviderWire  `json:"provider,omitempty"`
	Reasoning *openAICompatReasoningWire `json:"reasoning,omitempty"`
}

// openAICompatResponseFormat / openAICompatJSONSchema mirror the OpenAI
// response_format json_schema shape the endpoint accepts to constrain decoding
// to a schema.
type openAICompatResponseFormat struct {
	Type       string                 `json:"type"` // "json_schema"
	JSONSchema openAICompatJSONSchema `json:"json_schema"`
}

type openAICompatJSONSchema struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
	Strict bool            `json:"strict"`
}

type openAICompatChatResponse struct {
	// Model is the wire's echoed model field: this generic surface merely
	// reflects back the requested model id rather than confirming what
	// actually generated the completion (unlike the native adapters' own
	// served-identity fields) — the router's servedSource map tags it "echo"
	// accordingly, never "response".
	Model string `json:"model"`
	// Provider is the UPSTREAM that served, which a broker on this wire names
	// independently of the echoed Model above. A gateway fronting many
	// inference hosts picks one per request — the same model id can be served
	// by hosts differing in quantization, output ceiling and tail latency — so
	// without this field a call cannot be attributed to what actually ran, and
	// a slow or degraded upstream is indistinguishable from a slow model.
	//
	// Absent on a single-vendor OpenAI-wire host (a vLLM deployment), which
	// leaves it empty: there, the host we called is the host that served.
	Provider string               `json:"provider"`
	Choices  []openAICompatChoice `json:"choices"`
	Usage    struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		// CompletionTokensDetails itemizes reasoning spend inside
		// CompletionTokens (never additive to it — the port's contract for
		// Response.ReasoningTokens). Left 0 by a host that reports no breakdown.
		CompletionTokensDetails struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
		// PromptTokensDetails itemizes PromptTokens the same way: the cache
		// read and the cache write, both inside the total. A broker reports
		// them for every upstream that caches; a bare vLLM host omits them.
		PromptTokensDetails struct {
			CachedTokens     int `json:"cached_tokens"`
			CacheWriteTokens int `json:"cache_write_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}

func (c *openAICompatClient) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	resp, err := c.completeChat(ctx, req)
	return reportSchemaDowngrade(resp, err, strictDowngrade(req.ResponseSchema))
}

func (c *openAICompatClient) completeChat(ctx context.Context, req model.Request) (model.Response, error) {
	body, err := c.sendChat(ctx, req, false)
	if err != nil {
		return model.Response{}, err
	}
	//craft:ignore swallowed-errors best-effort close of a response body already read to completion — the decode result decides the outcome
	defer func() { _ = body.Close() }()
	var out openAICompatChatResponse
	if err := json.NewDecoder(body).Decode(&out); err != nil {
		return model.Response{}, fmt.Errorf("ai: openai-compat: decode response: %w", err)
	}
	if len(out.Choices) == 0 {
		return model.Response{}, fmt.Errorf("ai: openai-compat: response has no choices")
	}
	resp := model.Response{
		InputTokens:     out.Usage.PromptTokens,
		OutputTokens:    out.Usage.CompletionTokens,
		ReasoningTokens: reasoningWithin(out.Usage.CompletionTokens, out.Usage.CompletionTokensDetails.ReasoningTokens),
		ServedModel:     out.Model,
		ServedProvider:  out.Provider,
	}
	resp.CachedTokens, resp.CacheWriteTokens = cacheWithin(out.Usage.PromptTokens,
		out.Usage.PromptTokensDetails.CachedTokens, out.Usage.PromptTokensDetails.CacheWriteTokens)
	choice := out.Choices[0]
	finish, err := choice.terminal(ctx)
	if err != nil {
		return model.Response{}, withSpend(err, resp)
	}
	resp.Text = completionText(choice.Message.Content, choice.Message.Reasoning, finish)
	resp.FinishReason = finish
	return resp, nil
}

// reasoningWithin bounds a reported reasoning count by the completion it is a
// breakdown of, because on this wire the two counts do not always agree.
//
// The port defines ReasoningTokens as a subset of OutputTokens, never additive
// to it, and a broker's upstreams honour that unevenly. Measured against
// gpt-oss-120b on one structured-output request: DeepInfra reported 817
// reasoning tokens against 787 completion, Parasail 1117 against 1069, AkashML
// 1234 against 1121 — while total_tokens stayed prompt+completion on all three,
// so the excess is the upstream's two counters disagreeing rather than a
// different accounting basis. Capping the same request's output at 40 tokens
// showed the real relationship: completion came back at exactly the cap with
// reasoning just under it and no content at all, which is a subset.
//
// Left unbounded the excess does not merely misreport, it disables a check.
// aicert's checkCaps grades a run's answer as TokensOut minus ReasoningTokens,
// so a reasoning count above the completion makes that difference negative and
// the max_tokens cap can never be exceeded — a ceiling that silently always
// passes, which is worse than no ceiling because it still looks like one.
//
// (Some upstreams under-report instead: BaseTen returned 0 reasoning tokens on
// a response whose reasoning text was plainly there. Nothing can be recovered
// from a count that was never sent, so 0 stays 0 — this bounds the value, it
// does not invent one.)
func reasoningWithin(completion, reasoning int) int {
	if reasoning > completion {
		return completion
	}
	return reasoning
}

// completionText is the answer text, falling back to a reasoning model's
// thinking when the answer itself is empty.
//
// The fallback exists because this wire's two text fields are not
// alternatives — they are sequential. A reasoning model emits thinking first
// and the answer after it, both charged to the same output budget, so a budget
// that runs out mid-thought yields a response with every generated token in
// Reasoning and Content null. Returning Content alone hands the caller an
// empty string for a call that was generated and billed in full, with no error
// to retry on and nothing in the trace to explain it.
//
// The thinking is not as good as the answer, and it is not pretended to be:
// the caller's own schema validation will reject it, which is the honest
// outcome. What changes is that the rejection carries the text that was paid
// for, and FinishReason beside it says the budget was the cause.
func completionText(content, reasoning, finishReason string) string {
	if content != "" {
		return content
	}
	// Only when the BUDGET ran out. Empty content has several other causes on
	// this wire — a tool call, a refusal, a content filter — and in every one
	// of them the thinking is not a partial answer but a different thing
	// entirely, so handing it back as the answer would dress a refusal up as a
	// reply. Those keep their empty text, which is what the caller's own schema
	// check is there to reject.
	if finishReason == model.FinishReasonLength {
		return reasoning
	}
	return ""
}

//nolint:ireturn // model.Client.Stream returns the port's TokenStream interface by contract
func (c *openAICompatClient) Stream(ctx context.Context, req model.Request) (model.TokenStream, error) {
	body, err := c.sendChat(ctx, req, true)
	if err != nil {
		return nil, err
	}
	return &openAICompatStream{body: body, scanner: streamLineScanner(body), end: streamEnd{wire: "openai-compat"}}, nil
}

func (c *openAICompatClient) Embed(ctx context.Context, req model.EmbedRequest) (model.Embeddings, error) {
	// `dimensions` is OpenAI's matryoshka-truncation knob; the generic wire
	// gives no way to know whether the server honors it, and vLLM rejects it
	// outright on models that aren't MRL-trained (a 400 on every embed call).
	// Omit it — the store's width check catches a genuinely mismatched model.
	req.Dimensions = 0
	return openAIWireEmbed(ctx, c.post, c.defaultModel, req, c.routing.providerWire())
}

// isFetchableURL reports whether an attachment's URI is a URL the vendor can
// fetch for itself, as opposed to a handle scoped to some provider's own file
// registry. Every adapter that takes a URI has to answer this — openai to pick
// file_url over file_id, anthropic to decide whether it can send the part at
// all — so the two answer it the same way.
//
// Parsed rather than prefix-matched, because both mistakes a prefix makes are
// silent: a bare "https://" has no host to fetch and would go out as a URL, and
// a scheme in capitals — which URLs are case-insensitive in — would be handed to
// a file registry as if it were a handle.
func isFetchableURL(uri string) bool {
	parsed, err := url.Parse(uri)
	if err != nil {
		return false
	}
	return sendableHTTPScheme(parsed) && parsed.Host != ""
}

// sendableHTTPScheme reports whether a parsed URL carries a scheme this
// package's HTTP client can actually send on.
//
// One spelling, two callers: this and IsOpenRouterHost. Both ask the same
// question for the same reason — a URL without http(s) is not something a
// request can go out over, whatever else is right about it — and two copies
// would be two places to forget a scheme.
func sendableHTTPScheme(parsed *url.URL) bool {
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return true
	default:
		return false
	}
}

func (c *openAICompatClient) Caps() model.Capabilities {
	// EmbedDims stays 0 (unknown): the width is a property of whichever
	// model the deployment serves, discovered from the first Embed call.
	// LocalOnly is the provider's trust posture (vllm true, openai_compatible
	// false), fixed at construction — never a wire-visible property.
	// AttachmentMIMEs is the binding's declaration, not a constant: this one
	// adapter serves whatever model the operator pointed it at, so the answer
	// lives in the routing config and arrives at construction.
	return model.Capabilities{
		Streaming:       true,
		EmbedDims:       0,
		LocalOnly:       c.localOnly,
		AttachmentMIMEs: c.attachmentMIMEs,
	}
}

// attachmentUnsupported returns ErrAttachmentUnsupported (provider-tagged) for
// any attachment outside the adapter's declared carriage; nil otherwise. The
// map-or-reject invariant (spec §3.8): no adapter may silently drop an
// attachment.
//
// It takes the SAME declaration the adapter reports from Caps(), not a
// separately-written predicate, so a binding cannot advertise a media type its
// wire then refuses — the two halves are one list.
func attachmentUnsupported(provider string, atts []model.Attachment, declared []string) error {
	for _, a := range atts {
		if !model.CarriesMIME(declared, a.MIME) {
			return fmt.Errorf("ai: %s: %s: %w", provider, a.MIME, model.ErrAttachmentUnsupported)
		}
		// A wildcard declaration does not get to admit what no decoder reads.
		// This is the carriage sentinel because it IS a carriage answer: no
		// binding carries it, so a caller that falls back on one is right to.
		if containsFold(neverCarried, mediaKind(a.MIME)) {
			return fmt.Errorf("ai: %s: %s is not an image to any model this build talks to: %w",
				provider, a.MIME, model.ErrAttachmentUnsupported)
		}
		// Bytes XOR URI (model.Attachment): both-set would silently drop the
		// inline bytes, neither-set would emit an empty content part — reject both.
		if (len(a.Bytes) == 0) == (a.URI == "") {
			return fmt.Errorf("ai: %s: attachment %q needs exactly one of inline bytes or a uri", provider, a.MIME)
		}
		// The label got the attachment this far; the bytes decide whether it
		// goes further. Here rather than in each adapter's part builder because
		// this is already the one place every adapter asks whether it may be
		// handed an attachment at all — a fifth adapter inherits the reading by
		// existing rather than by remembering to.
		if finding := mislabelledAttachment(a); finding != "" {
			return fmt.Errorf("ai: %s: attachment claims %s but %s: %w",
				provider, a.MIME, finding, model.ErrAttachmentMislabelled)
		}
	}
	return nil
}

// errUnfetchableAttachmentURI refuses an attachment whose MIME the wire carries
// but whose URI it cannot resolve — a vendor file handle on an endpoint with no
// such registry, or a URL on a wire that takes inline bytes only.
//
// It is the carriage sentinel rather than a bare error because that is what it
// is: this binding cannot be handed this part, and a caller that falls back to
// another lane on ErrAttachmentUnsupported should fall back here too. The URI
// itself is not echoed — it can be a signed URL, and an error message is the
// wrong place for one.
func errUnfetchableAttachmentURI(provider, accepts string) error {
	return fmt.Errorf("ai: %s: this attachment's uri is not one this wire can resolve; it takes %s: %w",
		provider, accepts, model.ErrAttachmentUnsupported)
}

// refuseUnsupportedAttachments applies the map-or-reject invariant (spec §3.8)
// against THIS binding's declaration, and restates the refusal as something the
// operator can act on.
//
// On this wire the carriage is ALWAYS the operator's line — there is no adapter
// answer to fall back on — so the restatement is unconditional here, where
// refuseNarrowedAttachments makes it conditional for a wire that has one.
func (c *openAICompatClient) refuseUnsupportedAttachments(atts []model.Attachment) error {
	err := attachmentUnsupported("openai-compat", atts, c.attachmentMIMEs)
	if !errors.Is(err, model.ErrAttachmentUnsupported) {
		return err // nil, or the Bytes-XOR-URI fault, which no config line fixes
	}
	return fmt.Errorf("%w; this binding carries %s — set `input:` on its tier in the routing config to what the bound model accepts",
		err, describeCarriage(c.attachmentMIMEs))
}

// refuseNarrowedAttachments is the same invariant for an adapter whose carriage
// is fixed in its WIRE, and whose binding may have narrowed it.
//
// Which of the two refusals an operator gets is the difference between a dead
// end and an edit. A wire that cannot carry a media type has nothing to change,
// and saying "set `input:`" there would send them after a knob that cannot help.
// A binding that GAVE UP a lane its wire has is one config line from carrying it
// again, and an error stopping at "cannot carry" sends them into the adapter's
// source to find out why a capable provider refused.
func refuseNarrowedAttachments(provider string, atts []model.Attachment, declared, wireCarries []string) error {
	err := attachmentUnsupported(provider, atts, declared)
	if !errors.Is(err, model.ErrAttachmentUnsupported) || slices.Equal(declared, wireCarries) {
		return err
	}
	return fmt.Errorf("%w; this binding is narrowed to %s by its `input:` — %s itself carries %s, so widening or removing that line on its tier restores it",
		err, describeCarriage(declared), provider, describeCarriage(wireCarries))
}

// describeCarriage renders a carriage set for an error message, naming the empty
// case rather than printing "[]" at an operator.
func describeCarriage(declared []string) string {
	if len(declared) == 0 {
		return "no attachments"
	}
	return strings.Join(declared, ", ")
}

func (c *openAICompatClient) sendChat(ctx context.Context, req model.Request, stream bool) (io.ReadCloser, error) {
	// Carriage on this wire is the BINDING's declaration, not the adapter's: one
	// client serves whichever model the operator bound, and only images are
	// spelled uniformly enough here to be declarable (ai-operational-spec §1.4).
	if err := c.refuseUnsupportedAttachments(req.Attachments); err != nil {
		return nil, err
	}
	payload, _, err := sendablePayload(ctx, c.chatWire(req, stream), req.SecretStripper)
	if err != nil {
		return nil, err
	}
	return c.post(ctx, "/v1/chat/completions", payload)
}

// chatWire assembles the request body. Separated from sendChat so the body a
// binding produces can be asserted without a round trip — the strictness below
// is decided here and is invisible in a response.
func (c *openAICompatClient) chatWire(req model.Request, stream bool) openAICompatChatWire {
	wire := openAICompatChatWire{
		Model: req.Model, Stream: stream, MaxTokens: req.MaxTokens,
		Provider: c.routing.providerWire(), Reasoning: c.routing.reasoningWire(),
	}
	if wire.Model == "" {
		wire.Model = c.defaultModel
	}
	if wire.MaxTokens <= 0 {
		wire.MaxTokens = unsetMaxOutputTokens
	}
	wire.Messages = openAICompatMessages(req.System, req.Messages, req.Attachments)
	if len(req.ResponseSchema) > 0 {
		wire.ResponseFormat = &openAICompatResponseFormat{
			Type: jsonSchemaFormatType,
			JSONSchema: openAICompatJSONSchema{
				Name: openAICompatSchemaName, Schema: req.ResponseSchema,
				Strict: schemaAllowsStrict(req.ResponseSchema),
			},
		}
	}
	for _, tool := range req.Tools {
		var tw ollamaToolWire
		tw.Type = "function"
		tw.Function.Name = tool.Name
		tw.Function.Description = tool.Description
		tw.Function.Parameters = tool.InputSchema
		wire.Tools = append(wire.Tools, tw)
	}
	return wire
}

func (c *openAICompatClient) post(ctx context.Context, path string, payload []byte) (io.ReadCloser, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("ai: openai-compat: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ai: openai-compat: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		//craft:ignore swallowed-errors best-effort close on the error path — the API status error is the answer
		defer func() { _ = resp.Body.Close() }()
		return nil, openAICompatError(ctx, resp)
	}
	return resp.Body, nil
}
