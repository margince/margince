// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestTheAIAssistedMarkReachesTheStoreFromBothWrites(t *testing.T) {
	for name, sent := range map[string]*bool{"marked": new(true), "unmarked": new(false), "absent": nil} {
		in, err := logActivityInput(crmcontracts.CreateActivityRequest{
			Kind: crmcontracts.CreateActivityRequestKindCreateActivityRequestKindNote, Source: "manual",
			Subject: new("Call Anna about pricing"), AiAssisted: sent,
		}, provenanceAdmission{})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if want := sent != nil && *sent; in.AIAssisted != want {
			t.Errorf("%s create: AIAssisted = %t, want %t", name, in.AIAssisted, want)
		}
		patch := activityUpdateInput(crmcontracts.UpdateActivityRequest{AiAssisted: sent}, nil)
		if patch.AIAssisted != sent {
			t.Errorf("%s patch: AIAssisted = %v, want %v", name, patch.AIAssisted, sent)
		}
	}
}

func TestAPatchMarkingUnmarkedWordsIsAChangeAndFalseIsNone(t *testing.T) {
	plain := crmcontracts.Activity{}
	if (UpdateActivityInput{AIAssisted: new(true)}).changesNothing(plain) {
		t.Error("marking unmarked words reads as a patch that changes nothing")
	}
	if !(UpdateActivityInput{AIAssisted: new(false)}).changesNothing(plain) {
		t.Error("false lifts nothing, yet it reads as a change")
	}
	marked := crmcontracts.Activity{AiAssisted: new(true)}
	if !(UpdateActivityInput{AIAssisted: new(true)}).changesNothing(marked) {
		t.Error("marking words already marked reads as a change")
	}
}
