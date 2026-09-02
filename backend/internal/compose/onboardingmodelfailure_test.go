// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A model lane that cannot answer is a dependency being down, and the onboarding
// wizard is the first screen anybody ever sees.

import (
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// THE AGGREGATE IS RECOGNISABLE, which is what lets the wizard answer 503 with
// a way through rather than an opaque 500.
//
// Matched by sentinel and not by string: the message carries the task name and
// the last rung's own cause, so any test that read it would be pinned to
// wording that is meant to change.
func TestAModelLaneThatFailedEveryTierIsRecognisable(t *testing.T) {
	t.Parallel()

	served := fmt.Errorf("%w for %s: %w", ai.ErrAllTiersFailed, "cold_start",
		errors.New("openai-compat: response has no choices"))
	if !modelUnreachable(served) {
		t.Error("the every-tier-failed aggregate was not recognised, so the wizard answers 500 and names no remedy")
	}

	// A fault in the REQUEST is not the lane being down, and must not be
	// reported as one: telling somebody the assistant is unavailable when their
	// input was rejected sends them to check a binding that is fine.
	if modelUnreachable(errors.New("history: too many messages")) {
		t.Error("an ordinary error was reported as the model lane being unreachable")
	}
}

// AN ORDINARY PROVIDER ERROR still reaches the degraded answer, and that is
// deliberate rather than an oversight.
//
// The sentinel marks the walk reaching its end, whatever killed the last rung:
// a provider that is down, a credential it refused, a request every rung
// rejected. From the wizard's seat those are one fact — no draft — and the
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
			if !modelUnreachable(fmt.Errorf("%w: %w", ai.ErrAllTiersFailed, cause)) {
				t.Error("the walk ended with no answer and the wizard would still report an opaque 500")
			}
		})
	}
}
