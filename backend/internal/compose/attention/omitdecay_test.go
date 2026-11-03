// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// quietFor is one lapsed relationship owned by the page's reader.
func quietFor(contact ids.UUID) *stubDecay {
	actor, _ := principal.Actor(pageReader())
	return &stubDecay{rows: []QuietRelationship{{
		ContactID: contact, Name: "Dana Weiss", OwnerID: actor.UserID, QuietDays: 63, LastAt: readInstant,
	}}}
}

func decayService(decay *stubDecay) *Service {
	return NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil,
		decay, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
}

func TestAnOmittedRelationshipReadSkipsTheDecayLane(t *testing.T) {
	decay := quietFor(ids.NewV7())
	page, err := decayService(decay).OmittingRelationships().Worklist(pageReader(), "", "", ids.Nil, 25, "")
	if err != nil {
		t.Fatalf("worklist: %v", err)
	}
	if decay.calls != 0 {
		t.Errorf("the decay lane was read %d times on an omitted read", decay.calls)
	}
	if page.OptionalRelationships != nil || page.OptionalRelationshipsMoreAvailable != nil {
		t.Errorf("an omitted read still answered reconnections: %+v", page.OptionalRelationships)
	}
}

func TestADefaultReadStillAnswersReconnections(t *testing.T) {
	decay := quietFor(ids.NewV7())
	page, err := decayService(decay).Worklist(pageReader(), "", "", ids.Nil, 25, "")
	if err != nil {
		t.Fatalf("worklist: %v", err)
	}
	if decay.calls != 1 || page.OptionalRelationships == nil || len(*page.OptionalRelationships) != 1 {
		t.Fatalf("calls = %d, optional = %+v", decay.calls, page.OptionalRelationships)
	}
}

func TestAPinnedReconnectionKeepsItsLaneOnAnOmittedRead(t *testing.T) {
	contact := ids.NewV7()
	decay := quietFor(contact)
	ctx := pageReader()
	actor, _ := principal.Actor(ctx)
	pins := pinsFor{actor.ID: {{Source: string(sourceDecay), RowID: contact.String()}: true}}
	page, err := decayService(decay).WithPins(pins).OmittingRelationships().Worklist(ctx, "", "", ids.Nil, 25, "")
	if err != nil {
		t.Fatalf("worklist: %v", err)
	}
	if decay.calls != 1 {
		t.Fatalf("a pinned reconnection lost its lane: %d reads", decay.calls)
	}
	for _, row := range page.Queue {
		if row.Source == sourceDecay && row.Id == contact.String() {
			return
		}
	}
	t.Errorf("the pinned reconnection is not in the queue: %+v", page.Queue)
}
