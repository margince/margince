// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package modelfailure

// A model lane that cannot answer is a dependency being down, and the handler
// waiting on it says so rather than answering an opaque 500.

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// THE AGGREGATE IS RECOGNISABLE, which is what lets the handler answer 503 with
// a way through rather than an opaque 500.
//
// Matched by sentinel and not by string: the message carries the task name and
// the last rung's own cause, so any test that read it would be pinned to
// wording that is meant to change.
func TestAModelLaneThatFailedEveryTierIsRecognisable(t *testing.T) {
	t.Parallel()

	served := fmt.Errorf("%w for %s: %w", ai.ErrAllTiersFailed, "cold_start",
		errors.New("openai-compat: response has no choices"))
	if !unanswered(served) {
		t.Error("the every-tier-failed aggregate was not recognised, so the handler answers 500 and names no remedy")
	}

	// A fault in the REQUEST is not the lane being down, and must not be
	// reported as one: telling somebody the assistant is unavailable when their
	// input was rejected sends them to check a binding that is fine.
	if unanswered(errors.New("history: too many messages")) {
		t.Error("an ordinary error was reported as the model lane being unreachable")
	}
}

// AN ORDINARY PROVIDER ERROR still reaches the degraded answer, and that is
// deliberate rather than an oversight.
//
// The sentinel marks the walk reaching its end, whatever killed the last rung:
// a provider that is down, a credential it refused, a request every rung
// rejected. From the reader's seat those are one fact — no draft — and the
// answer it gives is true of all of them, which is why the message says the
// assistant did not ANSWER and names Settings → AI as a place to look rather
// than as the cause.
//
// The alternative is a 500 for the cases the sentinel does not cover, which is
// the opaque answer this whole change exists to remove.
func TestAnOrdinaryProviderFailureStillDegradesRatherThanFallingThrough(t *testing.T) {
	t.Parallel()

	for name, cause := range map[string]error{
		"a provider that is down":     errors.New("openai-compat: 502 bad gateway"),
		"a credential it refused":     errors.New("openai-compat: 401 invalid_api_key"),
		"a model nobody bound":        errors.New("no bound tier can serve cold_start in profile eu_hosted"),
		"a request every rung reject": errors.New("openai-compat: 400 context_length_exceeded"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if !unanswered(fmt.Errorf("%w: %w", ai.ErrAllTiersFailed, cause)) {
				t.Error("the walk ended with no answer and the handler would still report an opaque 500")
			}
		})
	}
}

// A WALK THAT STOPPED EARLY still left the reader with no draft. A withheld
// answer, a rejected request and an empty account each end the walk without
// the every-tier aggregate, and each is the same fact from the reader's seat:
// the assistant did not answer, and the fields can be typed by hand.
func TestAWalkThatEndedOnAnOutcomeIsAnsweredAsTheAssistantNotAnswering(t *testing.T) {
	const providerText = "I can't help with that PROVIDER-OWN-WORDS"
	for name, tc := range map[string]struct {
		cause     error
		wantLevel string
	}{
		"a withheld answer":         {cause: model.ErrOutputWithheld, wantLevel: ""},
		"a reply that never passed": {cause: ai.ErrOutputRejected, wantLevel: ""},
		"a rejected request":        {cause: model.ErrRequestRejected, wantLevel: "level=ERROR"},
		"an exhausted account":      {cause: ai.ErrProviderQuota, wantLevel: ""},
	} {
		t.Run(name, func(t *testing.T) {
			var logged bytes.Buffer
			restore := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelError})))
			t.Cleanup(func() { slog.SetDefault(restore) })

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/onboarding/company/messages", nil)
			if !answered(rec, req, fmt.Errorf("ai: anthropic: %s: %w", providerText, tc.cause)) {
				t.Fatal("the walk ended with no answer and the handler would still report an opaque 500")
			}
			if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), codeAssistantUnavailable) {
				t.Errorf("want 503 %s, got %d %s", codeAssistantUnavailable, rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "PROVIDER-OWN-WORDS") {
				t.Errorf("the provider's own text reached the client: %s", rec.Body.String())
			}
			if tc.wantLevel == "" {
				if logged.Len() != 0 {
					t.Errorf("an outcome that is not our defect was logged as an error: %q", logged.String())
				}
				return
			}
			// Our own request being refused is a defect nobody sees on the
			// screen, so the operator log is the only place it can be found.
			if entry := logged.String(); !strings.Contains(entry, tc.wantLevel) || !strings.Contains(entry, "PROVIDER-OWN-WORDS") {
				t.Errorf("a rejected request must reach the operator log at %s, log was: %q", tc.wantLevel, entry)
			}
		})
	}
}

// An error that is not the model lane is left for httperr.Write to answer.
func TestAnErrorThatIsNotTheModelLaneIsNotAnsweredAsOne(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	if answered(rec, httptest.NewRequest(http.MethodPost, "/", nil), errors.New("history: too many messages")) {
		t.Errorf("an ordinary error was answered as the assistant being unavailable: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	Write(rec, httptest.NewRequest(http.MethodPost, "/", nil), apperrors.ErrNotFound)
	if rec.Code != http.StatusNotFound {
		t.Errorf("a missing record reached Write and was answered %d rather than httperr's 404", rec.Code)
	}
}
