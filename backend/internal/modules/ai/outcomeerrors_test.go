// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// geminiFilterTerminals are Gemini's finishReasons that are a filter's decision
// about the content, per the FinishReason enum of the generateContent API.
var geminiFilterTerminals = []string{
	"BLOCKLIST", "IMAGE_PROHIBITED_CONTENT", "IMAGE_RECITATION", "IMAGE_SAFETY",
	"PROHIBITED_CONTENT", "RECITATION", "SAFETY", "SPII",
}

// Only a filter's terminal withholds. Every other abnormal finish is the reply
// going wrong, which a retry or the next rung may fix, and reading it as
// withheld made it terminal at every extraction site and skipped the
// structured retry.
func TestGeminiWithholdsOnlyAFiltersTerminal(t *testing.T) {
	wire := finishWires(t)["gemini"]
	complete := func(t *testing.T, body string) error {
		t.Helper()
		client, _ := wire.client(t, body)
		_, err := client.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "q"}}})
		return err
	}
	for _, reason := range geminiFilterTerminals {
		t.Run(reason, func(t *testing.T) {
			err := complete(t, `{"candidates":[{"content":{"parts":[]},"finishReason":"`+reason+`"}]}`)
			if !errors.Is(err, model.ErrOutputWithheld) || finishReasonFor("", err) != reason {
				t.Errorf("err = %v, want a withheld answer naming %s", err, reason)
			}
		})
	}
	for _, reason := range []string{
		"OTHER", "LANGUAGE", "MALFORMED_FUNCTION_CALL", "UNEXPECTED_TOOL_CALL", "TOO_MANY_TOOL_CALLS",
		"MISSING_THOUGHT_SIGNATURE", "MALFORMED_RESPONSE", "A_TERMINAL_ADDED_LATER",
	} {
		t.Run(reason, func(t *testing.T) {
			err := complete(t, `{"candidates":[{"content":{"parts":[{"text":"x"}]},"finishReason":"`+reason+`"}]}`)
			if err == nil || !strings.Contains(err.Error(), reason) {
				t.Fatalf("err = %v, want a failed call naming %s", err, reason)
			}
			if ModelDeclined(err) || errors.Is(err, model.ErrRequestRejected) {
				t.Errorf("%s ended the call as an outcome, so nothing retries it: %v", reason, err)
			}
		})
	}
}

// The list is positive, so it is pinned: a terminal joins it only by being
// named here as well.
func TestGeminiWithheldTerminalsArePinned(t *testing.T) {
	if got := slices.Sorted(maps.Keys(geminiWithheldReasons)); !slices.Equal(got, geminiFilterTerminals) {
		t.Errorf("withheld terminals = %v, want %v", got, geminiFilterTerminals)
	}
}

// plantedKey is a credential-shaped string a provider might echo back.
const plantedKey = "sk-ant-api03-FAKEFAKEFAKEfakefakefake0000"

// Every field of a provider's error is text a remote party chose, so each wire
// redacts it before it reaches an error, a log line or a stored call.
func TestNoWireEchoesAKeyFromAProviderError(t *testing.T) {
	for name, fixture := range map[string]refusalFixture{
		"anthropic": {
			provider: providerAnthropic, status: http.StatusBadRequest,
			body: `{"type":"error","error":{"type":"` + plantedKey + `","message":"key ` + plantedKey + ` is not valid"}}`,
		},
		"openai": {
			provider: providerOpenAI, status: http.StatusBadRequest,
			body: `{"error":{"type":"` + plantedKey + `","message":"key ` + plantedKey + ` is not valid","code":null}}`,
		},
		"openai failed response": {
			provider: providerOpenAI, status: http.StatusOK,
			body: `{"id":"r","status":"failed","error":{"code":"` + plantedKey + `","message":"key ` + plantedKey + `"},"output":[]}`,
		},
		"gemini": {
			provider: providerGemini, status: http.StatusBadRequest,
			body: `{"error":{"status":"` + plantedKey + `","message":"key ` + plantedKey + ` is not valid"}}`,
		},
		"gemini in-body error": {
			provider: providerGemini, status: http.StatusOK,
			body: `{"error":{"status":"INTERNAL","message":"key ` + plantedKey + `"}}`,
		},
		"gemini stop": {
			provider: providerGemini, status: http.StatusOK,
			body: `{"candidates":[{"content":{"parts":[]},"finishReason":"` + plantedKey + `"}]}`,
		},
		"openai-compat": {
			provider: providerOpenAICompatible, status: http.StatusBadRequest,
			body: `{"error":{"type":"` + plantedKey + `","message":"key ` + plantedKey + ` is not valid"}}`,
		},
		"ollama": {
			provider: providerOllama, status: http.StatusInternalServerError,
			body: `{"error":"key ` + plantedKey + ` is not valid"}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := refusingAdapter(t, fixture).Complete(context.Background(), model.Request{
				Messages: []model.Message{{Role: "user", Content: "q"}},
			})
			if err == nil {
				t.Fatal("a provider error was accepted as an answer")
			}
			if strings.Contains(err.Error(), plantedKey) {
				t.Errorf("the provider's key-shaped text reached the error: %v", err)
			}
		})
	}
}

// A usage-only message_delta carries stop_reason null, and it must not blank
// the terminal an earlier delta named.
func TestAnthropicStreamKeepsTheTerminalAcrossAUsageOnlyDelta(t *testing.T) {
	body := `data: {"type":"message_start","message":{"model":"claude-test","usage":{"input_tokens":3}}}` + "\n\n" +
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"par"}}` + "\n\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"max_tokens"},"usage":{"output_tokens":5}}` + "\n\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":null},"usage":{"output_tokens":5}}` + "\n\n" +
		`data: {"type":"message_stop"}` + "\n\n"
	client, _ := finishWires(t)["anthropic streamed"].client(t, body)
	resp, err := client.Complete(context.Background(), model.Request{
		MaxTokens: streamedCompleteThreshold + 1, Messages: []model.Message{{Role: "user", Content: "q"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.FinishReason != model.FinishReasonLength {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, model.FinishReasonLength)
	}
}

// A Responses call that failed on invalid_prompt is OpenAI's safety system
// refusing the content: an outcome, not an outage.
func TestOpenAIAPromptThePolicyRefusedIsWithheld(t *testing.T) {
	client, _ := finishWires(t)["openai"].client(t,
		`{"id":"r","model":"gpt-x","status":"failed","error":{"code":"invalid_prompt","message":"Invalid prompt: flagged as potentially violating our usage policy."},"output":[]}`)
	_, err := client.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "q"}}})
	if !errors.Is(err, model.ErrOutputWithheld) || finishReasonFor("", err) != "invalid_prompt" {
		t.Errorf("err = %v, want a withheld answer naming invalid_prompt", err)
	}
}

// The upstream's own terminal is kept only when it is spelled as a code; a
// sentence in that field is not a terminal, and the normalized one is stored.
func TestOpenAICompatKeepsAWithheldTerminalACode(t *testing.T) {
	c := &openAICompatClient{http: &http.Client{}, defaultModel: "m", baseURL: newJSONServer(t,
		`{"model":"m","choices":[{"finish_reason":"content_filter","native_finish_reason":"blocked: `+plantedKey+`","message":{"content":""}}]}`)}
	_, err := c.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}})
	if got := finishReasonFor("", err); got != finishContentFilter {
		t.Errorf("finish reason = %q, want %q: %v", got, finishContentFilter, err)
	}
}

// A withheld reply was still generated and billed, so the tokens it reports
// reach the meter before the walk moves on.
func TestAWithheldRungMetersWhatItSpent(t *testing.T) {
	refusing, _ := finishWires(t)["anthropic"].client(t,
		`{"model":"claude-test","stop_reason":"refusal","content":[],"usage":{"input_tokens":7,"output_tokens":2}}`)
	meter := &memMeter{}
	r := testRouter(map[Tier]model.Client{TierCheapCloud: refusing, TierPremium: NewFakeClient().Script("served")},
		meter, DefaultMonthlyTokens, ProfileCloudHosted)
	if _, _, err := r.Complete(wsContext(t), TaskColdStart, model.Request{Messages: []model.Message{{Role: "user", Content: "q"}}}); err != nil {
		t.Fatal(err)
	}
	for _, rec := range meter.records {
		if rec.Tier == TierCheapCloud {
			if rec.TokensIn != 7 || rec.TokensOut != 2 {
				t.Errorf("the withheld rung metered %+v, want 7 in and 2 out", rec)
			}
			return
		}
	}
	t.Errorf("the withheld rung's spend never reached the meter: %+v", meter.records)
}
