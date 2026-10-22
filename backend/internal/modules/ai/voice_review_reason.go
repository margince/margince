// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import crmcontracts "github.com/margince/margince/backend/internal/contracts"

func (outcome *VoiceBuildOutcome) appendReviewReason(code, text string) {
	outcome.Reasons = append(outcome.Reasons, crmcontracts.VoiceReviewReason{Code: code, Text: text, Values: map[string]float64{}})
}

func (outcome *VoiceBuildOutcome) recordReviewReasons() []string {
	if outcome.Reasons == nil {
		outcome.Reasons = []crmcontracts.VoiceReviewReason{}
	}
	texts := make([]string, len(outcome.Reasons))
	for i, reason := range outcome.Reasons {
		texts[i] = reason.Text
	}
	if outcome.Evaluation == nil {
		outcome.Evaluation = map[string]any{}
	}
	outcome.Evaluation["review_reason_details"] = outcome.Reasons
	return texts
}
