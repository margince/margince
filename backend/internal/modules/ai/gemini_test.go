// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Built through SelectBrain rather than by struct literal: carriage is decided
// there now (the wire's own set, narrowed by any `input:`), so a hand-built
// client would be a second, differently-configured production that proves
// nothing about the one that ships.
// testGeminiKey is the BYOK key this suite supplies, named so a case can assert
// the client sent THIS key rather than any key at all.
const testGeminiKey = "gk"

func newGeminiForTest(t *testing.T, handler http.HandlerFunc) model.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client, err := selectLocalBrain(ProviderConfig{Provider: providerGemini, BaseURL: srv.URL, Model: "gemini-x"}, cloudKeyFor("gemini", testGeminiKey))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestGeminiCompleteMapsNativeWireAndUsage(t *testing.T) {
	var body []byte
	client := newGeminiForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/models/gemini-x:generateContent") {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "gk" {
			t.Errorf("api key header %q", r.Header.Get("x-goog-api-key"))
		}
		body = readBody(t, r.Body)
		_, _ = w.Write([]byte(`{"modelVersion":"gemini-x-001","candidates":[{"content":{"role":"model","parts":[{"text":"answer"}]},"finishReason":"STOP"}],
			"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"cachedContentTokenCount":6,"thoughtsTokenCount":4}}`))
	})
	resp, err := client.Complete(context.Background(), model.Request{
		System:   "be terse",
		Messages: []model.Message{{Role: "user", Content: "q"}, {Role: "assistant", Content: "prior"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// OutputTokens is reasoning-inclusive (the port invariant): Gemini reports
	// candidates (5) and thoughts (4) separately, so the adapter sums them.
	if resp.Text != "answer" || resp.InputTokens != 10 || resp.OutputTokens != 9 || resp.CachedTokens != 6 || resp.ReasoningTokens != 4 {
		t.Fatalf("mapping wrong: %+v", resp)
	}
	// Gemini's promptTokenCount is already cache-inclusive (no separate
	// cache-write bucket on the wire), so CacheWriteTokens must stay at its
	// zero-value.
	if resp.CacheWriteTokens != 0 {
		t.Fatalf("CacheWriteTokens = %d, want 0 (Gemini reports no cache-write bucket)", resp.CacheWriteTokens)
	}
	if resp.ServedModel != "gemini-x-001" {
		t.Fatalf("ServedModel not decoded from modelVersion: %q", resp.ServedModel)
	}
	var wire struct {
		Contents []struct {
			Role  string `json:"role"`
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
		SystemInstruction struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"systemInstruction"` //nolint:tagliatelle // Google's wire format (camelCase)
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.SystemInstruction.Parts[0].Text != "be terse" {
		t.Fatalf("system not mapped to systemInstruction: %s", body)
	}
	if len(wire.Contents) != 2 || wire.Contents[0].Role != "user" || wire.Contents[1].Role != "model" {
		t.Fatalf("roles not mapped (assistant→model): %+v", wire.Contents)
	}
}

func TestGeminiStructuredOutputRidesResponseFormat(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}}}`)
	var body []byte
	client := newGeminiForTest(t, func(w http.ResponseWriter, r *http.Request) {
		body = readBody(t, r.Body)
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{}"}]},"finishReason":"STOP"}]}`))
	})
	if _, err := client.Complete(context.Background(), model.Request{
		Messages:       []model.Message{{Role: "user", Content: "hi"}},
		ResponseSchema: schema,
	}); err != nil {
		t.Fatal(err)
	}
	var wire struct {
		GenerationConfig map[string]json.RawMessage `json:"generationConfig"` //nolint:tagliatelle // Google's wire format (camelCase)
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	// Both older spellings are deprecated in v1beta; sending either beside
	// responseFormat would be two schemas on one request.
	for _, deprecated := range []string{"responseMimeType", "responseJsonSchema", "responseSchema"} {
		if _, sent := wire.GenerationConfig[deprecated]; sent {
			t.Errorf("deprecated %s sent: %s", deprecated, body)
		}
	}
	var format struct {
		Text struct {
			MimeType string          `json:"mimeType"` //nolint:tagliatelle // Google's wire format (camelCase)
			Schema   json.RawMessage `json:"schema"`
		} `json:"text"`
	}
	if err := json.Unmarshal(wire.GenerationConfig["responseFormat"], &format); err != nil {
		t.Fatalf("responseFormat absent or malformed: %v: %s", err, body)
	}
	if format.Text.MimeType != "APPLICATION_JSON" {
		t.Fatalf("responseFormat.text.mimeType = %q, want APPLICATION_JSON", format.Text.MimeType)
	}
	if !bytes.Equal(bytes.TrimSpace(format.Text.Schema), bytes.TrimSpace(schema)) {
		t.Fatalf("responseFormat.text.schema not verbatim: %s", format.Text.Schema)
	}
}

// A structured request thinks at low unless its caller chose a level, because
// Gemini's thinking is charged to the same maxOutputTokens the answer needs; a
// free-text request keeps the model's own default.
func TestGeminiThinkingDefaultsLowOnlyForAStructuredRequest(t *testing.T) {
	schema := json.RawMessage(`{"type":"object"}`)
	cases := []struct {
		name   string
		schema json.RawMessage
		chosen string
		want   string
	}{
		{"structured, no level chosen", schema, "", geminiStructuredThinkingLevel},
		{"structured, caller chose high", schema, "high", "high"},
		{"free text, no level chosen", nil, "", ""},
		{"free text, caller chose medium", nil, "medium", "medium"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := geminiGenerationConfig(model.Request{ResponseSchema: tc.schema}, geminiOptions{ThinkingLevel: tc.chosen})
			got := ""
			if cfg.ThinkingConfig != nil {
				got = cfg.ThinkingConfig.ThinkingLevel
			}
			if got != tc.want {
				t.Fatalf("thinking level = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGeminiThinkingLevelFromProviderOptions(t *testing.T) {
	var body []byte
	client := newGeminiForTest(t, func(w http.ResponseWriter, r *http.Request) {
		body = readBody(t, r.Body)
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
	})
	if _, err := client.Complete(context.Background(), model.Request{
		Messages:        []model.Message{{Role: "user", Content: "hi"}},
		ProviderOptions: map[string]json.RawMessage{"gemini": json.RawMessage(`{"thinking_level":"low"}`)},
	}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`"thinkingLevel":"low"`)) {
		t.Fatalf("thinkingLevel not on wire: %s", body)
	}
}

func TestGeminiMapsImageAndPDFAttachmentsToInlineData(t *testing.T) {
	var body []byte
	client := newGeminiForTest(t, func(w http.ResponseWriter, r *http.Request) {
		body = readBody(t, r.Body)
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
	})
	if _, err := client.Complete(context.Background(), model.Request{
		Messages: []model.Message{{Role: "user", Content: "read these"}},
		Attachments: []model.Attachment{
			{MIME: "image/png", Bytes: pngSample},
			{MIME: "application/pdf", Bytes: pdfSample},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`"inlineData"`)) || !bytes.Contains(body, []byte(`"image/png"`)) || !bytes.Contains(body, []byte(`"application/pdf"`)) {
		t.Fatalf("attachments not mapped to inlineData: %s", body)
	}
}

func TestGeminiStripsSecretsFromWire(t *testing.T) {
	var body []byte
	client := newGeminiForTest(t, func(w http.ResponseWriter, r *http.Request) {
		body = readBody(t, r.Body)
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
	})
	if _, err := client.Complete(context.Background(), model.Request{
		Messages:       []model.Message{{Role: "user", Content: "with password=verysecretpw inside"}},
		SecretStripper: NewSecretStripper(),
	}); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("verysecretpw")) {
		t.Fatalf("secret reached the wire: %s", body)
	}
}

func TestGeminiEmbedReturnsVectorsAndPinsOutputDimensionality(t *testing.T) {
	var body []byte
	client := newGeminiForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, ":embedContent") {
			t.Errorf("wrong path %s", r.URL.Path)
		}
		body = readBody(t, r.Body)
		_, _ = w.Write([]byte(`{"embedding":{"values":[0.1,0.2,0.3]}}`))
	})
	res, err := client.Embed(context.Background(), model.EmbedRequest{Inputs: []string{"a"}, Dimensions: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if res.Dims != 3 || len(res.Vectors) != 1 {
		t.Fatalf("unexpected shape: %+v", res)
	}
	if !bytes.Contains(body, []byte(`"outputDimensionality":1024`)) {
		t.Fatalf("outputDimensionality not sent to match the store column: %s", body)
	}
}

func TestGeminiReportsNotLocalOnly(t *testing.T) {
	client, err := SelectBrain(ProviderConfig{Provider: "gemini", Model: "gemini-x"}, allCloudKeys())
	if err != nil {
		t.Fatal(err)
	}
	if client.Caps().LocalOnly {
		t.Fatal("gemini is a cloud provider — LocalOnly must be false")
	}
}

func TestGeminiFailsClosedWithoutKey(t *testing.T) {
	if _, err := SelectBrain(ProviderConfig{Provider: "gemini"}, noCloudKeys()); err == nil || !strings.Contains(err.Error(), "api key") {
		t.Fatalf("gemini without a key must fail closed, got %v", err)
	}
}

func TestGeminiStreamYieldsPartTextChunks(t *testing.T) {
	client := newGeminiForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, ":streamGenerateContent") || r.URL.RawQuery != "alt=sse" {
			t.Errorf("stream path/query wrong: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `data: {"candidates":[{"content":{"parts":[{"text":"he"}]}}]}`+"\n\n")
		_, _ = io.WriteString(w, `data: {"candidates":[{"content":{"parts":[{"text":"llo"}]},"finishReason":"STOP"}]}`+"\n\n")
	})
	stream, err := client.Stream(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := stream.Close(); err != nil {
			t.Errorf("closing stream: %v", err)
		}
	}()
	var got strings.Builder
	for {
		chunk, ok, err := stream.Next(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		got.WriteString(chunk)
	}
	if got.String() != "hello" {
		t.Fatalf("stream mismatch: %q", got.String())
	}
}

func TestGeminiErrorSurfacesStatusAndMessageOnly(t *testing.T) {
	client := newGeminiForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"status":"INVALID_ARGUMENT","message":"bad request"}}`))
	})
	_, err := client.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "with password=verysecretpw inside"}}})
	if err == nil || !strings.Contains(err.Error(), "INVALID_ARGUMENT") || !strings.Contains(err.Error(), "bad request") {
		t.Fatalf("want vendor status+message, got %v", err)
	}
	if strings.Contains(err.Error(), "verysecretpw") {
		t.Fatalf("error must not echo the request: %v", err)
	}
}

func TestGeminiMapsAttachmentByURIToFileData(t *testing.T) {
	var body []byte
	client := newGeminiForTest(t, func(w http.ResponseWriter, r *http.Request) {
		body = readBody(t, r.Body)
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
	})
	if _, err := client.Complete(context.Background(), model.Request{
		Messages:    []model.Message{{Role: "user", Content: "look"}},
		Attachments: []model.Attachment{{MIME: "application/pdf", URI: "gs://bucket/contract.pdf"}},
	}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`"fileData"`)) || !bytes.Contains(body, []byte("gs://bucket/contract.pdf")) {
		t.Fatalf("URI attachment not carried as fileData: %s", body)
	}
}

func TestGeminiThoughtSignatureRoundTrips(t *testing.T) {
	// (1) A response carrying a thoughtSignature on the model part surfaces it in ProviderMetadata.
	var reqBody []byte
	client := newGeminiForTest(t, func(w http.ResponseWriter, r *http.Request) {
		reqBody = readBody(t, r.Body)
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[
			{"text":"answer","thoughtSignature":"SIG-abc"}]},"finishReason":"STOP"}],
			"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":2}}`))
	})
	resp, err := client.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "q1"}}})
	if err != nil {
		t.Fatal(err)
	}
	meta := resp.ProviderMetadata["gemini"]
	if !bytes.Contains(meta, []byte("SIG-abc")) {
		t.Fatalf("thought signature not surfaced in ProviderMetadata: %s", meta)
	}

	// (2) On the NEXT call, a signature passed back in ProviderOptions is echoed onto the model part.
	_, err = client.Complete(context.Background(), model.Request{
		Messages: []model.Message{
			{Role: "user", Content: "q1"},
			{Role: "assistant", Content: "answer"},
			{Role: "user", Content: "q2"},
		},
		ProviderOptions: map[string]json.RawMessage{"gemini": json.RawMessage(`{"thought_signatures":["SIG-abc"]}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(reqBody, []byte(`"thoughtSignature":"SIG-abc"`)) {
		t.Fatalf("thought signature not echoed onto the model turn: %s", reqBody)
	}
}

// SAFETY / RECITATION arrive inside a 200 body — a withholding finishReason
// must surface as an error, never as a clean answer. MAX_TOKENS is not one:
// finishreasonparity_test.go holds it to a truncated Response.
func TestGeminiAbnormalFinishReasonIsAnError(t *testing.T) {
	client := newGeminiForTest(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"withheld"}]},"finishReason":"SAFETY"}]}`))
	})
	_, err := client.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "q"}}})
	if err == nil || !strings.Contains(err.Error(), "SAFETY") {
		t.Fatalf("want error naming SAFETY, got %v", err)
	}
}

// The terminal has to survive as DATA, not only inside the message: every
// withholding finishReason classifies to the one `output_withheld` sentinel, so
// without an accessor the stored row cannot separate a refused answer (SAFETY —
// retrying is pointless) from a recited one (RECITATION — change the prompt).
func TestGeminiAbnormalFinishReasonCarriesTheTerminalAsData(t *testing.T) {
	for _, reason := range []string{"SAFETY", "RECITATION", "PROHIBITED_CONTENT"} {
		t.Run(reason, func(t *testing.T) {
			client := newGeminiForTest(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"x"}]},"finishReason":"` + reason + `"}]}`))
			})
			_, err := client.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "q"}}})
			if err == nil {
				t.Fatal("want an error for an abnormal finishReason")
			}
			if !errors.Is(err, model.ErrOutputWithheld) || !strings.Contains(err.Error(), reason) {
				t.Fatalf("want a withheld answer naming %s, got %v", reason, err)
			}
			var stopped interface{ FinishReason() string }
			if !errors.As(err, &stopped) {
				t.Fatalf("error does not expose FinishReason(), so the trace cannot record it: %T", err)
			}
			if got := stopped.FinishReason(); got != reason {
				t.Fatalf("FinishReason() = %q, want %q", got, reason)
			}
		})
	}
}

// A mid-stream error object and an abnormal finishReason both ride 200 SSE
// chunks; either passing for EOF would report a failed call as complete.
func TestGeminiStreamSurfacesErrorChunkAndAbnormalFinish(t *testing.T) {
	cases := map[string]struct {
		chunk string
		want  string
	}{
		"error object":  {`data: {"error":{"status":"RESOURCE_EXHAUSTED","message":"quota"}}`, "RESOURCE_EXHAUSTED"},
		"safety finish": {`data: {"candidates":[{"content":{"parts":[]},"finishReason":"SAFETY"}]}`, "SAFETY"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			client := newGeminiForTest(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, `data: {"candidates":[{"content":{"parts":[{"text":"he"}]}}]}`+"\n\n")
				_, _ = io.WriteString(w, tc.chunk+"\n\n")
			})
			stream, err := client.Stream(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := stream.Close(); err != nil {
					t.Errorf("closing stream: %v", err)
				}
			}()
			if chunk, ok, err := stream.Next(context.Background()); err != nil || !ok || chunk != "he" {
				t.Fatalf("first chunk: %q %v %v", chunk, ok, err)
			}
			_, _, err = stream.Next(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error naming %q, got %v", tc.want, err)
			}
		})
	}
}

// A stream cut off at MAX_TOKENS delivers the text it generated, then ends on
// model.ErrOutputTruncated: a clean end would pass the half-written answer off
// as a complete one.
func TestGeminiStreamCutOffDeliversItsTextThenSaysSo(t *testing.T) {
	client := newGeminiForTest(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `data: {"candidates":[{"content":{"parts":[{"text":"he"}]}}]}`+"\n\n")
		_, _ = io.WriteString(w, `data: {"candidates":[{"content":{"parts":[{"text":"llo"}]},"finishReason":"MAX_TOKENS"}]}`+"\n\n")
	})
	stream, err := client.Stream(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := stream.Close(); err != nil {
			t.Errorf("closing stream: %v", err)
		}
	}()
	for _, want := range []string{"he", "llo"} {
		if chunk, ok, err := stream.Next(context.Background()); err != nil || !ok || chunk != want {
			t.Fatalf("chunk %q: got %q %v %v — the cut-off chunk's own text was generated and billed", want, chunk, ok, err)
		}
	}
	_, ok, err := stream.Next(context.Background())
	if ok || err == nil {
		t.Fatalf("a cut-off stream ended as %v %v, want an error saying it was cut off", ok, err)
	}
	// Stored as Complete stores the same truncation, so one terminal is one
	// value in the trace whichever path served it.
	if got := finishReasonFor("", err); got != model.FinishReasonLength {
		t.Errorf("finish reason = %q, want %q", got, model.FinishReasonLength)
	}
	if errors.Is(err, model.ErrOutputWithheld) || errors.Is(err, model.ErrRequestRejected) {
		t.Errorf("a truncation was classified as an outcome: %v", err)
	}
}

// A final chunk finishing with STOP is the clean terminal — it must not be
// mistaken for an abnormal finish.
func TestGeminiStreamCleanStopIsNotAnError(t *testing.T) {
	client := newGeminiForTest(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `data: {"candidates":[{"content":{"parts":[{"text":"done"}]},"finishReason":"STOP"}]}`+"\n\n")
	})
	stream, err := client.Stream(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := stream.Close(); err != nil {
			t.Errorf("closing stream: %v", err)
		}
	}()
	if chunk, ok, err := stream.Next(context.Background()); err != nil || !ok || chunk != "done" {
		t.Fatalf("STOP chunk must deliver its text: %q %v %v", chunk, ok, err)
	}
	if _, ok, err := stream.Next(context.Background()); ok || err != nil {
		t.Fatalf("stream after STOP must end cleanly: %v %v", ok, err)
	}
}

// Config may carry Google's canonical "models/…" id form; the adapter adds the
// prefix itself, so it must trim a canonical id rather than double it
// (/models/models/… → 404).
func TestGeminiAcceptsCanonicalModelsPrefixedIDs(t *testing.T) {
	var paths []string
	client := newGeminiForTest(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if strings.Contains(r.URL.Path, ":embedContent") {
			_, _ = w.Write([]byte(`{"embedding":{"values":[0.1]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
	})
	if _, err := client.Embed(context.Background(), model.EmbedRequest{Model: "models/gemini-embedding-001", Inputs: []string{"a"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Complete(context.Background(), model.Request{Model: "models/gemini-x", Messages: []model.Message{{Role: "user", Content: "q"}}}); err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		if strings.Contains(p, "/models/models/") {
			t.Fatalf("canonical id double-prefixed: %s", p)
		}
	}
	if paths[0] != "/models/gemini-embedding-001:embedContent" {
		t.Fatalf("embed path wrong: %s", paths[0])
	}
	if paths[1] != "/models/gemini-x:generateContent" {
		t.Fatalf("generate path wrong: %s", paths[1])
	}
}

// A non-stream response is terminal by definition — a candidate with no
// finishReason is a truncated or foreign body, never a complete answer.
func TestGeminiCompleteWithoutTerminalStopIsAnError(t *testing.T) {
	client := newGeminiForTest(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"partial"}]}}]}`))
	})
	_, err := client.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "q"}}})
	if err == nil || !strings.Contains(err.Error(), "STOP") {
		t.Fatalf("missing finishReason must be an error, got %v", err)
	}
}

// A stream that closes before the STOP terminal dropped mid-generation — EOF
// alone must not read as a finished answer.
func TestGeminiStreamEOFWithoutStopIsAnError(t *testing.T) {
	client := newGeminiForTest(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `data: {"candidates":[{"content":{"parts":[{"text":"partial"}]}}]}`+"\n\n")
	})
	stream, err := client.Stream(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := stream.Close(); err != nil {
			t.Errorf("closing stream: %v", err)
		}
	}()
	if chunk, ok, err := stream.Next(context.Background()); err != nil || !ok || chunk != "partial" {
		t.Fatalf("first chunk: %q %v %v", chunk, ok, err)
	}
	if _, _, err := stream.Next(context.Background()); err == nil || errors.Is(err, model.ErrOutputTruncated) {
		t.Fatalf("EOF without STOP must be an error, and not a truncation, got %v", err)
	}
}

// A prompt Gemini refused to read comes back with no candidates at all and the
// reason under promptFeedback; the trace has to name that reason, or a blocked
// prompt is indistinguishable from a body cut short in transit.
func TestGeminiNamesTheReasonAPromptWasBlocked(t *testing.T) {
	wire := finishWires(t)["gemini"]
	client, _ := wire.client(t, `{"promptFeedback":{"blockReason":"PROHIBITED_CONTENT"}}`)
	_, err := client.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "q"}}})
	if !errors.Is(err, model.ErrOutputWithheld) {
		t.Fatalf("err = %v, want model.ErrOutputWithheld", err)
	}
	if got := finishReasonFor("", err); !strings.Contains(got, "PROHIBITED_CONTENT") {
		t.Errorf("finish reason = %q, want it to name PROHIBITED_CONTENT", got)
	}
}
