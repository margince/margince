// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// openaiClient is the native OpenAI adapter (BYOK, ADR-0020) speaking the
// Responses API (POST /v1/responses) rather than the generic
// /v1/chat/completions the openai_compatible transport uses. The Responses
// wire is what carries native reasoning (reasoning.effort), itemized usage
// (cached + reasoning tokens), and typed image/file input parts — none of
// which the chat-completions shape expresses. stdlib HTTP only, mirroring
// anthropic.go; no vendor SDK.
type openaiClient struct {
	http         *http.Client
	baseURL      string
	apiKey       string
	defaultModel string
	// attachmentMIMEs is what THIS binding carries: the wire's own carriage,
	// narrowed by any `input:` the operator declared (inputmodality.go).
	attachmentMIMEs []string
}

type openaiWire struct {
	Model           string            `json:"model"`
	Input           []openaiInputItem `json:"input"`
	MaxOutputTokens int               `json:"max_output_tokens,omitempty"`
	Text            *openaiText       `json:"text,omitempty"`
	Reasoning       *openaiReasoning  `json:"reasoning,omitempty"`
	Stream          bool              `json:"stream,omitempty"`
	// Store is pinned false (no omitempty — the field must be on the wire):
	// the Responses API defaults to store:true, which retains prompts — CRM
	// record content — server-side for ~30 days. BYOK egress sends the
	// request for inference only, never for vendor-side retention.
	Store bool `json:"store"`
}

type openaiInputItem struct {
	Role    string            `json:"role"`
	Content []openaiInputPart `json:"content"`
}

// openaiInputPart is one content part. Only the fields relevant to the part's
// Type are populated; the rest are omitted so the wire stays minimal.
type openaiInputPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	FileData string `json:"file_data,omitempty"`
	FileName string `json:"filename,omitempty"`
	FileURL  string `json:"file_url,omitempty"`
	FileID   string `json:"file_id,omitempty"`
}

type openaiText struct {
	Format openaiResponseFormat `json:"format"`
}

// openaiResponseFormat is the Responses API structured-output shape: the
// json_schema descriptor sits directly under text.format (siblings, no
// json_schema:{} wrapper). Strict is derived exactly as on the
// openai_compatible wire: OpenAI answers strict over a schema outside its
// supported subset with an error, so only a schema that already fits is sent
// enforced.
type openaiResponseFormat struct {
	Type   string          `json:"type"`
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
	Strict bool            `json:"strict"`
}

type openaiReasoning struct {
	Effort string `json:"effort"`
}

// openaiOptions is the vendor-only knob namespace read from
// Request.ProviderOptions["openai"].
type openaiOptions struct {
	ReasoningEffort string `json:"reasoning_effort"`
}

type openaiResponse struct {
	ID string `json:"id"`
	// Model is the Responses API's served-identity field: the specific model
	// that generated this response.
	Model string `json:"model"`
	// Status is the terminal response state: "completed" is the success;
	// "failed" carries Error, "incomplete" carries IncompleteDetails (e.g.
	// max_output_tokens, content_filter). An answer cut off at
	// max_output_tokens is a truncated Response (openaiCutOff); anything else
	// surfaces as an error, so a filtered answer never reads as a clean one.
	Status string `json:"status"`
	Error  struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	IncompleteDetails struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Type    string `json:"type"`
			Text    string `json:"text"`
			Refusal string `json:"refusal"`
		} `json:"content"`
	} `json:"output"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
		// InputTokenDetails itemizes input_tokens: the cache read and, from
		// gpt-5.6 on, the cache write — both parts of the total, never added
		// to it.
		InputTokenDetails struct {
			CachedTokens     int `json:"cached_tokens"`
			CacheWriteTokens int `json:"cache_write_tokens"`
		} `json:"input_tokens_details"`
		OutputTokenDetails struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"output_tokens_details"`
	} `json:"usage"`
}

func (c *openaiClient) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	ctx, attempt := trackHTTPAttempt(ctx)
	resp, err := c.completeResponse(ctx, req)
	return reportSchemaDowngrade(resp, err, strictDowngrade(req.ResponseSchema), attempt)
}

func (c *openaiClient) completeResponse(ctx context.Context, req model.Request) (model.Response, error) {
	body, err := c.post(ctx, "/v1/responses", req, false)
	if err != nil {
		return model.Response{}, err
	}
	//craft:ignore swallowed-errors best-effort close of a response body already read to completion — the decode result decides the outcome
	defer func() { _ = body.Close() }()
	var out openaiResponse
	if err := json.NewDecoder(body).Decode(&out); err != nil {
		return model.Response{}, fmt.Errorf("ai: openai: decode response: %w", err)
	}
	resp := model.Response{
		InputTokens:     out.Usage.InputTokens,
		OutputTokens:    out.Usage.OutputTokens,
		ReasoningTokens: out.Usage.OutputTokenDetails.ReasoningTokens,
		ServedModel:     out.Model,
	}
	resp.CachedTokens, resp.CacheWriteTokens = cacheWithin(out.Usage.InputTokens,
		out.Usage.InputTokenDetails.CachedTokens, out.Usage.InputTokenDetails.CacheWriteTokens)
	cutOff := openaiCutOff(out)
	if err := openaiTerminalStatus(ctx, out); err != nil && !cutOff {
		return model.Response{}, withSpend(err, resp)
	}
	text, err := openaiReplyText(ctx, out)
	if err != nil {
		return model.Response{}, withSpend(err, resp)
	}
	resp.Text = text
	if cutOff {
		resp.FinishReason = model.FinishReasonLength
	}
	if out.ID != "" {
		if meta, err := json.Marshal(map[string]string{"response_id": out.ID}); err == nil {
			resp.ProviderMetadata = map[string]json.RawMessage{"openai": meta}
		}
	}
	return resp, nil
}

// openaiReplyText walks output[]: a type:"reasoning" item can precede the
// message, and a type:"refusal" part is a first-class outcome — never
// output[0].content[0].
func openaiReplyText(ctx context.Context, out openaiResponse) (string, error) {
	var text strings.Builder
	for _, item := range out.Output {
		if item.Type != "message" {
			continue
		}
		for _, part := range item.Content {
			switch part.Type {
			case "output_text":
				text.WriteString(part.Text)
			case finishRefusal:
				return "", withheldError{wire: providerOpenAI, reason: finishRefusal, detail: safeProviderText(ctx, part.Refusal)}
			}
		}
	}
	return text.String(), nil
}

//nolint:ireturn // model.Client.Stream returns the port's TokenStream interface by contract
func (c *openaiClient) Stream(ctx context.Context, req model.Request) (model.TokenStream, error) {
	body, err := c.post(ctx, "/v1/responses", req, true)
	if err != nil {
		return nil, err
	}
	return &openaiStream{body: body, scanner: streamLineScanner(body), end: streamEnd{wire: providerOpenAI}}, nil
}

func (c *openaiClient) Embed(ctx context.Context, req model.EmbedRequest) (model.Embeddings, error) {
	return openAIWireEmbed(ctx, c.postRaw, c.defaultModel, req, nil)
}

func (c *openaiClient) Caps() model.Capabilities {
	return model.Capabilities{Streaming: true, EmbedDims: 0, LocalOnly: false, AttachmentMIMEs: c.attachmentMIMEs}
}

func (c *openaiClient) post(ctx context.Context, path string, req model.Request, stream bool) (io.ReadCloser, error) {
	// OpenAI carries image and PDF/file parts natively; reject any other MIME
	// rather than silently drop it (spec §3.8).
	if err := refuseNarrowedAttachments("openai", req.Attachments, c.attachmentMIMEs, openAICarries); err != nil {
		return nil, err
	}
	// Native Responses-API tool mapping is a follow-up; reject tools rather than
	// silently drop them (the tasks run in JSON mode today, so none are passed).
	if len(req.Tools) > 0 {
		return nil, fmt.Errorf("ai: openai: native tool-use is not implemented yet (request set %d tool(s))", len(req.Tools))
	}
	wire := openaiWire{Model: req.Model, MaxOutputTokens: req.MaxTokens, Stream: stream}
	if wire.Model == "" {
		wire.Model = c.defaultModel
	}
	if wire.MaxOutputTokens <= 0 {
		wire.MaxOutputTokens = unsetMaxOutputTokens
	}
	wire.Input = openaiInputMessages(req.System, req.Messages, req.Attachments)
	if len(req.ResponseSchema) > 0 {
		wire.Text = &openaiText{Format: openaiResponseFormat{
			Type: jsonSchemaFormatType, Name: openAICompatSchemaName, Schema: req.ResponseSchema,
			Strict: schemaAllowsStrict(req.ResponseSchema),
		}}
	}
	effort, err := openaiReasoningEffort(req.ProviderOptions)
	if err != nil {
		return nil, err
	}
	if effort == "" {
		effort = openaiEffortFor(wire.Model, req.ThinkingFloor)
	}
	if effort != "" {
		wire.Reasoning = &openaiReasoning{Effort: effort}
	}
	payload, _, err := SendablePayload(ctx, wire, req.SecretStripper)
	if err != nil {
		return nil, err
	}
	return c.postRaw(ctx, path, payload)
}

// openaiInputMessages builds the Responses `input` array: system as a leading
// message, each turn's text as an input_text/output_text part, and every
// attachment appended to the final user turn's content.
func openaiInputMessages(system string, msgs []model.Message, atts []model.Attachment) []openaiInputItem {
	items := make([]openaiInputItem, 0, len(msgs)+1)
	if system != "" {
		items = append(items, openaiInputItem{Role: roleSystem, Content: []openaiInputPart{{Type: "input_text", Text: system}}})
	}
	for _, m := range msgs {
		partType := "input_text"
		if m.Role == roleAssistant {
			partType = "output_text"
		}
		items = append(items, openaiInputItem{Role: m.Role, Content: []openaiInputPart{{Type: partType, Text: m.Content}}})
	}
	if len(atts) > 0 {
		items = attachToLastUserTurn(items, atts)
	}
	return items
}

// attachToLastUserTurn appends attachment parts to the last user-role item,
// adding a user item if none exists — attachments belong to a user turn.
func attachToLastUserTurn(items []openaiInputItem, atts []model.Attachment) []openaiInputItem {
	idx := -1
	for i := range items {
		if items[i].Role == roleUser {
			idx = i
		}
	}
	if idx == -1 {
		items = append(items, openaiInputItem{Role: roleUser})
		idx = len(items) - 1
	}
	for _, a := range atts {
		items[idx].Content = append(items[idx].Content, openaiAttachmentPart(a))
	}
	return items
}

func openaiAttachmentPart(a model.Attachment) openaiInputPart {
	if isImage(a.MIME) {
		part := openaiInputPart{Type: "input_image"}
		if a.URI != "" {
			part.ImageURL = a.URI
		} else {
			part.ImageURL = dataURI(a.MIME, a.Bytes)
		}
		return part
	}
	// application/pdf (the only other allowed MIME, gated in post). A URI is
	// either a public URL (file_url) or an OpenAI file handle (file_id); inline
	// bytes ride file_data.
	part := openaiInputPart{Type: "input_file", FileName: a.Name}
	switch {
	case a.URI == "":
		part.FileData = dataURI(a.MIME, a.Bytes)
	case isFetchableURL(a.URI):
		part.FileURL = a.URI
	default:
		part.FileID = a.URI
	}
	return part
}

func dataURI(mime string, raw []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw)
}

func openaiReasoningEffort(opts map[string]json.RawMessage) (string, error) {
	raw, ok := opts["openai"]
	if !ok {
		return "", nil
	}
	var o openaiOptions
	if err := json.Unmarshal(raw, &o); err != nil {
		return "", fmt.Errorf("ai: openai: provider options: %w", err)
	}
	return o.ReasoningEffort, nil
}

func (c *openaiClient) postRaw(ctx context.Context, path string, payload []byte) (io.ReadCloser, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("ai: openai: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := sendModelRequest(c.http, httpReq)
	if err != nil {
		return nil, fmt.Errorf("ai: openai: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		//craft:ignore swallowed-errors best-effort close on the error path — the API status error is the answer
		defer func() { _ = resp.Body.Close() }()
		return nil, openaiError(ctx, resp)
	}
	return resp.Body, nil
}

// openaiError surfaces the API's error type and message — and only those, so a
// logged failure can never echo the request (or the key). Both are redacted,
// and the code decides whether the request itself was malformed or its content
// was refused by policy — the latter the same withholding a failed response
// with that code reports.
func openaiError(ctx context.Context, resp *http.Response) error {
	var apiErr struct {
		Error struct {
			Type    string          `json:"type"`
			Message string          `json:"message"`
			Code    json.RawMessage `json:"code"`
		} `json:"error"`
	}
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if readErr == nil && json.Unmarshal(raw, &apiErr) == nil && apiErr.Error.Type != "" {
		var code string
		if json.Unmarshal(apiErr.Error.Code, &code) == nil && openaiPolicyCodes[code] {
			return withheldError{wire: providerOpenAI, reason: code, detail: safeProviderText(ctx, apiErr.Error.Message)}
		}
		err := providerRefusal(resp, "", fmt.Errorf("ai: openai: %s: %s (http %d)",
			safeProviderText(ctx, apiErr.Error.Type), safeProviderText(ctx, apiErr.Error.Message), resp.StatusCode))
		if openAIRejectsTheRequest(apiErr.Error.Code) {
			return rejectedRequest(err)
		}
		return err
	}
	return providerRefusal(resp, "", fmt.Errorf("ai: openai: http %d", resp.StatusCode))
}

// openaiCutOff reports a response the output ceiling stopped: an answer,
// truncated, rather than a failed call. Only Complete reads it — a stream ends
// on truncatedError, the port's model.ErrOutputTruncated.
func openaiCutOff(out openaiResponse) bool {
	return out.Status == "incomplete" && out.IncompleteDetails.Reason == openaiMaxOutputTokens
}

// openaiTerminalStatus maps a non-completed Responses object to an error: a
// failed call carries the API's error, an incomplete one names why generation
// stopped (max_output_tokens, content_filter), and a missing status means the
// body was not a terminal Responses object at all. Any of them read as a clean
// answer would silently hand the caller a truncated or filtered result —
// "completed" is the only success.
func openaiTerminalStatus(ctx context.Context, out openaiResponse) error {
	switch out.Status {
	case "completed":
		return nil
	case "failed":
		if openaiPolicyCodes[out.Error.Code] {
			return withheldError{wire: providerOpenAI, reason: out.Error.Code, detail: safeProviderText(ctx, out.Error.Message)}
		}
		return fmt.Errorf("ai: openai: response failed: %s: %s", safeProviderText(ctx, out.Error.Code), safeProviderText(ctx, out.Error.Message))
	case "incomplete":
		switch out.IncompleteDetails.Reason {
		case finishContentFilter:
			return withheldError{wire: providerOpenAI, reason: finishContentFilter}
		case openaiMaxOutputTokens:
			return truncatedError{wire: providerOpenAI}
		}
		return fmt.Errorf("ai: openai: response incomplete: %s", safeProviderText(ctx, out.IncompleteDetails.Reason))
	case "":
		return fmt.Errorf("ai: openai: response carries no terminal status")
	default:
		return fmt.Errorf("ai: openai: response ended with status %q", safeProviderText(ctx, out.Status))
	}
}

// openaiPolicyCodes are the codes, on a failed response or a 400, that are a
// policy decision about the content, so the answer is withheld rather than the
// call failed.
var openaiPolicyCodes = map[string]bool{"invalid_prompt": true, "bio_policy": true}

// openaiMaxOutputTokens is the incomplete reason for a reply the output
// ceiling cut off.
const openaiMaxOutputTokens = "max_output_tokens"

// openaiStream parses the Responses SSE stream, yielding text deltas from
// response.output_text.delta events. response.completed is the ONLY clean
// terminal — failed/incomplete/error events surface as errors, never as EOF.
type openaiStream struct {
	body    io.ReadCloser
	scanner *bufio.Scanner
	end     streamEnd
}

func (s *openaiStream) Next(ctx context.Context) (string, bool, error) {
	for !s.end.read && s.scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return "", false, err
		}
		line := s.scanner.Text()
		data, isData := strings.CutPrefix(line, "data: ")
		if !isData {
			continue
		}
		var ev struct {
			Type     string         `json:"type"`
			Delta    string         `json:"delta"`
			Response openaiResponse `json:"response"`
			// Code/Message are the top-level `error` event's shape.
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return "", false, fmt.Errorf("ai: openai: stream event: %w", err)
		}
		switch ev.Type {
		case "response.output_text.delta":
			if ev.Delta != "" {
				return ev.Delta, true, nil
			}
		case "response.completed":
			s.end.finish("")
		case "response.failed", "response.incomplete":
			if err := openaiTerminalStatus(ctx, ev.Response); err != nil {
				return "", false, err
			}
			// The embedded response object omitted its status — the event
			// type itself is still the authority that this is a failure.
			return "", false, fmt.Errorf("ai: openai: stream ended with %s", ev.Type)
		case sseErrorEvent:
			return "", false, fmt.Errorf("ai: openai: stream error: %s: %s", safeProviderText(ctx, ev.Code), safeProviderText(ctx, ev.Message))
		}
	}
	return s.end.outcome(s.scanner.Err())
}

func (s *openaiStream) Close() error { return s.body.Close() }
