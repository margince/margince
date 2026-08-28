// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Each optional lane fills ITS OWN field. The four are wired through
// pointers-to-pointers into one response struct, which is exactly the shape
// where a copy-paste slip puts the meetings into at_risk and nothing fails to
// compile.
func TestEachOptionalLaneFillsItsOwnField(t *testing.T) {
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{},
		&stubCommitments{rows: []Commitment{promise("a promise", readInstant)}},
		stubAtRisk{rows: []RiskyDeal{{Name: "a deal", QuietDays: 20}}},
		&stubDecay{rows: []QuietRelationship{{Name: "a contact", QuietDays: 63, LastAt: readInstant}}},
		&stubMeetings{rows: []Meeting{{Subject: "a meeting", StartsAt: readInstant}}},
		&stubFailedEffects{rows: []FailedEffect{{
			ID: ids.NewV7(), Kind: "send_email",
			Sentence: "this was approved, but the work it released did not run",
			FailedAt: readInstant, TargetType: "person", TargetID: ids.NewV7(),
		}}},
		fixedClock,
	)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	for _, lane := range []struct {
		name  string
		items *[]crmcontracts.AttentionItem
		want  string
	}{
		{"meetings", out.Meetings, "a meeting"},
		{"at_risk", out.AtRisk, "a deal"},
		{"relationship_decay", out.RelationshipDecay, "a contact"},
		{"commitments", out.Commitments, "a promise"},
	} {
		if lane.items == nil || len(*lane.items) != 1 {
			t.Fatalf("%s carries %v, want one item", lane.name, lane.items)
		}
		if got := (*lane.items)[0]; got.Title == nil || *got.Title != lane.want {
			t.Errorf("%s holds %q, want %q — a lane wrote into the wrong field",
				lane.name, stringOr(got.Title), lane.want)
		}
	}
}

// No lane advertises an action the surface cannot perform.
//
// A card offering an action nothing renders is a promise to a client that
// nothing keeps — a generated client is entitled to draw a control for whatever
// the server says it offers. AttentionRow renders `complete` and `snooze` as
// buttons, and `open` as the row's own link to the record named in `subject`.
//
// `open` is performable only where the subject is a record with a page. The
// meetings lane's subject is the ACTIVITY behind the appointment, which is a
// timeline entry and not a destination, which is why that lane still sends no
// action at all.
//
// Derived from the lanes rather than listed, so a fourth joins the check by
// existing.
func TestNoOptionalLaneOffersAnActionTheSurfaceCannotPerform(t *testing.T) {
	performable := map[crmcontracts.AttentionItemActions]bool{
		"complete": true, "snooze": true, "open": true,
	}
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{},
		&stubCommitments{rows: []Commitment{promise("a promise", readInstant)}},
		stubAtRisk{rows: []RiskyDeal{{Name: "a deal", QuietDays: 20}}},
		&stubDecay{rows: []QuietRelationship{{Name: "a contact", QuietDays: 63, LastAt: readInstant}}},
		&stubMeetings{rows: []Meeting{{Subject: "a meeting", StartsAt: readInstant}}},
		&stubFailedEffects{rows: []FailedEffect{{
			ID: ids.NewV7(), Kind: "send_email",
			Sentence: "this was approved, but the work it released did not run",
			FailedAt: readInstant, TargetType: "person", TargetID: ids.NewV7(),
		}}},
		fixedClock,
	)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	for name, lane := range map[string]*[]crmcontracts.AttentionItem{
		"meetings": out.Meetings, "at_risk": out.AtRisk, "commitments": out.Commitments,
		"relationship_decay": out.RelationshipDecay,
	} {
		if lane == nil {
			t.Fatalf("%s is absent, so this gate checked nothing", name)
		}
		for _, item := range *lane {
			for _, action := range item.Actions {
				if !performable[action] {
					t.Errorf("%s offers %q, which no control on this surface performs",
						name, action)
				}
			}
		}
	}
}

// An item offering `open` names a record the reader can open.
//
// `open` means "go to the thing this card is about", so it is only honest when
// the card carries the thing: a subject, of a type that is a record with a page
// rather than an entry on a timeline. Without this the previous gate would pass
// for a lane that advertised navigation to nowhere, which is the exact defect
// that kept every one of these lanes inert.
func TestOpenIsOfferedOnlyWithARecordToOpen(t *testing.T) {
	// `activity` is deliberately absent: it is a timeline entry, and no screen
	// answers to it.
	openable := map[crmcontracts.AttentionSubjectType]bool{
		"organization": true, "person": true, "deal": true,
		"lead": true, "project": true,
	}
	// Every lane carries a row, because a lane the fixture leaves empty is a
	// lane this gate silently does not check.
	svc := NewService(
		stubApprovals{rows: []crmcontracts.Approval{approval("Send the Weber follow-up")}},
		stubDuplicates{}, &stubTasks{},
		stubReceipts{rows: []Receipt{{
			ID: ids.NewV7(), Kind: "send_email",
			Summary: "sent the Weber follow-up", OccurredAt: readInstant,
		}}},
		stubBriefing{},
		&stubCommitments{rows: []Commitment{promise("a promise", readInstant)}},
		stubAtRisk{rows: []RiskyDeal{{Name: "a deal", QuietDays: 20}}},
		&stubDecay{rows: []QuietRelationship{{Name: "a contact", QuietDays: 63, LastAt: readInstant}}},
		&stubMeetings{rows: []Meeting{{Subject: "a meeting", StartsAt: readInstant}}},
		&stubFailedEffects{rows: []FailedEffect{{
			ID: ids.NewV7(), Kind: "send_email",
			Sentence: "this was approved, but the work it released did not run",
			FailedAt: readInstant, TargetType: "person", TargetID: ids.NewV7(),
		}}},
		fixedClock,
	)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	// EVERY lane the feed sends, not the optional ones alone. A map naming a
	// subset is the shape of gate that reports PASS over the defect it was
	// written for: done_for_you offered `open` with no subject the whole time
	// this test was green, because the first version of it did not look there.
	lanes := map[string]*[]crmcontracts.AttentionItem{
		"meetings": out.Meetings, "at_risk": out.AtRisk,
		"commitments": out.Commitments, "planned": &out.Planned,
		"relationship_decay": out.RelationshipDecay,
		"did_not_run":        out.DidNotRun,
		"needs_you":          &out.NeedsYou,
		"done_for_you":       &out.DoneForYou,
	}
	for name, lane := range lanes {
		if lane == nil {
			t.Fatalf("%s is absent, so this gate checked nothing", name)
		}
		for _, item := range *lane {
			if !slices.Contains(item.Actions, "open") {
				continue
			}
			if item.Subject == nil {
				t.Errorf("%s offers open with no subject to open", name)
				continue
			}
			if !openable[item.Subject.Type] {
				t.Errorf("%s offers open on a %q, which has no page",
					name, item.Subject.Type)
			}
		}
	}
}
