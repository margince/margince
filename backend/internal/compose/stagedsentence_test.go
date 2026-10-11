// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The approver reads one sentence per operation, whichever door staged it.
func TestStagedSentenceIsTheResolversWhenItNamesTheRecords(t *testing.T) {
	pol := agentPolicy{Op: "mergeTags", Tool: "merge_tags"}
	info := agents.StageInfo{Summary: `Fold tag "Conf" into "Conference 2026"`, TargetID: ids.NewV7()}

	got := stagedSentence(info, inEnglish, pol, summaryRequest("POST", "/v1/tags/x/merge"), []byte(`{"into_tag_id":"x"}`))

	if got != info.Summary {
		t.Errorf("staged %q, want the resolver's %q", got, info.Summary)
	}
}

// A resolver line that says less than this door's own is not the one staged.
// This door's line names the act and the values in the body.
func TestStagedSentenceFallsBackWhenTheResolverSaysLess(t *testing.T) {
	target := ids.NewV7()
	pol := agentPolicy{Op: "setDealStakeholder", Tool: "update_record", RecordType: recordTypeDeal}
	req := summaryRequest("PUT", "/v1/deals/"+target.String()+"/stakeholders/c")
	body := []byte(`{"role":"champion"}`)

	for name, info := range map[string]agents.StageInfo{
		"by id":       {Summary: "Set the stakeholder on " + target.String(), TargetID: target},
		"empty":       {TargetID: target},
		"fields only": {Summary: "Update a deal, setting role", TargetID: target, FieldsOnly: true},
	} {
		if got, want := stagedSentence(info, inEnglish, pol, req, body), restSummary(inEnglish, pol, req, body); got != want {
			t.Errorf("%s: staged %q, want this door's %q", name, got, want)
		}
	}
}
