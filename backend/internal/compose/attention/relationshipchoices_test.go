// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestQuietRelationshipsAreOptionalUntilTheReaderChoosesThem(t *testing.T) {
	contact := ids.NewV7()
	day := crmcontracts.Attention{AsOf: rankInstant, RelationshipDecay: lane(lapsedItem(QuietRelationship{
		ContactID: contact, Name: "Demo contact", QuietDays: 180, LastAt: rankInstant, HasOpenDeal: true,
	}))}
	for _, pinned := range []bool{false, true} {
		svc := &Service{pinned: map[RowRef]bool{{Source: "relationship_decay", RowID: contact.String()}: pinned}}
		out := svc.worklistFrom(context.Background(), day, "all", "all", 25, waitingRead{}, worklistCursor{}, nil)
		if out.OptionalRelationships == nil || out.Focus == nil {
			t.Fatal("missing optional/focus projection")
		}
		if !pinned && (len(out.Queue) != 0 || out.Focus.Total != 0 || len(*out.OptionalRelationships) != 1) {
			t.Fatalf("quiet contact manufactured responsible work: %+v", out)
		}
		if pinned && (len(out.Queue) != 1 || out.Focus.Total != 1 || len(*out.OptionalRelationships) != 0 || !hasReason(out.Queue[0], "pinned")) {
			t.Fatalf("explicit choice was lost: %+v", out)
		}
		if out.Focus.UrgentRemaining != 0 {
			t.Fatal("a personal choice became urgency")
		}
	}
}

func TestAnExistingTaskSuppressesAnOptionalReconnect(t *testing.T) {
	contact := ids.NewV7()
	planned := item("task", "task", withDue(rankInstant))
	planned.Subject = &crmcontracts.AttentionSubject{Type: "contact", Id: crmcontracts.Id(contact)}
	day := crmcontracts.Attention{
		AsOf: rankInstant, Planned: []crmcontracts.AttentionItem{planned},
		RelationshipDecay: lane(lapsedItem(QuietRelationship{ContactID: contact, Name: "Demo contact", QuietDays: 90, LastAt: rankInstant})),
	}
	out := (&Service{}).worklistFrom(context.Background(), day, "all", "all", 25, waitingRead{}, worklistCursor{}, nil)
	if len(out.Queue) != 1 || out.OptionalRelationships == nil || len(*out.OptionalRelationships) != 0 {
		t.Fatalf("task and reconnection duplicated work: %+v", out)
	}
}

func TestFocusKeepsObligationsAheadOfChosenRelationships(t *testing.T) {
	contact := ids.NewV7()
	day := crmcontracts.Attention{AsOf: rankInstant, RelationshipDecay: lane(lapsedItem(QuietRelationship{ContactID: contact, Name: "Demo contact", QuietDays: 90, LastAt: rankInstant}))}
	for i := range 6 {
		day.Planned = append(day.Planned, item(string(rune('a'+i)), "task", withDue(rankInstant.Add(-time.Hour))))
	}
	svc := &Service{pinned: map[RowRef]bool{{Source: "relationship_decay", RowID: contact.String()}: true}}
	out := svc.worklistFrom(context.Background(), day, "all", "all", 25, waitingRead{}, worklistCursor{}, nil)
	if out.Focus == nil || out.Focus.Total != 7 || out.Focus.UrgentRemaining != 0 || len(out.Focus.Items) != 6 {
		t.Fatalf("choice displaced urgency: %+v", out.Focus)
	}
	for _, row := range out.Focus.Items {
		if row.Source != "task" {
			t.Fatalf("routine choice outranked obligations: %+v", row)
		}
	}
	if len(out.Queue) != 7 || out.Queue[0].Source != sourceDecay || !hasReason(out.Queue[0], "pinned") {
		t.Fatalf("personal queue lost the deliberate choice: %+v", out.Queue)
	}
}

func TestAnyExplicitContactWorkSuppressesADuplicateReconnection(t *testing.T) {
	contact := ids.NewV7()
	for _, source := range []string{"customer_waiting", "awaiting_reply", "meeting_follow_up", "conversation_claim", "task", "future_contact_work"} {
		work := ranked{item: crmcontracts.WorklistItem{Source: crmcontracts.WorklistItemSource(source)}, contact: contact}
		quiet := classifyDecay(lapsedItem(QuietRelationship{ContactID: contact, Name: "Demo contact"}), rankInstant)
		if got := dropDecayAlreadyWaiting([]ranked{work, quiet}); len(got) != 1 || got[0].item.Source != work.item.Source {
			t.Fatalf("%s duplicated a reconnection: %+v", source, got)
		}
	}
	work := ranked{item: crmcontracts.WorklistItem{Source: "conversation_claim", PromiseRecipients: &[]crmcontracts.WorklistPromiseRecipient{{ContactId: crmcontracts.Id(contact)}}}}
	quiet := classifyDecay(lapsedItem(QuietRelationship{ContactID: contact, Name: "Demo contact"}), rankInstant)
	if got := dropDecayAlreadyWaiting([]ranked{work, quiet}); len(got) != 1 {
		t.Fatalf("explicit promise recipient duplicated a reconnection: %+v", got)
	}
}

func TestChosenRelationshipsKeepFailureAndScanCoverage(t *testing.T) {
	contact := ids.NewV7()
	svc := &Service{pinned: map[RowRef]bool{{Source: "relationship_decay", RowID: contact.String()}: true}}
	omitted := []crmcontracts.AttentionLanesOmitted{"relationship_decay"}
	day := crmcontracts.Attention{AsOf: rankInstant, LanesOmitted: &omitted}
	out := svc.worklistFrom(context.Background(), day, "all", "all", 25, waitingRead{}, worklistCursor{}, nil)
	found := false
	for _, source := range out.SourcesUnavailable {
		if source.Source == "relationship_decay" {
			found = source.ContributesRows != nil && *source.ContributesRows
		}
	}
	if !found {
		t.Fatalf("missing chosen work looked complete: %+v", out.SourcesUnavailable)
	}
	day.LanesOmitted = nil
	day.RelationshipDecay = lane(lapsedItem(QuietRelationship{ContactID: contact, Name: "Demo contact"}))
	day.RelationshipDecayMoreAvailable = new(true)
	out = svc.worklistFrom(context.Background(), day, "all", "all", 25, waitingRead{}, worklistCursor{}, nil)
	for _, reach := range out.Reach {
		if reach.Source == "relationship_decay" && reach.MoreAvailable {
			return
		}
	}
	t.Fatalf("chosen source lost its scan bound: %+v", out.Reach)
}
