// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The subject-label pass as a spec: a bound resolver names every card's
// record once, a refusal costs the label and never the reference, and an
// unbound feed sends subjects unnamed as it always did.

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stubNames answers from a fixed map and counts every ask, so a test can
// assert both the answer and that one record costs one read.
type stubNames struct {
	labels map[ids.UUID]string
	asked  map[ids.UUID]int
}

func (s *stubNames) Label(_ context.Context, _ string, id ids.UUID) (string, bool, error) {
	if s.asked == nil {
		s.asked = map[ids.UUID]int{}
	}
	s.asked[id]++
	label, ok := s.labels[id]
	return label, ok, nil
}

func TestABoundResolverNamesEveryCardOnce(t *testing.T) {
	deal := ids.NewV7()
	names := &stubNames{labels: map[ids.UUID]string{deal: "Fleet retrofit GmbH"}}
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{rows: []BriefEntry{
			{ID: ids.NewV7(), DealID: deal, Rank: 1},
			{ID: ids.NewV7(), DealID: deal, Rank: 2},
		}}, nil,
		stubAtRisk{rows: []RiskyDeal{{DealID: deal, Name: "Fleet retrofit", QuietDays: 19}}},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, names,
		fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	for _, item := range out.ThisMorning {
		if item.Subject == nil || item.Subject.Label == nil || *item.Subject.Label != "Fleet retrofit GmbH" {
			t.Errorf("a brief item's subject is unnamed: %+v", item.Subject)
		}
	}
	risk := (*out.AtRisk)[0]
	if risk.Subject == nil || risk.Subject.Label == nil || *risk.Subject.Label != "Fleet retrofit GmbH" {
		t.Errorf("the risk card's subject is unnamed: %+v", risk.Subject)
	}
	// Three cards, one record, one read: the pass resolves per DISTINCT
	// subject, or it reintroduces the N+1 it exists to remove.
	if got := names.asked[deal]; got != 1 {
		t.Errorf("the resolver was asked %d times for one record, want 1", got)
	}
}

func TestARefusedLabelCostsTheNameAndNeverTheReference(t *testing.T) {
	deal := ids.NewV7()
	// The resolver knows nothing about this record: gone, archived, or not
	// this reader's to see. All three answer ok=false.
	names := &stubNames{labels: map[ids.UUID]string{}}
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{rows: []BriefEntry{{ID: ids.NewV7(), DealID: deal, Rank: 1}}}, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, names,
		fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	item := out.ThisMorning[0]
	if item.Subject == nil {
		t.Fatal("the refusal removed the subject — the reference is the producer's claim, not the resolver's to retract")
	}
	if item.Subject.Label != nil {
		t.Errorf("label = %q for a record the resolver refused, want absent", *item.Subject.Label)
	}
	if ids.UUID(item.Subject.Id) != deal {
		t.Errorf("subject id = %s, want the deal", item.Subject.Id)
	}
}

func TestAnUnboundFeedSendsSubjectsUnnamed(t *testing.T) {
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{rows: []BriefEntry{{ID: ids.NewV7(), DealID: ids.NewV7(), Rank: 1}}}, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if item := out.ThisMorning[0]; item.Subject == nil || item.Subject.Label != nil {
		t.Errorf("an unbound feed changed the subject shape: %+v", item.Subject)
	}
}

// Every lane's subjects reach the pass — enumerated from the same answer the
// wire carries, so a lane the pass skipped would fail here by existing.
func TestEveryLanesSubjectsAreNamed(t *testing.T) {
	person, deal := ids.NewV7(), ids.NewV7()
	names := &stubNames{labels: map[ids.UUID]string{person: "Dana Weiss", deal: "Fleet retrofit GmbH"}}
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{},
		&stubCommitments{rows: []Commitment{{
			ID: ids.NewV7(), PersonID: person, Body: "a promise", Quote: "q",
			OccurredAt: readInstant, DueAt: readInstant,
		}}},
		stubAtRisk{rows: []RiskyDeal{{DealID: deal, Name: "Fleet retrofit", QuietDays: 19}}},
		&stubDecay{rows: []QuietRelationship{{PersonID: person, Name: "Dana Weiss", QuietDays: 63, LastAt: readInstant}}},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, names,
		fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	for name, lane := range map[string]*[]crmcontracts.AttentionItem{
		"commitments": out.Commitments, "at_risk": out.AtRisk, "relationship_decay": out.RelationshipDecay,
	} {
		if lane == nil || len(*lane) == 0 {
			t.Fatalf("%s is empty, so this test checked nothing there", name)
		}
		for _, item := range *lane {
			if item.Subject != nil && item.Subject.Label == nil {
				t.Errorf("%s carries an unnamed subject: %+v", name, item.Subject)
			}
		}
	}
}
