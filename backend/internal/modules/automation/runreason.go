// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"strings"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// The run reason codes: why a firing failed or was blocked, as a closed set a
// client can put into the reader's language. The stored reason is an English
// sentence, so a reader in German would otherwise get English on a German page.
const (
	RunReasonTargetMissing   = "target_missing"
	RunReasonNotPermitted    = "not_permitted"
	RunReasonConflict        = "conflict"
	RunReasonRecordChanged   = "record_changed"
	RunReasonNoConsent       = "no_consent"
	RunReasonBudgetExhausted = "budget_exhausted"
	RunReasonNotCompleted    = "not_completed"
	RunReasonBlocked         = "blocked"
)

// failureReason is one error a firing can fail with. It holds the sentence the
// run row stores for it and the code that sentence is read back as.
type failureReason struct {
	sentinel error
	text     string
	code     string
}

// failureReasons is read both ways: sanitizedReason writes the sentence and
// runReasonCode reads the code back from it.
var failureReasons = []failureReason{
	{apperrors.ErrNotFound, "the target record could not be found", RunReasonTargetMissing},
	{apperrors.ErrPermissionDenied, "the action is no longer permitted", RunReasonNotPermitted},
	{apperrors.ErrConflict, "the action conflicted with the record's current state", RunReasonConflict},
	{apperrors.ErrVersionSkew, "the record changed before this action could be applied", RunReasonRecordChanged},
	{apperrors.ErrConsentNotGranted, "consent has not been granted for this action", RunReasonNoConsent},
	{apperrors.ErrBudgetExceeded, "the workspace's budget for this action has been exhausted", RunReasonBudgetExhausted},
}

const genericFailureText = "the action could not be completed"

// RunReasonCodes lists every code a troubled run can carry, so a client's
// translation table can be checked against it.
func RunReasonCodes() []string {
	codes := make([]string, 0, len(failureReasons)+2)
	for _, r := range failureReasons {
		codes = append(codes, r.code)
	}
	return append(codes, RunReasonNotCompleted, RunReasonBlocked)
}

// runReasonCode reads a troubled run's code from its outcome and stored
// sentence. A failure stored before Apply carries a stage prefix
// ("planning the effect: ..."), so the sentence is matched at its end.
func runReasonCode(outcome string, reason *string) string {
	if outcome == runBlocked {
		return RunReasonBlocked
	}
	if reason != nil {
		for _, r := range failureReasons {
			if strings.HasSuffix(*reason, r.text) {
				return r.code
			}
		}
	}
	return RunReasonNotCompleted
}
