// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// effectFailure is why an approved decision's work did not run: the code a
// client words in the reader's language, and the English the row stores.
// A struct, so a writer cannot pass a bare sentence where a failure belongs.
type effectFailure struct {
	code     string
	sentence string
}

var (
	// It says what to do, because a reader told only "expired" decides again
	// and the row refuses them as already decided.
	failureLapsed = effectFailure{code: "approval_lapsed",
		sentence: "Approval expired before execution. Nothing changed. Request a new proposal to try again."}
	failureNotRun = effectFailure{code: "effect_not_run",
		sentence: "this was approved, but the work it released did not run"}
	failureUnconfirmed = effectFailure{code: "effect_unconfirmed",
		sentence: "this was approved and sent on, but the system it went to never confirmed it, " +
			"so it may have happened. Check the record before you ask for it again"}
	failureFollowUpFailed = effectFailure{code: "follow_up_failed",
		sentence: "this was approved and the change was made, but the automation that asked for it " +
			"could not be marked finished"}
	failureWindowNotWidened = effectFailure{code: "window_not_widened",
		sentence: "the agent's window could not be widened, so the approval has not taken effect"}
)

// effectFailures is every failure a row can store, for reading one back.
var effectFailures = []effectFailure{
	failureLapsed, failureNotRun, failureUnconfirmed, failureFollowUpFailed, failureWindowNotWidened,
}

// EffectFailureCodes is every code a stored failure can carry.
func EffectFailureCodes() []string {
	codes := make([]string, 0, len(effectFailures))
	for _, failure := range effectFailures {
		codes = append(codes, failure.code)
	}
	return codes
}

// effectFailureCode reads a stored sentence back as its code.
func effectFailureCode(sentence *string) *string {
	if sentence == nil {
		return nil
	}
	for _, failure := range effectFailures {
		if failure.sentence == *sentence {
			return &failure.code
		}
	}
	return nil
}
