// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// offerUpdate is one machine update of an offer, placed on its deal.
func offerUpdate(before, after string) entry {
	label, parent, parentType := "Northwind renewal", ids.NewV7(), typeDeal
	return entry{
		ID: ids.NewV7(), OccurredAt: time.Now(), Action: actionUpdate, EntityType: childOffer,
		EntityID: ids.NewV7(), ActorType: "agent", ActorID: "agent:test", Label: &label,
		Before: []byte(before), After: []byte(after), ParentType: &parentType, ParentID: &parent,
	}
}

func TestAPlacedLineNamesItsParentAndShowsNoImages(t *testing.T) {
	e := offerUpdate(`{"total_minor": 100}`, `{"total_minor": 200}`)
	line, _, ok := lineOf(unmasked(t), e)
	if !ok {
		t.Fatal("an offer update that moved a field was not shown")
	}
	if line.Summary.Key != "magic.action.update_offer" {
		t.Errorf("the line reads %q, want the offer's own sentence", line.Summary.Key)
	}
	if line.Entity.Type != typeDeal || ids.UUID(line.Entity.Id) != *e.ParentID {
		t.Errorf("the line names %s %v, want the offer's deal", line.Entity.Type, line.Entity.Id)
	}
	if line.Before != nil || line.After != nil {
		t.Error("the line shows the offer's images, which no field mask governs")
	}
	if changes := changesOf(e); len(changes) != 0 {
		t.Errorf("the opened line lists the offer's fields: %+v", changes)
	}
}

func TestAPlacedUpdateOfOnlyBookkeepingIsNotShown(t *testing.T) {
	if _, _, ok := lineOf(unmasked(t), offerUpdate(`{"version": 1}`, `{"version": 2}`)); ok {
		t.Fatal("an offer update that moved only its version was shown")
	}
}

// The undo judge is asked about the audited record. A placed line names the
// parent, and judging the offer's row as a deal's would offer the deal's undo.
func TestTheUndoJudgeIsAskedAboutTheAuditedType(t *testing.T) {
	e := offerUpdate(`{"total_minor": 100}`, `{"total_minor": 200}`)
	line, _, ok := lineOf(unmasked(t), e)
	if !ok {
		t.Fatal("the line was not drawn")
	}
	subjects := entriesOf([]crmcontracts.MagicLine{line}, []entry{e})
	if len(subjects) != 1 || subjects[0].EntityType != childOffer {
		t.Fatalf("the judge is asked about %+v, want the offer", subjects)
	}
}

// Every placed child has a sentence for each verb a placement reads, so no
// placed row falls silently into housekeeping for want of one.
func TestEveryPlacedChildHasItsSentences(t *testing.T) {
	for _, p := range placements {
		for _, action := range []string{actionCreate, actionUpdate, actionArchive} {
			if placedSentences[p.child][action] == "" {
				t.Errorf("%s has no sentence for %s", p.child, action)
			}
		}
		if recordLabels[p.parent] == "" {
			t.Errorf("%s is placed on %s, which has no label column", p.child, p.parent)
		}
	}
}
