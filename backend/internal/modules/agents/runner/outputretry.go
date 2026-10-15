// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// outputRetry is the loop's memory of a model that has not yet produced a step.
// An unparseable reply is one of two things. Either the model broke the format,
// which asking again can fix. Or the provider cut it off at the output cap,
// which asking again at the same cap cannot.
type outputRetry struct {
	invalidStreak int
	ceiling       int
}

func newOutputRetry() *outputRetry {
	return &outputRetry{ceiling: perCallOutputCeiling}
}

// accepted resets the state: the cap is raised for one step, not for the run.
func (o *outputRetry) accepted() {
	o.invalidStreak = 0
	o.ceiling = perCallOutputCeiling
}

// refuse records one unusable reply, tells the model what was wrong with it,
// and says whether the run must degrade, and why.
func (o *outputRetry) refuse(win *window, resp model.Response, parseErr error) (reason string, giveUp bool) {
	if resp.FinishReason == model.FinishReasonLength {
		if o.ceiling >= raisedOutputCeiling {
			return reasonStepTooLong, true
		}
		o.ceiling = raisedOutputCeiling
		win.observeThen(outputValidatorSource,
			fmt.Sprintf("your previous step was cut off at %d output tokens", resp.OutputTokens),
			"Return a shorter step.")
		return "", false
	}
	o.invalidStreak++
	if o.invalidStreak >= consecutiveInvalidLimit {
		return invalidOutputReason(o.invalidStreak), true
	}
	win.observeThen(outputValidatorSource, "your previous output failed validation: "+parseErr.Error(), "Return ONLY the step JSON.")
	return "", false
}
