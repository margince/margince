// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// stubVoiceBrain answers the one builder call with a canned inference.
type stubVoiceBrain struct {
	inference VoiceInference
	err       error
	prompt    string
}

func (s *stubVoiceBrain) Complete(_ context.Context, req model.Request) (model.Response, error) {
	if s.err != nil {
		return model.Response{}, s.err
	}
	s.prompt = req.Messages[0].Content
	payload, err := json.Marshal(s.inference)
	if err != nil {
		return model.Response{}, err
	}
	return model.Response{Text: string(payload), ServedModel: "stub-model-1"}, nil
}

func builderSamples() []VoiceSample {
	words := strings.Repeat("plain honest sentence about work. ", 200)
	return []VoiceSample{
		{ID: "s-email", Kind: "email", Register: "email", Text: "We ship on Monday, no excuses. " + words, WordCount: WordCount(words) + 6},
		{ID: "s-spoken", Kind: "transcript", Register: "spoken", Text: "Look, it either works or it does not. " + words, WordCount: WordCount(words) + 9},
	}
}

func validInference() VoiceInference {
	return VoiceInference{
		IdentitySummary:    "Direct, operational, allergic to filler.",
		ThinkingPattern:    "Notice the anomaly, state the verdict, then justify it operationally.",
		ObservedObsessions: []string{"second-order effects"},
		Directness:         "Very high; verdict first.",
		Structure:          "Short paragraphs, one point each.",
		Openings:           []string{"straight into the subject"},
		Closings:           []string{"a concrete next step"},
		Vocabulary:         []string{"ship", "honest"},
		Avoid:              []string{"corporate filler"},
		SignatureMoves: []VoiceSignatureMove{{
			Move: "verdict before argument", Quote: "We ship on Monday, no excuses.", SampleID: "s-email",
		}},
		RegisterNotes: []string{"spoken register is blunter than email"},
		Evidence:      []string{"s-email", "s-spoken"},
	}
}

func TestDeriveVoiceBuildsAValidatedArtifact(t *testing.T) {
	brain := &stubVoiceBrain{inference: validInference()}
	artifact, err := DeriveVoice(context.Background(), brain, "Prefers German directness.", "hash-1", builderSamples())
	if err != nil {
		t.Fatal(err)
	}
	if artifact.ModelName != "stub-model-1" || artifact.SourceHash != "hash-1" {
		t.Fatalf("artifact identity = %q / %q", artifact.ModelName, artifact.SourceHash)
	}
	if len(artifact.Exemplars) != 2 {
		t.Fatalf("exemplars = %d, want 2", len(artifact.Exemplars))
	}
	for _, section := range []string{"## Identity", "## How you think", "## Signature moves", "## Universal anti-AI rules", "## Style metrics"} {
		if !strings.Contains(artifact.Markdown, section) {
			t.Fatalf("markdown misses section %q:\n%s", section, artifact.Markdown)
		}
	}
	if !strings.Contains(artifact.Markdown, "We ship on Monday, no excuses.") {
		t.Fatal("the signature move's verbatim quote must appear in the artifact")
	}
	if !strings.Contains(brain.prompt, "Human-authored preferences") || !strings.Contains(brain.prompt, "German directness") {
		t.Fatal("the personality document must reach the builder prompt with priority framing")
	}
}

// validatedStubBrain also satisfies the optional CompleteValidated seam. It
// exercises the validator the way the router does: the first candidate is
// malformed, the second fabricates a quote, the third is the real answer —
// so the test proves the builder both selects the validated pipeline and
// hands it a validator that actually rejects bad output.
type validatedStubBrain struct {
	stubVoiceBrain
	completeCalls  int
	validatedCalls int
	rejections     []error
}

func (s *validatedStubBrain) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	s.completeCalls++
	return s.stubVoiceBrain.Complete(ctx, req)
}

func (s *validatedStubBrain) CompleteValidated(ctx context.Context, req model.Request, validate Validator) (model.Response, error) {
	s.validatedCalls++
	if err := validate("not json at all"); err != nil {
		s.rejections = append(s.rejections, err)
	}
	fabricated := s.inference
	fabricated.SignatureMoves = []VoiceSignatureMove{{
		Move: "invented", Quote: "words the author never wrote", SampleID: "s-email",
	}}
	payload, err := json.Marshal(fabricated)
	if err != nil {
		return model.Response{}, err
	}
	if err := validate(string(payload)); err != nil {
		s.rejections = append(s.rejections, err)
	}
	// The embedded stub is called directly: it is this fake's answer supply,
	// not a routed Complete attempt, so completeCalls stays untouched.
	resp, err := s.stubVoiceBrain.Complete(ctx, req)
	if err != nil {
		return model.Response{}, err
	}
	if err := validate(resp.Text); err != nil {
		return model.Response{}, err
	}
	return resp, nil
}

func TestDeriveVoicePrefersTheValidatedPipeline(t *testing.T) {
	brain := &validatedStubBrain{stubVoiceBrain: stubVoiceBrain{inference: validInference()}}
	artifact, err := DeriveVoice(context.Background(), brain, "", "hash-1", builderSamples())
	if err != nil {
		t.Fatal(err)
	}
	if brain.validatedCalls != 1 || brain.completeCalls != 0 {
		t.Fatalf("validated=%d complete=%d — a validated brain must be driven only through CompleteValidated",
			brain.validatedCalls, brain.completeCalls)
	}
	if len(brain.rejections) != 2 {
		t.Fatalf("rejections = %d, want 2 — the supplied validator must refuse malformed JSON and a fabricated quote", len(brain.rejections))
	}
	if artifact.ModelName != "stub-model-1" {
		t.Fatalf("artifact model = %q", artifact.ModelName)
	}
}

func TestDeriveVoiceRejectsFabricatedEvidence(t *testing.T) {
	cases := map[string]func(*VoiceInference){
		"unknown move sample":    func(v *VoiceInference) { v.SignatureMoves[0].SampleID = "s-invented" },
		"non-verbatim quote":     func(v *VoiceInference) { v.SignatureMoves[0].Quote = "words the author never wrote" },
		"empty thinking pattern": func(v *VoiceInference) { v.ThinkingPattern = " " },
	}
	for name, corrupt := range cases {
		t.Run(name, func(t *testing.T) {
			inference := validInference()
			corrupt(&inference)
			_, err := DeriveVoice(context.Background(), &stubVoiceBrain{inference: inference}, "", "h", builderSamples())
			if err == nil {
				t.Fatal("a corrupted inference must be rejected, never persisted")
			}
		})
	}
}

func TestDeriveVoiceEnforcesTheWordFloorAndModelPath(t *testing.T) {
	if _, err := DeriveVoice(context.Background(), &stubVoiceBrain{inference: validInference()}, "", "h",
		[]VoiceSample{{ID: "tiny", Kind: "other", Register: "general", Text: "too short", WordCount: 2}}); err == nil {
		t.Fatal("a sub-floor corpus must refuse to build")
	}
	if _, err := DeriveVoice(context.Background(), nil, "", "h", builderSamples()); err == nil {
		t.Fatal("a nil brain must be an explicit configuration error")
	}
}

func TestContainsNormalizedFoldsWhitespaceOnly(t *testing.T) {
	if !containsNormalized("line one\n  line two", "one line two") {
		t.Fatal("whitespace folding must let a wrapped quote match")
	}
	if containsNormalized("some text", "some other text") {
		t.Fatal("invented words must never match")
	}
	if containsNormalized("anything", "  ") {
		t.Fatal("an empty quote is not evidence")
	}
}

func TestFabricatedEvidenceCitationsAreDroppedNotFatal(t *testing.T) {
	inference := validInference()
	inference.Evidence = []string{"s-email", "a prose sentence pretending to be a citation"}
	brain := &stubVoiceBrain{inference: inference}
	artifact, err := DeriveVoice(context.Background(), brain, "", "h", builderSamples())
	if err != nil {
		t.Fatalf("a fabricated supplementary citation must not fail the build: %v", err)
	}
	if len(artifact.Inference.Evidence) != 1 || artifact.Inference.Evidence[0] != "s-email" {
		t.Fatalf("evidence = %v, want only the real sample id kept", artifact.Inference.Evidence)
	}
}

func TestVoicePromptNamesTheValidSampleIDs(t *testing.T) {
	brain := &stubVoiceBrain{inference: validInference()}
	if _, err := DeriveVoice(context.Background(), brain, "", "h", builderSamples()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(brain.prompt, "Valid sample ids: s-email, s-spoken") {
		t.Fatal("the prompt must enumerate the exact ids the model may cite")
	}
}

// The message a failed build shows its operator is the only thing they get, so
// each cause has to be distinguishable in it. A provider that refused on
// billing never ran the model, and saying otherwise sent an operator to audit
// writing samples over a spending cap.
func TestSafeVoiceBuildFailureNamesTheCause(t *testing.T) {
	quota := fmt.Errorf("voice build model call: %w: ai: gemini: RESOURCE_EXHAUSTED: spending cap (http 429)", ErrProviderQuota)
	message := SafeVoiceBuildFailure(quota)
	if !strings.Contains(strings.ToLower(message), "budget") {
		t.Fatalf("a provider out of budget says so: %q", message)
	}
	if strings.Contains(strings.ToLower(message), "could not produce") {
		t.Fatalf("the model never ran, so nothing may claim it answered badly: %q", message)
	}

	// A model that DID answer, unreadably, keeps its own separate wording —
	// otherwise one message covers two causes again.
	unreadable := SafeVoiceBuildFailure(errors.New("voice build model call: ai: output rejected by the validator"))
	if unreadable == message {
		t.Fatal("a bad model answer and a billing refusal are different failures and read differently")
	}

	// Nothing leaks the provider's own payload to the operator.
	for _, leak := range []string{"RESOURCE_EXHAUSTED", "http 429", "gemini"} {
		if strings.Contains(message, leak) {
			t.Fatalf("message leaks provider internals (%q): %q", leak, message)
		}
	}
}

// Every provider ADAPTER classifies its own 429, asserted through the real
// error functions rather than the helper they share — testing the helper alone
// let three of four adapters drop their wrap with the suite still green.
//
// The bodies are the ones these vendors actually return.
func TestEveryProviderAdapterClassifiesItsRefusal(t *testing.T) {
	adapters := []struct {
		name     string
		quota    string
		throttle string
		read     func(*http.Response) error
	}{
		{
			name:     "gemini",
			quota:    `{"error":{"status":"RESOURCE_EXHAUSTED","message":"Your project has exceeded its monthly spending cap."}}`,
			throttle: `{"error":{"status":"RESOURCE_EXHAUSTED","message":"Rate limit exceeded for this model."}}`,
			read:     geminiError,
		},
		{
			name:     "openai",
			quota:    `{"error":{"type":"insufficient_quota","message":"You exceeded your current quota, please check your plan and billing details."}}`,
			throttle: `{"error":{"type":"rate_limit_exceeded","message":"Rate limit reached for gpt-4o."}}`,
			read:     openaiError,
		},
		{
			name:     "anthropic",
			quota:    `{"error":{"type":"invalid_request_error","message":"Your credit balance is too low to access the API."}}`,
			throttle: `{"error":{"type":"rate_limit_error","message":"Number of requests has exceeded your rate limit."}}`,
			read:     anthropicError,
		},
		{
			// The broker shape: its OWN message says nothing, and the vendor's
			// sentence is in metadata.raw. This is the live OpenRouter answer.
			name:     "openai-compat",
			quota:    `{"error":{"message":"Provider returned error","metadata":{"provider_name":"Mistral","raw":"Insufficient credits to run this request.","limit_source":"account_credits"}}}`,
			throttle: `{"error":{"message":"Provider returned error","metadata":{"provider_name":"Mistral","raw":"mistralai/mistral-large-2512 is temporarily rate-limited upstream. Please retry shortly","limit_source":"upstream_provider_shared_pool"}}}`,
			read:     openAICompatError,
		},
	}
	for _, adapter := range adapters {
		t.Run(adapter.name, func(t *testing.T) {
			read := func(status int, body, retryAfter string) error {
				resp := refusalResponse(status, body, retryAfter)
				defer func() {
					if closeErr := resp.Body.Close(); closeErr != nil {
						t.Errorf("closing the provider response: %v", closeErr)
					}
				}()
				return adapter.read(resp)
			}

			quota := read(http.StatusTooManyRequests, adapter.quota, "")
			if !errors.Is(quota, ErrProviderQuota) || errors.Is(quota, ErrProviderThrottled) {
				t.Fatalf("an account with nothing left is a quota refusal: %v", quota)
			}
			if !strings.Contains(SafeVoiceBuildFailure(fmt.Errorf("voice build model call: %w", quota)), "out of budget") {
				t.Fatal("the build message names the account, not the model")
			}

			// A burst limit is the opposite answer even though the status is
			// the same, and it must never send the reader to their billing.
			throttle := read(http.StatusTooManyRequests, adapter.throttle, "")
			if !errors.Is(throttle, ErrProviderThrottled) || errors.Is(throttle, ErrProviderQuota) {
				t.Fatalf("a busy model is a throttle, not an empty account: %v", throttle)
			}
			if strings.Contains(SafeVoiceBuildFailure(fmt.Errorf("voice build model call: %w", throttle)), "out of budget") {
				t.Fatal("a throttle must not send the reader to a billing console")
			}

			// A body this adapter cannot read still says the model was never
			// reached, without inventing which limit was hit.
			garbled := read(http.StatusTooManyRequests, "<html>gateway</html>", "")
			if errors.Is(garbled, ErrProviderQuota) || errors.Is(garbled, ErrProviderThrottled) {
				t.Fatalf("an unreadable body names no cause: %v", garbled)
			}
			if message := SafeVoiceBuildFailure(fmt.Errorf("voice build model call: %w", garbled)); !strings.Contains(message, "turned the call away") {
				t.Fatalf("an unclassified refusal still says the model never ran: %q", message)
			}

			// Any other status is an ordinary provider error.
			other := read(http.StatusInternalServerError, adapter.quota, "")
			if errors.Is(other, ErrProviderQuota) || errors.Is(other, ErrProviderThrottled) {
				t.Fatalf("only a 429 is a quota or throttle answer: %v", other)
			}
		})
	}
}

// The broker's own message says nothing about the cause; the upstream vendor's
// sentence and name are what a reader needs, so both reach the log line.
func TestABrokerRefusalQuotesTheUpstreamVendor(t *testing.T) {
	body := `{"error":{"message":"Provider returned error","metadata":{"provider_name":"Mistral","raw":"temporarily rate-limited upstream","limit_source":"upstream_provider_shared_pool"}}}`
	resp := refusalResponse(http.StatusTooManyRequests, body, "")
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Errorf("closing the provider response: %v", closeErr)
		}
	}()

	err := openAICompatError(resp)
	for _, want := range []string{"Mistral", "temporarily rate-limited upstream"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the logged failure carries %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "Provider returned error") {
		t.Fatalf("the broker's own placeholder is not the cause: %v", err)
	}
}

// One provider response, as the adapters read it.
func refusalResponse(status int, body, retryAfter string) *http.Response {
	header := http.Header{"Content-Type": []string{"application/json"}}
	if retryAfter != "" {
		header.Set("Retry-After", retryAfter)
	}
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// Gemini spends RESOURCE_EXHAUSTED and the word "quota" on BOTH an exhausted
// spending cap and an ordinary per-minute limit, so the words cannot separate
// them. Only the retryable one carries a RetryInfo detail, and reading the
// words alone is what told an operator with unspent credit to raise a limit.
func TestGeminiQuotaWordingDoesNotHideARetryableLimit(t *testing.T) {
	retryable := `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"You exceeded your current quota. Quota exceeded for metric generate_requests_per_minute.","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"45s"}]}}`
	exhausted := `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"Your project has exceeded its monthly spending cap. Please go to AI Studio to manage your project spend cap."}}`

	read := func(body string) error {
		resp := refusalResponse(http.StatusTooManyRequests, body, "")
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				t.Errorf("closing the provider response: %v", closeErr)
			}
		}()
		return geminiError(resp)
	}

	perMinute := read(retryable)
	if !errors.Is(perMinute, ErrProviderThrottled) || errors.Is(perMinute, ErrProviderQuota) {
		t.Fatalf("a limit the vendor offers a retry delay for is a throttle: %v", perMinute)
	}
	if strings.Contains(SafeVoiceBuildFailure(fmt.Errorf("voice build model call: %w", perMinute)), "out of budget") {
		t.Fatal("a per-minute limit must not send the reader to a billing console")
	}

	spendCap := read(exhausted)
	if !errors.Is(spendCap, ErrProviderQuota) || errors.Is(spendCap, ErrProviderThrottled) {
		t.Fatalf("a spending cap with no retry offered is an account refusal: %v", spendCap)
	}
}

// A vendor's sentence is text a REMOTE party chose. It reaches this
// installation's logs, so it is redacted and bounded on the way — nothing else
// guards that path.
func TestABrokerSentenceIsRedactedAndBoundedBeforeItIsLogged(t *testing.T) {
	leaky := `{"error":{"message":"Provider returned error","metadata":{"provider_name":"Mistral","raw":"upstream rejected key sk-or-v1-abcdefghijklmnopqrstuvwxyz012345 while calling the model"}}}`
	resp := refusalResponse(http.StatusTooManyRequests, leaky, "")
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Errorf("closing the provider response: %v", closeErr)
		}
	}()

	err := openAICompatError(resp)
	if strings.Contains(err.Error(), "sk-or-v1-abcdefghijklmnopqrstuvwxyz012345") {
		t.Fatalf("a credential a vendor echoed back must not reach the log: %v", err)
	}
	if !strings.Contains(err.Error(), "Mistral") {
		t.Fatalf("the vendor is still named, so the cause stays readable: %v", err)
	}

	// A body that lost its way into a message field is truncated rather than
	// written whole.
	long := `{"error":{"message":"Provider returned error","metadata":{"provider_name":"X","raw":"` + strings.Repeat("verbose ", 200) + `"}}}`
	longResp := refusalResponse(http.StatusTooManyRequests, long, "")
	defer func() {
		if closeErr := longResp.Body.Close(); closeErr != nil {
			t.Errorf("closing the provider response: %v", closeErr)
		}
	}()
	if got := len(openAICompatError(longResp).Error()); got > providerTextMax+120 {
		t.Fatalf("one logged vendor sentence is bounded; got %d characters", got)
	}
}
