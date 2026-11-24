// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func ladder(names ...string) []proposalStageCandidate {
	out := make([]proposalStageCandidate, 0, len(names))
	for i, name := range names {
		out = append(out, proposalStageCandidate{id: ids.New[ids.StageKind]().UUID, position: i + 1, open: true, names: []string{name}})
	}
	return out
}

func TestPickProposalStage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		stages  func() []proposalStageCandidate
		current int
		want    int
		by      string
	}{
		{name: "default pipeline names Proposal", stages: func() []proposalStageCandidate {
			return ladder("Qualified", "Discovery", "Proposal", "Negotiation")
		}, current: 0, want: 2, by: ProposalMatchedByStageName},
		{name: "german name", stages: func() []proposalStageCandidate { return ladder("Neu", "Angebot", "Verhandlung") }, current: 0, want: 1, by: ProposalMatchedByStageName},
		{name: "vietnamese name with accents", stages: func() []proposalStageCandidate { return ladder("Mới", "Báo giá", "Đàm phán") }, current: 0, want: 1, by: ProposalMatchedByStageName},
		{name: "translation only", stages: func() []proposalStageCandidate {
			s := ladder("Start", "Phase 2", "Phase 3")
			s[1].names = append(s[1].names, "Angebot")
			return s
		}, current: 0, want: 1, by: ProposalMatchedByStageName},
		{name: "offer accepted is a later stage", stages: func() []proposalStageCandidate {
			return ladder("Qualified", "Offer accepted", "Proposal sent")
		}, current: 0, want: 2, by: ProposalMatchedByStageName},
		{name: "exit criterion proposal sent targets the next stage", stages: func() []proposalStageCandidate {
			s := ladder("Qualified", "Drafting", "Review", "Closing")
			s[1].criteria = []string{"Proposal sent to the buyer "}
			return s
		}, current: 0, want: 2, by: ProposalMatchedByExitCriterion},
		{name: "german criterion", stages: func() []proposalStageCandidate {
			s := ladder("Neu", "Entwurf", "Prüfung")
			s[1].criteria = []string{"Angebot verschickt"}
			return s
		}, current: 0, want: 2, by: ProposalMatchedByExitCriterion},
		{name: "already at the proposal stage", stages: func() []proposalStageCandidate { return ladder("Qualified", "Proposal", "Negotiation") }, current: 1, want: -1},
		{name: "already past the proposal stage", stages: func() []proposalStageCandidate { return ladder("Qualified", "Proposal", "Negotiation") }, current: 2, want: -1},
		{name: "no stage matches", stages: func() []proposalStageCandidate { return ladder("Qualified", "Pricing", "Commercials") }, current: 0, want: -1},
		{name: "criterion naming a proposal without sent is no match", stages: func() []proposalStageCandidate {
			s := ladder("Qualified", "Drafting", "Review")
			s[1].criteria = []string{"Proposal drafted"}
			return s
		}, current: 0, want: -1},
		{name: "the stage after the criterion is closed", stages: func() []proposalStageCandidate {
			s := ladder("Qualified", "Drafting", "Won")
			s[2].open = false
			s[1].criteria = []string{"Proposal sent"}
			return s
		}, current: 0, want: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stages := tt.stages()
			pick, ok := pickProposalStage(stages, stages[tt.current].id)
			if tt.want < 0 {
				if ok {
					t.Fatalf("picked stage %d, want none", pick.stage.position)
				}
				return
			}
			if !ok || pick.stage.id != stages[tt.want].id || pick.matchedBy != tt.by {
				t.Fatalf("pick = %+v ok=%v, want stage %d by %s", pick, ok, tt.want, tt.by)
			}
		})
	}
}
