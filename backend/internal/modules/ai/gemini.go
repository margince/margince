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

// geminiClient is the native Google Gemini adapter (BYOK, ADR-0020) speaking
// the v1beta generateContent surface — not the OpenAI-compat layer, which
// cannot reach the Files API for document input. The native wire is what
// carries attachments (inlineData/fileData), thinking control
// (thinkingConfig.thinkingLevel), full-JSON-Schema structured output
// (responseFormat.text.schema), and the thought-signature continuity channel. stdlib
// HTTP only, mirroring anthropic.go; no vendor SDK. Field tags are camelCase to
// match Google's wire (see the //nolint:tagliatelle markers).
type geminiClient struct {
	http         *http.Client
	baseURL      string
	apiKey       string
	defaultModel string
	// attachmentMIMEs is what THIS binding carries: the wire's own carriage,
	// narrowed by any `input:` the operator declared (inputmodality.go).
	attachmentMIMEs []string
	// thinkingLevel is the binding's own level, sent when a request names none.
	thinkingLevel string
}

// geminiEmbedModel is Gemini's dedicated embedding model; the chat model id
// does not serve :embedContent.
const geminiEmbedModel = "gemini-embedding-001"

type geminiWire struct {
	Contents          []geminiContent  `json:"contents"`
	SystemInstruction *geminiContent   `json:"systemInstruction,omitempty"` //nolint:tagliatelle // Google's wire format (camelCase)
	GenerationConfig  *geminiGenConfig `json:"generationConfig,omitempty"`  //nolint:tagliatelle // Google's wire format (camelCase)
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

// geminiPart is one content part; only the field relevant to the part kind is
// populated. ThoughtSignature rides the model turn for reasoning continuity.
type geminiPart struct {
	Text             string            `json:"text,omitempty"`
	InlineData       *geminiInlineData `json:"inlineData,omitempty"`       //nolint:tagliatelle // Google's wire format (camelCase)
	FileData         *geminiFileData   `json:"fileData,omitempty"`         //nolint:tagliatelle // Google's wire format (camelCase)
	ThoughtSignature string            `json:"thoughtSignature,omitempty"` //nolint:tagliatelle // Google's wire format (camelCase)
}

type geminiInlineData struct {
	MimeType string `json:"mimeType"` //nolint:tagliatelle // Google's wire format (camelCase)
	Data     string `json:"data"`
}

type geminiFileData struct {
	MimeType string `json:"mimeType"` //nolint:tagliatelle // Google's wire format (camelCase)
	FileURI  string `json:"fileUri"`  //nolint:tagliatelle // Google's wire format (camelCase)
}

// geminiGenConfig carries structured-output and thinking controls. Structured
// output rides responseFormat.text, which takes a full JSON Schema: v1beta marks
// both older spellings deprecated in its favour — responseSchema (the OpenAPI
// subset) and responseJsonSchema with its responseMimeType pairing.
type geminiGenConfig struct {
	MaxOutputTokens int                   `json:"maxOutputTokens,omitempty"` //nolint:tagliatelle // Google's wire format (camelCase)
	ResponseFormat  *geminiResponseFormat `json:"responseFormat,omitempty"`  //nolint:tagliatelle // Google's wire format (camelCase)
	ThinkingConfig  *geminiThinking       `json:"thinkingConfig,omitempty"`  //nolint:tagliatelle // Google's wire format (camelCase)
}

// geminiResponseFormat configures output per modality; only text is asked for.
type geminiResponseFormat struct {
	Text geminiTextFormat `json:"text"`
}

// geminiTextFormat constrains the text part to a JSON Schema. MimeType is the
// enum spelling (APPLICATION_JSON), not the MIME string the deprecated
// responseMimeType took.
type geminiTextFormat struct {
	MimeType string          `json:"mimeType"` //nolint:tagliatelle // Google's wire format (camelCase)
	Schema   json.RawMessage `json:"schema"`
}

// geminiJSONOutput is TextResponseFormat's mime type for JSON output.
const geminiJSONOutput = "APPLICATION_JSON"

// geminiEmbedWire is the :embedContent request body — a single content whose
// parts carry the text to embed. outputDimensionality (MRL truncation) pins the
// vector width to the retrieval store's column when the caller asks for one.
type geminiEmbedWire struct {
	Model                string        `json:"model"`
	Content              geminiContent `json:"content"`
	OutputDimensionality int           `json:"outputDimensionality,omitempty"` //nolint:tagliatelle // Google's wire format (camelCase)
}

// geminiOptions is the vendor-only knob namespace read from
// Request.ProviderOptions["gemini"].
type geminiOptions struct {
	ThinkingLevel     string   `json:"thinking_level"`
	ThoughtSignatures []string `json:"thought_signatures"`
}

type geminiResponse struct {
	// ModelVersion is Gemini's served-identity field: the specific model
	// version that generated the response, which can differ from the alias
	// requested (e.g. a "-latest" alias resolving to a dated version).
	ModelVersion string `json:"modelVersion"` //nolint:tagliatelle // Google's wire format (camelCase)
	Candidates   []struct {
		Content geminiContent `json:"content"`
		// FinishReason names why generation stopped. STOP finishes the answer
		// and MAX_TOKENS truncates it; SAFETY / RECITATION / … withhold it and
		// surface as errors, so a filtered answer never reads as a complete one.
		FinishReason string `json:"finishReason"` //nolint:tagliatelle // Google's wire format (camelCase)
	} `json:"candidates"`
	// PromptFeedback names why the PROMPT was refused, and is the only signal
	// a blocked prompt sends: its candidates array is empty.
	PromptFeedback struct {
		BlockReason string `json:"blockReason"` //nolint:tagliatelle // Google's wire format (camelCase)
	} `json:"promptFeedback"` //nolint:tagliatelle // Google's wire format (camelCase)
	// Error is Google's mid-stream/in-body error object (a 200 stream can
	// still deliver {"error":{…}} as a chunk).
	Error struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	} `json:"error"`
	UsageMetadata struct {
		PromptTokenCount        int `json:"promptTokenCount"`        //nolint:tagliatelle // Google's wire format (camelCase)
		CandidatesTokenCount    int `json:"candidatesTokenCount"`    //nolint:tagliatelle // Google's wire format (camelCase)
		CachedContentTokenCount int `json:"cachedContentTokenCount"` //nolint:tagliatelle // Google's wire format (camelCase)
		ThoughtsTokenCount      int `json:"thoughtsTokenCount"`      //nolint:tagliatelle // Google's wire format (camelCase)
	} `json:"usageMetadata"` //nolint:tagliatelle // Google's wire format (camelCase)
}

func (c *geminiClient) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	body, err := c.generate(ctx, req, false)
	if err != nil {
		return model.Response{}, err
	}
	//craft:ignore swallowed-errors best-effort close of a response body already read to completion — the decode result decides the outcome
	defer func() { _ = body.Close() }()
	var out geminiResponse
	if err := json.NewDecoder(body).Decode(&out); err != nil {
		return model.Response{}, fmt.Errorf("ai: gemini: decode response: %w", err)
	}
	spent := model.Response{
		InputTokens: out.UsageMetadata.PromptTokenCount,
		// candidatesTokenCount EXCLUDES thinking tokens, but the port's
		// OutputTokens invariant is reasoning-inclusive (model.Response) so the
		// budget meter charges true spend on every provider — add them here.
		OutputTokens:    out.UsageMetadata.CandidatesTokenCount + out.UsageMetadata.ThoughtsTokenCount,
		CachedTokens:    out.UsageMetadata.CachedContentTokenCount,
		ReasoningTokens: out.UsageMetadata.ThoughtsTokenCount,
	}
	if err := geminiResponseError(ctx, out); err != nil {
		return model.Response{}, withSpend(err, spent)
	}
	// A non-stream response is terminal by definition, so it must name its
	// terminal — a candidate with no finishReason is a body cut short in
	// transit, not a complete answer.
	finish, terminal := geminiTerminal(out)
	if !terminal {
		return model.Response{}, fmt.Errorf("ai: gemini: response carries no terminal STOP or MAX_TOKENS")
	}
	var text strings.Builder
	var signatures []string
	for _, cand := range out.Candidates {
		for _, part := range cand.Content.Parts {
			text.WriteString(part.Text)
			if part.ThoughtSignature != "" {
				signatures = append(signatures, part.ThoughtSignature)
			}
		}
	}
	resp := spent
	resp.Text = text.String()
	resp.ServedModel = out.ModelVersion
	resp.FinishReason = geminiFinishReason(finish)
	if len(signatures) > 0 {
		if meta, err := json.Marshal(map[string][]string{"thought_signatures": signatures}); err == nil {
			resp.ProviderMetadata = map[string]json.RawMessage{"gemini": meta}
		}
	}
	return resp, nil
}

//nolint:ireturn // model.Client.Stream returns the port's TokenStream interface by contract
func (c *geminiClient) Stream(ctx context.Context, req model.Request) (model.TokenStream, error) {
	body, err := c.generate(ctx, req, true)
	if err != nil {
		return nil, err
	}
	return &geminiStream{body: body, scanner: streamLineScanner(body), end: streamEnd{wire: providerGemini}}, nil
}

func (c *geminiClient) Embed(ctx context.Context, req model.EmbedRequest) (model.Embeddings, error) {
	// Accept both the bare id and Google's canonical "models/…" form — the
	// path and wire body add the prefix themselves, so a canonical id would
	// otherwise double it (/models/models/… → 404).
	embedModel := strings.TrimPrefix(req.Model, "models/")
	if embedModel == "" {
		embedModel = geminiEmbedModel
	}
	// One :embedContent call per input (spec §3.5's named endpoint). A large
	// retrieval batch is therefore N sequential round-trips; folding onto
	// :batchEmbedContents for a single call is a follow-up.
	vectors := make([][]float32, 0, len(req.Inputs))
	dims := 0
	for _, input := range req.Inputs {
		wire := geminiEmbedWire{
			Model:                "models/" + embedModel,
			Content:              geminiContent{Parts: []geminiPart{{Text: input}}},
			OutputDimensionality: req.Dimensions, // 0 ⇒ omitted ⇒ provider default
		}
		payload, _, err := SendablePayload(ctx, wire, nil)
		if err != nil {
			return model.Embeddings{}, err
		}
		body, err := c.post(ctx, "/models/"+embedModel+":embedContent", payload)
		if err != nil {
			return model.Embeddings{}, err
		}
		var out struct {
			Embedding struct {
				Values []float32 `json:"values"`
			} `json:"embedding"`
		}
		decErr := json.NewDecoder(body).Decode(&out)
		//craft:ignore swallowed-errors best-effort close of a response body already read to completion — the decode result decides the outcome
		_ = body.Close()
		if decErr != nil {
			return model.Embeddings{}, fmt.Errorf("ai: gemini: decode embeddings: %w", decErr)
		}
		// Every vector must share one width — the store ranks against a fixed
		// column, so a ragged batch (model/version skew) is a hard error, not a
		// silently-advertised max.
		if len(vectors) == 0 {
			dims = len(out.Embedding.Values)
		} else if len(out.Embedding.Values) != dims {
			return model.Embeddings{}, fmt.Errorf("ai: gemini: embedding width skew: got %d, expected %d", len(out.Embedding.Values), dims)
		}
		vectors = append(vectors, out.Embedding.Values)
	}
	return model.Embeddings{Vectors: vectors, Dims: dims}, nil
}

func (c *geminiClient) Caps() model.Capabilities {
	return model.Capabilities{Streaming: true, EmbedDims: 0, LocalOnly: false, AttachmentMIMEs: c.attachmentMIMEs}
}

func (c *geminiClient) generate(ctx context.Context, req model.Request, stream bool) (io.ReadCloser, error) {
	// Gemini carries image and PDF parts natively; reject any other MIME rather
	// than silently drop it (spec §3.8).
	if err := refuseNarrowedAttachments("gemini", req.Attachments, c.attachmentMIMEs, geminiCarries); err != nil {
		return nil, err
	}
	// Native functionDeclarations mapping is a follow-up; reject tools rather than
	// silently drop them (the tasks run in JSON mode today, so none are passed).
	if len(req.Tools) > 0 {
		return nil, fmt.Errorf("ai: gemini: native tool-use is not implemented yet (request set %d tool(s))", len(req.Tools))
	}
	// Same id normalization as Embed: the ":generateContent" path adds the
	// "models/" prefix, so trim a canonical "models/…" id.
	genModel := strings.TrimPrefix(req.Model, "models/")
	if genModel == "" {
		genModel = strings.TrimPrefix(c.defaultModel, "models/")
	}
	opts, err := geminiReadOptions(req.ProviderOptions)
	if err != nil {
		return nil, err
	}
	if opts.ThinkingLevel == "" {
		opts.ThinkingLevel = c.thinkingLevel
	}
	wire := geminiWire{Contents: geminiContents(req.Messages, req.Attachments, opts.ThoughtSignatures)}
	if req.System != "" {
		wire.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: req.System}}}
	}
	wire.GenerationConfig = geminiGenerationConfig(req, genModel, opts)

	method := "generateContent"
	query := ""
	if stream {
		method = "streamGenerateContent"
		query = "?alt=sse"
	}
	payload, _, err := SendablePayload(ctx, wire, req.SecretStripper)
	if err != nil {
		return nil, err
	}
	// Paths are version-relative: the API version (/v1beta) lives in baseURL
	// (defaultGeminiBaseURL), so a proxy override keeps the whole prefix in one place.
	return c.post(ctx, "/models/"+genModel+":"+method+query, payload)
}

// geminiContents maps messages to the native contents array (assistant→model)
// and echoes any thought signatures back onto the model-role parts in order, so
// stateless multi-turn keeps Gemini's reasoning continuity (spec §3.5).
func geminiContents(msgs []model.Message, atts []model.Attachment, signatures []string) []geminiContent {
	contents := make([]geminiContent, 0, len(msgs))
	sigIdx := 0
	for _, m := range msgs {
		role := m.Role
		if role == roleAssistant {
			role = roleModel
		}
		part := geminiPart{Text: m.Content}
		if role == roleModel && sigIdx < len(signatures) {
			part.ThoughtSignature = signatures[sigIdx]
			sigIdx++
		}
		contents = append(contents, geminiContent{Role: role, Parts: []geminiPart{part}})
	}
	if len(atts) > 0 {
		contents = geminiAttachToLastUserTurn(contents, atts)
	}
	return contents
}

func geminiAttachToLastUserTurn(contents []geminiContent, atts []model.Attachment) []geminiContent {
	idx := -1
	for i := range contents {
		if contents[i].Role == roleUser {
			idx = i
		}
	}
	if idx == -1 {
		contents = append(contents, geminiContent{Role: roleUser})
		idx = len(contents) - 1
	}
	for _, a := range atts {
		contents[idx].Parts = append(contents[idx].Parts, geminiAttachmentPart(a))
	}
	return contents
}

func geminiAttachmentPart(a model.Attachment) geminiPart {
	if a.URI != "" {
		return geminiPart{FileData: &geminiFileData{MimeType: a.MIME, FileURI: a.URI}}
	}
	return geminiPart{InlineData: &geminiInlineData{MimeType: a.MIME, Data: base64.StdEncoding.EncodeToString(a.Bytes)}}
}

// geminiGenerationConfig takes the model id the request resolves to, because
// the thinking level a schema-constrained request gets depends on that model's
// own default.
func geminiGenerationConfig(req model.Request, modelID string, opts geminiOptions) *geminiGenConfig {
	cfg := &geminiGenConfig{}
	if req.MaxTokens > 0 {
		cfg.MaxOutputTokens = req.MaxTokens
	} else {
		cfg.MaxOutputTokens = unsetMaxOutputTokens
	}
	level := opts.ThinkingLevel
	if len(req.ResponseSchema) > 0 {
		cfg.ResponseFormat = &geminiResponseFormat{Text: geminiTextFormat{MimeType: geminiJSONOutput, Schema: req.ResponseSchema}}
		if level == "" && !geminiThinksShallowByDefault(modelID) && geminiTakesThinkingLevel(modelID) {
			level = geminiStructuredThinkingLevel
		}
	}
	if level != "" {
		cfg.ThinkingConfig = &geminiThinking{ThinkingLevel: level}
	}
	return cfg
}

func geminiReadOptions(opts map[string]json.RawMessage) (geminiOptions, error) {
	raw, ok := opts["gemini"]
	if !ok {
		return geminiOptions{}, nil
	}
	var o geminiOptions
	if err := json.Unmarshal(raw, &o); err != nil {
		return geminiOptions{}, fmt.Errorf("ai: gemini: provider options: %w", err)
	}
	return o, nil
}

func (c *geminiClient) post(ctx context.Context, path string, payload []byte) (io.ReadCloser, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("ai: gemini: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", c.apiKey)
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ai: gemini: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		//craft:ignore swallowed-errors best-effort close on the error path — the API status error is the answer
		defer func() { _ = resp.Body.Close() }()
		return nil, geminiError(ctx, resp)
	}
	return resp.Body, nil
}

// geminiStream reads the :streamGenerateContent?alt=sse stream. There is no
// [DONE] sentinel — the final chunk carries finishReason STOP (or MAX_TOKENS)
// and then the stream closes; text arrives at candidates[0].content.parts[].text
// on each chunk, the final one included.
type geminiStream struct {
	body    io.ReadCloser
	scanner *bufio.Scanner
	end     streamEnd
}

func (s *geminiStream) Next(ctx context.Context) (string, bool, error) {
	for !s.end.read && s.scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return "", false, err
		}
		data, isData := strings.CutPrefix(s.scanner.Text(), "data: ")
		if !isData {
			continue
		}
		var ev geminiResponse
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return "", false, fmt.Errorf("ai: gemini: stream event: %w", err)
		}
		// A mid-stream error object or a withholding finishReason (SAFETY, …)
		// arrives inside a 200 chunk — surface it instead of letting a
		// zero-text chunk fall through to a clean-looking EOF.
		if err := geminiResponseError(ctx, ev); err != nil {
			return "", false, err
		}
		if finish, terminal := geminiTerminal(ev); terminal {
			s.end.finish(geminiFinishReason(finish))
		}
		var chunk strings.Builder
		for _, cand := range ev.Candidates {
			for _, part := range cand.Content.Parts {
				chunk.WriteString(part.Text)
			}
		}
		if chunk.Len() > 0 {
			return chunk.String(), true, nil
		}
	}
	return s.end.outcome(s.scanner.Err())
}

func (s *geminiStream) Close() error { return s.body.Close() }
