// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The two kinds of reason a correction rests on. A rule counts what the data
// shows: days of silence, the pace of won deals. An inference is Margince's
// reading of what the customer wrote or said.
const (
	ReasonFromRule      = "rule"
	ReasonFromInference = "inference"
)

// CitedActivitiesEvidenceKey names the activities a verdict change rests on,
// inside the change's audit evidence.
const CitedActivitiesEvidenceKey = "cited_activity_ids"

// CorrectionReasonKind says whether a correction's reason is a rule or an
// inference, or "" when the row cannot say. A close as lost is a rule on
// silence and an inference on a refusal. lostCode is the loss reason the
// close wrote, absent when the installation had that reason switched off.
func CorrectionReasonKind(correction string, lostCode *string) string {
	switch correction {
	case string(CloseDateActionAutoApply), string(CloseDateActionProvisionalConfirm), string(CloseDateActionDowngradeAndReview):
		return ReasonFromRule
	case VerdictCloseDateCorrection, VerdictAmountCorrection, VerdictProbabilityCorrection,
		VerdictStageMoveCorrection, DealOpenCorrection, DealReviveCorrection:
		return ReasonFromInference
	case CloseLostCorrection:
		if lostCode == nil {
			return ""
		}
		switch *lostCode {
		case LossReasonWentSilent:
			return ReasonFromRule
		case LossReasonOther:
			return ReasonFromInference
		}
	}
	return ""
}

// citedActivities reads the activities a change's evidence cites. Evidence
// the reader may not read arrives empty, so it cites nothing.
func citedActivities(evidence []byte) []ids.UUID {
	var fields map[string]json.RawMessage
	if len(evidence) == 0 || json.Unmarshal(evidence, &fields) != nil {
		return nil
	}
	var cited []string
	if json.Unmarshal(fields[CitedActivitiesEvidenceKey], &cited) != nil {
		return nil
	}
	out := make([]ids.UUID, 0, len(cited))
	for _, raw := range cited {
		if id, err := ids.Parse(raw); err == nil && !id.IsZero() {
			out = append(out, id)
		}
	}
	return out
}
