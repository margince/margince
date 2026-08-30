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

// Every provider ADAPTER marks its own 429 — asserted through the real error
// functions, not through the helper they share. Testing the helper alone let
// three of four adapters drop their wrap with the suite still green.
func TestEveryProviderAdapterMarksAQuotaRefusal(t *testing.T) {
	// The body each vendor returns, so the adapter takes its structured path
	// rather than its "could not parse" fallback.
	adapters := []struct {
		name string
		body string
		read func(*http.Response) error
	}{
		{"gemini", `{"error":{"status":"RESOURCE_EXHAUSTED","message":"cap reached"}}`, geminiError},
		{"openai", `{"error":{"type":"insufficient_quota","message":"cap reached"}}`, openaiError},
		{"anthropic", `{"error":{"type":"rate_limit_error","message":"cap reached"}}`, anthropicError},
		{"openai-compat", `{"error":{"type":"insufficient_quota","message":"cap reached"}}`, openAICompatError},
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

			// An account with nothing left names no moment to come back at.
			refusal := read(http.StatusTooManyRequests, adapter.body, "")
			if !errors.Is(refusal, ErrProviderQuota) {
				t.Fatalf("a 429 naming no retry moment is a quota refusal: %v", refusal)
			}
			if !strings.Contains(SafeVoiceBuildFailure(fmt.Errorf("voice build model call: %w", refusal)), "out of budget") {
				t.Fatal("the build message names the account, not the model")
			}

			// A burst limit says when to return, and is not the same failure.
			throttle := read(http.StatusTooManyRequests, adapter.body, "30")
			if !errors.Is(throttle, ErrProviderThrottled) || errors.Is(throttle, ErrProviderQuota) {
				t.Fatalf("Retry-After marks a throttle, not an empty account: %v", throttle)
			}

			// A body this adapter cannot parse still classifies by status:
			// the fallback path is the one a vendor outage actually takes.
			garbled := read(http.StatusTooManyRequests, "<html>gateway</html>", "")
			if !errors.Is(garbled, ErrProviderQuota) {
				t.Fatalf("an unparseable 429 body is still a refusal: %v", garbled)
			}

			// Any other status is an ordinary provider error. Dressing one up
			// as a billing problem sends an operator to the wrong console.
			other := read(http.StatusInternalServerError, adapter.body, "")
			if errors.Is(other, ErrProviderQuota) || errors.Is(other, ErrProviderThrottled) {
				t.Fatalf("only a 429 is a quota or throttle answer: %v", other)
			}
		})
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
