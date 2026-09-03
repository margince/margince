// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestOpenAICompatSendsBearerWhenKeyedAndOmitsWhenNot(t *testing.T) {
	for _, tc := range []struct {
		name       string
		apiKey     string
		wantHeader string
	}{
		{"keyed cloud", "sk-test", "Bearer sk-test"},
		{"unkeyed local", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gotAuth string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
			}))
			defer srv.Close()
			c := &openAICompatClient{http: &http.Client{}, baseURL: srv.URL, apiKey: tc.apiKey, defaultModel: "m"}
			if _, err := c.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}}); err != nil {
				t.Fatal(err)
			}
			if gotAuth != tc.wantHeader {
				t.Fatalf("Authorization = %q, want %q", gotAuth, tc.wantHeader)
			}
		})
	}
}

func TestOpenAICompatSurfacesHTTPErrorWithoutEchoingRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream boom"))
	}))
	defer srv.Close()
	c := &openAICompatClient{http: &http.Client{}, baseURL: srv.URL, apiKey: "k", defaultModel: "m"}
	_, err := c.Complete(context.Background(), model.Request{
		Messages:       []model.Message{{Role: "user", Content: "with password=verysecretpw inside"}},
		SecretStripper: NewSecretStripper(),
	})
	if err == nil || !strings.Contains(err.Error(), "http 500") {
		t.Fatalf("want http 500 surfaced, got %v", err)
	}
	if strings.Contains(err.Error(), "verysecretpw") {
		t.Fatalf("error must not echo the request: %v", err)
	}
}

// The generic OpenAI-compatible wire's "model" field is merely echoed back
// from the request by the server, never independently confirmed — the
// adapter still decodes it into ServedModel (the router tags it "echo" to
// keep that distinction honest, rather than treating it as "response").
func TestOpenAICompatCompleteDecodesEchoedModelField(t *testing.T) {
	c := &openAICompatClient{
		http: &http.Client{}, defaultModel: "m",
		baseURL: newJSONServer(t, `{"model":"mistral-echoed","choices":[{"message":{"content":"ok"}}]}`),
	}
	resp, err := c.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ServedModel != "mistral-echoed" {
		t.Fatalf("ServedModel not decoded from the echoed model field: %q", resp.ServedModel)
	}
}

func TestOpenAICompatEmptyChoicesIsAnError(t *testing.T) {
	c := &openAICompatClient{http: &http.Client{}, defaultModel: "m", baseURL: newJSONServer(t, `{"choices":[]}`)}
	if _, err := c.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}}); err == nil || !strings.Contains(err.Error(), "no choices") {
		t.Fatalf("want a no-choices error, got %v", err)
	}
}

func newJSONServer(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// vLLM emits its error at the TOP level ({"object":"error",type,message}), not
// under OpenAI's nested {"error":{…}} — the operator must still see the
// message, not a bare "http 400".
func TestOpenAICompatErrorDecodesVLLMTopLevelShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"object":"error","type":"BadRequestError","message":"dimensions is not supported"}`))
	}))
	defer srv.Close()
	client := &openAICompatClient{http: &http.Client{}, baseURL: srv.URL, localOnly: true, defaultModel: "m"}
	_, err := client.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "q"}}})
	if err == nil || !strings.Contains(err.Error(), "dimensions is not supported") || !strings.Contains(err.Error(), "BadRequestError") {
		t.Fatalf("want vLLM's top-level type+message, got %v", err)
	}
}

// The generic wire gives no way to know whether the server honors OpenAI's
// `dimensions` matryoshka knob, and vLLM 400s on models that aren't MRL-trained
// — the adapter must not put it on the wire even when the caller pins a width.
func TestOpenAICompatEmbedOmitsDimensions(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body = readBody(t, r.Body)
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]}]}`))
	}))
	defer srv.Close()
	client := &openAICompatClient{http: &http.Client{}, baseURL: srv.URL, localOnly: true, defaultModel: "bge-m3"}
	if _, err := client.Embed(context.Background(), model.EmbedRequest{Inputs: []string{"a"}, Dimensions: 1024}); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte(`"dimensions"`)) {
		t.Fatalf("dimensions must not reach an openai-compatible/vllm server: %s", body)
	}
}

// A broker names the upstream that served independently of the echoed model
// field, and the whole point of reading it is attribution: one model id is
// served by hosts differing in quantization, output ceiling and tail latency,
// so a call that cannot name its host cannot be compared with another.
func TestOpenAICompatCompleteDecodesTheServedUpstream(t *testing.T) {
	c := &openAICompatClient{
		http: &http.Client{}, defaultModel: "m",
		baseURL: newJSONServer(t, `{"model":"openai/gpt-oss-120b","provider":"BaseTen",
			"choices":[{"finish_reason":"stop","message":{"content":"ok"}}],
			"usage":{"prompt_tokens":9,"completion_tokens":4,
			  "completion_tokens_details":{"reasoning_tokens":3}}}`),
	}
	resp, err := c.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ServedProvider != "BaseTen" {
		t.Errorf("ServedProvider = %q, want BaseTen", resp.ServedProvider)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want stop", resp.FinishReason)
	}
	// Itemized inside OutputTokens, never additive to it (the port's contract).
	if resp.ReasoningTokens != 3 {
		t.Errorf("ReasoningTokens = %d, want 3", resp.ReasoningTokens)
	}
	if resp.OutputTokens != 4 {
		t.Errorf("OutputTokens = %d, want 4", resp.OutputTokens)
	}
}

// A single-vendor host on this wire names no upstream, and an absent report
// must stay absent rather than being filled in with the vendor we called: the
// field's whole value is that it says who actually served.
func TestOpenAICompatCompleteLeavesTheUpstreamEmptyWhenNoneIsReported(t *testing.T) {
	c := &openAICompatClient{
		http: &http.Client{}, defaultModel: "m",
		baseURL: newJSONServer(t, `{"model":"m","choices":[{"message":{"content":"ok"}}]}`),
	}
	resp, err := c.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ServedProvider != "" {
		t.Errorf("ServedProvider = %q, want empty", resp.ServedProvider)
	}
}

// The silent-empty failure this fallback exists for: a reasoning model spends
// its whole output budget thinking, the answer never starts, and the wire
// returns content:null with every generated token under `reasoning`. Reading
// content alone hands the caller an empty string for a call billed in full,
// with no error to retry on.
func TestOpenAICompatCompleteFallsBackToReasoningWhenTheAnswerNeverStarted(t *testing.T) {
	c := &openAICompatClient{
		http: &http.Client{}, defaultModel: "m",
		baseURL: newJSONServer(t, `{"model":"m","provider":"Cerebras",
			"choices":[{"finish_reason":"length",
			  "message":{"content":null,"reasoning":"the user asks for a reply, so I should"}}],
			"usage":{"prompt_tokens":685,"completion_tokens":300}}`),
	}
	resp, err := c.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text == "" {
		t.Fatal("Text is empty: 300 tokens were generated and billed, and the caller was told nothing")
	}
	if !strings.Contains(resp.Text, "the user asks for a reply") {
		t.Errorf("Text = %q, want the reasoning text", resp.Text)
	}
	// The reason the text is thinking rather than an answer has to travel with
	// it, or a caller's schema failure blames the model for the output budget.
	if resp.FinishReason != "length" {
		t.Errorf("FinishReason = %q, want length", resp.FinishReason)
	}
}

// Content wins whenever there is any: the fallback is for an answer that never
// started, not a way for thinking to overwrite one that did.
func TestOpenAICompatCompletePrefersTheAnswerOverTheThinking(t *testing.T) {
	c := &openAICompatClient{
		http: &http.Client{}, defaultModel: "m",
		baseURL: newJSONServer(t, `{"model":"m","choices":[{"finish_reason":"stop","message":
			{"content":"the answer","reasoning":"first I should think"}}]}`),
	}
	resp, err := c.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "the answer" {
		t.Errorf("Text = %q, want the answer", resp.Text)
	}
}

// A broker's upstreams disagree with themselves about reasoning tokens, and an
// unbounded excess does not just misreport — it disables aicert's max_tokens
// cap, which grades an answer as output minus reasoning and can never exceed a
// ceiling once that difference goes negative.
func TestOpenAICompatBoundsReasoningTokensByTheCompletionTheyBreakDown(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		completion, reasoning int
		want                  int
	}{
		// Measured against gpt-oss-120b: DeepInfra 817 reasoning on 787 completion.
		{"upstream reports more reasoning than completion", 787, 817, 787},
		{"an ordinary subset is untouched", 1037, 952, 952},
		{"equal is a subset too", 40, 40, 40},
		// BaseTen sends no count on a response that plainly reasoned. Nothing can
		// be recovered from a number that was never sent.
		{"an unreported count stays unreported", 500, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := reasoningWithin(tc.completion, tc.reasoning); got != tc.want {
				t.Errorf("reasoningWithin(%d, %d) = %d, want %d", tc.completion, tc.reasoning, got, tc.want)
			}
		})
	}
}

// The whole point of the bound, asserted through the adapter rather than the
// helper: a caller subtracting reasoning from output must never get a negative.
func TestOpenAICompatNeverReportsMoreReasoningThanOutput(t *testing.T) {
	c := &openAICompatClient{
		http: &http.Client{}, defaultModel: "m",
		baseURL: newJSONServer(t, `{"model":"m","provider":"DeepInfra",
			"choices":[{"finish_reason":"stop","message":{"content":"{}"}}],
			"usage":{"prompt_tokens":1851,"completion_tokens":787,
			  "completion_tokens_details":{"reasoning_tokens":817}}}`),
	}
	resp, err := c.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if answer := resp.OutputTokens - resp.ReasoningTokens; answer < 0 {
		t.Errorf("output %d minus reasoning %d = %d; a caller grading the answer alone reads a negative budget as always under cap",
			resp.OutputTokens, resp.ReasoningTokens, answer)
	}
}

// Every remote string on the error path is redacted and bounded, the `type`
// discriminator included.
//
// The type is the one that looks safe and is not: it reads like a short enum an
// API author picked, and on a broker it is a string an upstream chose, so it can
// be long or carry the request back. A secret in it would otherwise reach this
// installation's logs through the one field nothing checked.
func TestOpenAICompatErrorRedactsTheTypeAsWellAsTheMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"type":"invalid_request password=typesecretpw","message":"refused: password=messagesecretpw"}`))
	}))
	defer srv.Close()
	c := &openAICompatClient{http: &http.Client{}, baseURL: srv.URL, apiKey: "k", defaultModel: "m"}
	_, err := c.Complete(context.Background(), model.Request{
		Messages: []model.Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("a 400 must surface as an error")
	}
	// Both fields carry a shape the stripper recognises. It is not a general
	// secret detector — a bare vendor token it has no pattern for would pass —
	// so what is asserted here is that BOTH remote fields are routed through it,
	// which is the part this path is responsible for.
	for _, secret := range []string{"typesecretpw", "messagesecretpw"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("the error carries %q from a remote field: %v", secret, err)
		}
	}
}
