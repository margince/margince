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
