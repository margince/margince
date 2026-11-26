// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEveryCorrectionSaysWhetherItsReasonIsARuleOrAnInference(t *testing.T) {
	t.Parallel()
	silent, refused, retired := LossReasonWentSilent, LossReasonOther, "budget"
	for _, tc := range []struct {
		correction string
		lostCode   *string
		want       string
	}{
		{string(CloseDateActionAutoApply), nil, ReasonFromRule},
		{string(CloseDateActionProvisionalConfirm), nil, ReasonFromRule},
		{string(CloseDateActionDowngradeAndReview), nil, ReasonFromRule},
		{CloseLostCorrection, &silent, ReasonFromRule},
		{CloseLostCorrection, &refused, ReasonFromInference},
		{CloseLostCorrection, &retired, ""},
		{CloseLostCorrection, nil, ""},
		{VerdictCloseDateCorrection, nil, ReasonFromInference},
		{VerdictAmountCorrection, nil, ReasonFromInference},
		{VerdictProbabilityCorrection, nil, ReasonFromInference},
		{VerdictStageMoveCorrection, nil, ReasonFromInference},
		{DealOpenCorrection, nil, ReasonFromInference},
		{DealReviveCorrection, nil, ReasonFromInference},
		{"a_kind_this_build_does_not_know", nil, ""},
	} {
		if got := CorrectionReasonKind(tc.correction, tc.lostCode); got != tc.want {
			t.Errorf("CorrectionReasonKind(%q, %v) = %q, want %q", tc.correction, tc.lostCode, got, tc.want)
		}
	}
}

func TestACorrectionCitesOnlyTheActivitiesItsEvidenceNames(t *testing.T) {
	t.Parallel()
	first, second := ids.NewV7(), ids.NewV7()
	evidence := []byte(`{"basis":"They will sign then","cited_activity_ids":["` + first.String() + `","not-an-id","` + second.String() + `"]}`)
	if got := citedActivities(evidence); !slices.Equal(got, []ids.UUID{first, second}) {
		t.Errorf("cited = %v, want the two ids the evidence names", got)
	}
	for _, withheld := range [][]byte{nil, []byte(`{}`), []byte(`not json`)} {
		if got := citedActivities(withheld); len(got) != 0 {
			t.Errorf("evidence %q cites %v, want nothing", withheld, got)
		}
	}
}
