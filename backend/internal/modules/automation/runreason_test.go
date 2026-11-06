// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestEveryStoredFailureSentenceReadsBackAsItsCode(t *testing.T) {
	t.Parallel()
	for _, r := range failureReasons {
		stored := sanitizedReason(fmt.Errorf("provider: %w", r.sentinel))
		if got := runReasonCode("failed", &stored); got != r.code {
			t.Errorf("%q read back as %q, want %q", stored, got, r.code)
		}
		prefixed := "planning the effect: " + stored
		if got := runReasonCode("failed", &prefixed); got != r.code {
			t.Errorf("a stage-prefixed %q read back as %q, want %q", prefixed, got, r.code)
		}
	}
}

func TestAFailureNoSentenceNamesIsNotCompleted(t *testing.T) {
	t.Parallel()
	stored := sanitizedReason(errors.New("SQLSTATE 23505 on secret_table"))
	if got := runReasonCode("failed", &stored); got != RunReasonNotCompleted {
		t.Errorf("an unrecognised failure read back as %q, want %q", got, RunReasonNotCompleted)
	}
	if got := runReasonCode("failed", nil); got != RunReasonNotCompleted {
		t.Errorf("a failure with no reason read back as %q, want %q", got, RunReasonNotCompleted)
	}
}

func TestABlockedRunIsBlockedWhateverItsSentence(t *testing.T) {
	t.Parallel()
	stored := sanitizedReason(apperrors.ErrNotFound)
	if got := runReasonCode("blocked", &stored); got != RunReasonBlocked {
		t.Errorf("a blocked run read back as %q, want %q", got, RunReasonBlocked)
	}
}
