// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import "github.com/margince/margince/backend/internal/shared/apperrors"

const codeIntervalInvalid = "reporting_interval_invalid"

// The reasons a custom interval is refused, sent as `details.reason` so a
// client words each one in its reader's language instead of parsing the detail.
const (
	intervalMissing         = "interval_missing"
	intervalEndBeforeStart  = "end_before_start"
	intervalLongerThanAYear = "longer_than_a_year"
	intervalAfterCutoff     = "starts_after_cutoff"
)

type intervalError struct{ reason, message string }

func (e intervalError) Error() string { return e.message }
func (e intervalError) Unwrap() error { return apperrors.ErrInvalidArgument }
func (e intervalError) MessageFault() (code, message string) {
	return codeIntervalInvalid, e.message
}

func (e intervalError) FaultReference() map[string]any { return map[string]any{"reason": e.reason} }

// Unreferenced keeps the code and the sentence, which decide the answer.
func (e intervalError) Unreferenced() error { return intervalSentence(e.message) }

type intervalSentence string

func (e intervalSentence) Error() string { return string(e) }
func (e intervalSentence) Unwrap() error { return apperrors.ErrInvalidArgument }
func (e intervalSentence) MessageFault() (code, message string) {
	return codeIntervalInvalid, string(e)
}
