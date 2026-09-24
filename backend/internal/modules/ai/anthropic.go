// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// anthropicClient is the cloud-frontier adapter (B-EP06.2): the
// Anthropic Messages API over a customer-supplied key (BYOK, ADR-0020 —
// we provide no inference; the key, endpoint and DPA are the
// customer's). stdlib HTTP only: the vendor wire format is small enough
// that an SDK would cost more in dependency surface than it saves.
type anthropicClient struct {
	http         *http.Client
	baseURL      string
	apiKey       string
	defaultModel string
	// attachmentMIMEs is what THIS binding carries: the wire's own carriage,
	// narrowed by any `input:` the operator declared (inputmodality.go). One
	// field, two uses — Caps() advertises it and send enforces it — so a binding
	// cannot advertise a media type its own gate then refuses.
	attachmentMIMEs []string
}

const anthropicAPIVersion = "2023-06-01"

type anthropicWire struct {
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`
	System    string `json:"system,omitempty"`
	// Messages is this adapter's own message type rather than a shared one:
	// only here does a turn's body become an array of Anthropic content blocks
	// once it carries an image (anthropicparts.go).
	Messages     []anthropicMessage     `json:"messages"`
	Tools        []anthropicToolWire    `json:"tools,omitempty"`
	Stream       bool                   `json:"stream,omitempty"`
	OutputConfig *anthropicOutputConfig `json:"output_config,omitempty"`
}

type anthropicToolWire struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// anthropicOutputConfig / anthropicResponseFormat carry Anthropic's native
// structured-output constraint (output_config.format). A json_schema format
// constrains the completion to the schema at generation — the same guardrail
// Ollama's `format` and vLLM's response_format provide — and the completion
// still arrives as an ordinary text block of JSON, so no response handling
// changes. Sent only when the request carries a schema, and only in the form
// the decoder can hold (anthropicschema.go).
type anthropicOutputConfig struct {
	Format *anthropicResponseFormat `json:"format,omitempty"`
}

type anthropicResponseFormat struct {
	Type   string          `json:"type"`
	Schema json.RawMessage `json:"schema"`
}

// streamedCompleteThreshold is the MaxTokens above which Complete rides
// the SSE wire and accumulates: Anthropic (and intermediaries) drop a
// non-streaming connection that stays silent for ~a minute, and a large
// completion produces no bytes until it is done. Small calls keep the
// simple wire.
const streamedCompleteThreshold = 8192

// Complete fits the response schema once, before either wire is chosen, and
// reports the downgrade that fit cost on whatever the call returns — the
// Response, or the error of a call that failed after the schema was decided,
// whose record still says what was sent.
func (c *anthropicClient) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	schema, downgrade := anthropicOutputSchema(req.ResponseSchema)
	var (
		resp model.Response
		err  error
	)
	if req.MaxTokens > streamedCompleteThreshold {
		resp, err = c.completeStreamed(ctx, req, schema)
	} else {
		resp, err = c.completePlain(ctx, req, schema)
	}
	return reportSchemaDowngrade(resp, err, downgrade)
}

func (c *anthropicClient) completePlain(ctx context.Context, req model.Request, schema json.RawMessage) (model.Response, error) {
	body, err := c.post(ctx, req, schema)
	if err != nil {
		return model.Response{}, err
	}
	//craft:ignore swallowed-errors best-effort close of a response body already read to completion — the decode result decides the outcome
	defer func() { _ = body.Close() }()
	var out struct {
		Model      string `json:"model"`
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
		Usage struct {
			InputTokens int `json:"input_tokens"`
			// CacheReadInputTokens / CacheCreationInputTokens: Anthropic reports
			// input_tokens EXCLUSIVE of both cache buckets (unlike OpenAI/Gemini,
			// which already report a cache-inclusive prompt total) — normalizing
			// below adds them back so model.Response.InputTokens lands on the
			// port's pinned cache-inclusive contract.
			CacheReadInputTokens     int                          `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int                          `json:"cache_creation_input_tokens"`
			OutputTokens             int                          `json:"output_tokens"`
			OutputTokensDetails      anthropicOutputTokensDetails `json:"output_tokens_details"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(body).Decode(&out); err != nil {
		return model.Response{}, fmt.Errorf("ai: anthropic: decode response: %w", err)
	}
	// The port's Response carries text only: tasks run in JSON mode
	// (ai-operational-spec §5.1), so a tool_use block is rendered as its
	// JSON rather than dropped silently.
	var text strings.Builder
	for _, block := range out.Content {
		switch block.Type {
		case "text":
			text.WriteString(block.Text)
		case "tool_use":
			blockJSON, _ := json.Marshal(map[string]any{"tool": block.Name, "input": block.Input})
			text.Write(blockJSON)
		}
	}
	resp := model.Response{
		Text:             text.String(),
		InputTokens:      out.Usage.InputTokens + out.Usage.CacheReadInputTokens + out.Usage.CacheCreationInputTokens,
		OutputTokens:     out.Usage.OutputTokens,
		CachedTokens:     out.Usage.CacheReadInputTokens,
		CacheWriteTokens: out.Usage.CacheCreationInputTokens,
		ReasoningTokens:  reasoningWithin(out.Usage.OutputTokens, out.Usage.OutputTokensDetails.ThinkingTokens),
		ServedModel:      out.Model,
	}
	finish, err := anthropicFinishReason(out.StopReason)
	if err != nil {
		return model.Response{}, withSpend(err, resp)
	}
	resp.FinishReason = finish
	return resp, nil
}

// anthropicOutputTokensDetails itemizes output_tokens. ThinkingTokens is the
// share that went to extended or adaptive thinking — already counted inside
// output_tokens, so it is the port's ReasoningTokens as reported, bounded by
// reasoningWithin like every wire's.
type anthropicOutputTokensDetails struct {
	ThinkingTokens int `json:"thinking_tokens"`
}

// anthropicFinishReason is a stop_reason in the port's vocabulary. Both ceilings
// — max_tokens and the model's own context window — are
// model.FinishReasonLength, because the structured retry and the cert lane's
// ungraded run read that value and no other. A refusal arrives as a 200 with
// whatever text preceded it, so it is withheld rather than returned as though
// it were the answer.
func anthropicFinishReason(stopReason string) (string, error) {
	switch stopReason {
	case "max_tokens", "model_context_window_exceeded":
		return model.FinishReasonLength, nil
	case finishRefusal:
		return "", withheldError{wire: providerAnthropic, reason: stopReason}
	default:
		return stopReason, nil
	}
}

// completeStreamed is Complete over the SSE wire: text deltas (and
// schema-constrained JSON deltas) accumulate into one response, and the
// usage counts are read off the message_start / message_delta events so
// metering stays exact.
func (c *anthropicClient) completeStreamed(ctx context.Context, req model.Request, schema json.RawMessage) (model.Response, error) {
	body, err := c.postStream(ctx, req, schema)
	if err != nil {
		return model.Response{}, err
	}
	//craft:ignore swallowed-errors best-effort close of a stream already consumed to message_stop — the scan result decides the outcome
	defer func() { _ = body.Close() }()

	var text strings.Builder
	var resp model.Response
	scanner := streamLineScanner(body)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return model.Response{}, err
		}
		data, isData := strings.CutPrefix(scanner.Text(), "data: ")
		if !isData {
			continue
		}
		var ev anthropicStreamEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return model.Response{}, fmt.Errorf("ai: anthropic: stream event: %w", err)
		}
		switch ev.Type {
		case "message_start":
			resp.InputTokens = ev.Message.Usage.InputTokens + ev.Message.Usage.CacheReadInputTokens + ev.Message.Usage.CacheCreationInputTokens
			resp.CachedTokens = ev.Message.Usage.CacheReadInputTokens
			resp.CacheWriteTokens = ev.Message.Usage.CacheCreationInputTokens
			resp.ServedModel = ev.Message.Model
		case "content_block_delta":
			text.WriteString(ev.Delta.Text)
			text.WriteString(ev.Delta.PartialJSON)
		case "message_delta":
			resp.OutputTokens = ev.Usage.OutputTokens
			resp.ReasoningTokens = reasoningWithin(ev.Usage.OutputTokens, ev.Usage.OutputTokensDetails.ThinkingTokens)
			// A delta may carry usage alone, with stop_reason null.
			if ev.Delta.StopReason == "" {
				continue
			}
			finish, err := anthropicFinishReason(ev.Delta.StopReason)
			if err != nil {
				return model.Response{}, withSpend(err, resp)
			}
			resp.FinishReason = finish
		case "message_stop":
			resp.Text = text.String()
			return resp, nil
		case sseErrorEvent:
			return model.Response{}, ev.failure(ctx)
		}
	}
	if err := scanner.Err(); err != nil {
		return model.Response{}, fmt.Errorf("ai: anthropic: stream: %w", err)
	}
	return model.Response{}, fmt.Errorf("ai: anthropic: stream ended without message_stop")
}

func (c *anthropicClient) Stream(ctx context.Context, req model.Request) (model.TokenStream, error) {
	// A stream has no Response to report a schema downgrade on, and no caller
	// that sends a schema down it.
	schema, _ := anthropicOutputSchema(req.ResponseSchema)
	body, err := c.postStream(ctx, req, schema)
	if err != nil {
		return nil, err
	}
	return &anthropicStream{body: body, scanner: streamLineScanner(body), end: streamEnd{wire: providerAnthropic}}, nil
}

// Embed is a different lane, not a chat-tier capability: Anthropic
// serves no embeddings API, and the routing config binds the embed lane
// to a local or fake embedder (ai-operational-spec §1.1).
func (c *anthropicClient) Embed(context.Context, model.EmbedRequest) (model.Embeddings, error) {
	return model.Embeddings{}, fmt.Errorf("ai: anthropic: %w", model.ErrEmbeddingsUnsupported)
}

func (c *anthropicClient) Caps() model.Capabilities {
	// AttachmentMIMEs stops at images deliberately: the Messages API's document
	// block exists, but which models accept one is a per-model fact this adapter
	// cannot see, and advertising a lane a bound model refuses is worse than not
	// advertising it at all.
	return model.Capabilities{Streaming: true, EmbedDims: 0, LocalOnly: false, AttachmentMIMEs: c.attachmentMIMEs}
}

// post sends one non-streaming Messages call; postStream opens the SSE
// variant of the same call. Two names so a call site says which wire
// mode it gets instead of passing a bare boolean. Both send schema, the
// response schema already fitted to the decoder (anthropicschema.go), or no
// output_config when it is nil.
func (c *anthropicClient) post(ctx context.Context, req model.Request, schema json.RawMessage) (io.ReadCloser, error) {
	return c.send(ctx, req, schema, false)
}

func (c *anthropicClient) postStream(ctx context.Context, req model.Request, schema json.RawMessage) (io.ReadCloser, error) {
	return c.send(ctx, req, schema, true)
}

// send performs one Messages call, attaching the output_config.format
// guardrail fitted to what the decoder enforces. The fit is settled before the
// request leaves, so a 400 here is the vendor's answer about the request and
// is never retried with the schema cleared.
func (c *anthropicClient) send(ctx context.Context, req model.Request, schema json.RawMessage, stream bool) (io.ReadCloser, error) {
	// Images map to native content blocks; a PDF does not, because
	// `document` support is model-dependent here in a way image support is not,
	// and this adapter cannot see which model the binding named. Anything outside
	// the declaration is refused rather than dropped (spec §3.8).
	if err := anthropicRefuseAttachments(req.Attachments, c.attachmentMIMEs); err != nil {
		return nil, err
	}
	wire := anthropicWire{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		System:    req.System,
		Messages:  anthropicMessages(req.Messages, req.Attachments),
		Stream:    stream,
	}
	if wire.Model == "" {
		wire.Model = c.defaultModel
	}
	if wire.MaxTokens <= 0 {
		wire.MaxTokens = unsetMaxOutputTokens
	}
	for _, tool := range req.Tools {
		wire.Tools = append(wire.Tools, anthropicToolWire{
			Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema,
		})
	}
	if len(schema) > 0 {
		wire.OutputConfig = &anthropicOutputConfig{
			Format: &anthropicResponseFormat{Type: jsonSchemaFormatType, Schema: schema},
		}
	}
	payload, _, err := sendablePayload(ctx, wire, req.SecretStripper)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("ai: anthropic: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Api-Key", c.apiKey)
	httpReq.Header.Set("Anthropic-Version", anthropicAPIVersion)
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ai: anthropic: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		//craft:ignore swallowed-errors best-effort close on the error path — the API status error is the answer
		defer func() { _ = resp.Body.Close() }()
		return nil, anthropicError(ctx, resp)
	}
	return resp.Body, nil
}

// anthropicError surfaces the API's error type and message — and only
// those, so a logged failure can never echo the request (or the key).
//
// It never marks the request rejected: Anthropic answers invalid_request_error
// for a malformed body, a prompt too long for the model, a parameter the model
// does not take and a spend limit alike, so no type here means "malformed".
func anthropicError(ctx context.Context, resp *http.Response) error {
	var apiErr struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if readErr == nil && json.Unmarshal(raw, &apiErr) == nil && apiErr.Error.Type != "" {
		return providerRefusal(resp, "", fmt.Errorf("ai: anthropic: %s: %s (http %d)",
			safeProviderText(ctx, apiErr.Error.Type), safeProviderText(ctx, apiErr.Error.Message), resp.StatusCode))
	}
	return providerRefusal(resp, "", fmt.Errorf("ai: anthropic: http %d", resp.StatusCode))
}

// anthropicStream parses the Messages SSE stream, yielding text deltas. The
// reply's terminal is message_delta's stop_reason, read through the same
// anthropicFinishReason Complete uses; message_stop closes the stream after it.
type anthropicStream struct {
	body    io.ReadCloser
	scanner *bufio.Scanner
	end     streamEnd
	// finish is the stop_reason message_delta reported, held until
	// message_stop confirms the reply is over.
	finish string
}

// anthropicStreamEvent is one Messages SSE event, as both readers of the
// stream decode it: Complete over SSE and the token stream.
type anthropicStreamEvent struct {
	Type    string `json:"type"`
	Message struct {
		Model string `json:"model"`
		Usage struct {
			InputTokens int `json:"input_tokens"`
			// Same cache-inclusive normalization as the non-streaming path
			// (see the Complete usage struct above): Anthropic's
			// message_start usage also reports input_tokens exclusive of
			// both cache buckets.
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	Usage struct {
		OutputTokens        int                          `json:"output_tokens"`
		OutputTokensDetails anthropicOutputTokensDetails `json:"output_tokens_details"`
	} `json:"usage"`
	// Error is the `error` event's body: an overload or a server fault after
	// the 200 went out.
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// failure is an `error` event as the caller sees it: the provider's own reason,
// never mistaken for a connection that simply dropped.
func (ev anthropicStreamEvent) failure(ctx context.Context) error {
	return fmt.Errorf("ai: anthropic: stream error: %s: %s",
		safeProviderText(ctx, ev.Error.Type), safeProviderText(ctx, ev.Error.Message))
}

func (s *anthropicStream) Next(ctx context.Context) (string, bool, error) {
	for !s.end.read && s.scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return "", false, err
		}
		data, isData := strings.CutPrefix(s.scanner.Text(), "data: ")
		if !isData {
			continue
		}
		var ev anthropicStreamEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return "", false, fmt.Errorf("ai: anthropic: stream event: %w", err)
		}
		switch ev.Type {
		case "content_block_delta":
			if ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
				return ev.Delta.Text, true, nil
			}
		case "message_delta":
			// A delta may carry usage alone, with stop_reason null.
			if ev.Delta.StopReason == "" {
				continue
			}
			finish, err := anthropicFinishReason(ev.Delta.StopReason)
			if err != nil {
				return "", false, err
			}
			s.finish = finish
		case "message_stop":
			s.end.finish(s.finish)
		case sseErrorEvent:
			return "", false, ev.failure(ctx)
		}
	}
	return s.end.outcome(s.scanner.Err())
}

func (s *anthropicStream) Close() error { return s.body.Close() }
