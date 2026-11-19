// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

// The pack's agreements, touches and tasks. The real readers run in compose's
// suite, and the fixture answers here. These tests hold that the pack freezes
// ids and dates and reads titles at view time. They also hold that it leaves
// out what a reader may not open.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// recordFixture is what the fixture readers hold. A nil open set means the
// reader may open everything; otherwise only the ids it names.
type recordFixture struct {
	contracts     []PacketContract
	touches       []PacketTouch
	tasks         []PacketTask
	subjects      map[ids.UUID]*string
	contractsOpen map[ids.UUID]bool
}

func (e *reviewEnv) ReviewContracts(context.Context, ids.UUID) ([]PacketContract, error) {
	var out []PacketContract
	for _, c := range e.records.contracts {
		if e.records.contractsOpen == nil || e.records.contractsOpen[c.ContractID] {
			out = append(out, c)
		}
	}
	return out, nil
}

func (e *reviewEnv) ReviewActivity(context.Context, ids.UUID) ([]PacketTouch, []PacketTask, error) {
	return e.records.touches, e.records.tasks, nil
}

func (e *reviewEnv) OpenableActivity(_ context.Context, _ ids.UUID, activityIDs []ids.UUID) (map[ids.UUID]*string, error) {
	out := map[ids.UUID]*string{}
	for _, id := range activityIDs {
		if subject, ok := e.records.subjects[id]; ok {
			out[id] = subject
		}
	}
	return out, nil
}

func TestThePackNamesTheDealsAgreementsTouchesAndTasks(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := gatedDeal(t, e)
	now := clocktest.Now(t).UTC()
	msa, hidden := ids.NewV7(), ids.NewV7()
	number, days, auto := "MSA-7", 60, true
	renewal := now.AddDate(1, 0, 0)
	e.records.contracts = []PacketContract{
		{ContractID: msa, ContractNumber: &number, Status: "active", RenewalOn: &renewal, NoticePeriodDays: &days, AutoRenew: &auto, Title: "Master agreement"},
		{ContractID: hidden, Status: "draft", Title: "Side letter"},
	}
	call, mail, task, mailTask := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	subject, taskSubject := "Pricing call", "Send the DPA"
	e.records.touches = []PacketTouch{{ActivityID: call, Kind: "call", OccurredAt: now}, {ActivityID: mail, Kind: "email", OccurredAt: now}}
	e.records.tasks = []PacketTask{{ActivityID: task, DueAt: &now}, {ActivityID: mailTask}}
	// The mail and the second task are discoverable but not readable by the
	// reader, so the fixture's readable set leaves them out.
	e.records.subjects = map[ids.UUID]*string{call: &subject, task: &taskSubject}
	id := e.request(t, dealID, nil)

	// A title changed after the request is read now, and an agreement the
	// reader may no longer open is left out.
	e.records.contracts[0].Title = "Master agreement, renewed"
	e.records.contractsOpen = map[ids.UUID]bool{msa: true}
	reader := humanWith(e.ws, e.reviewer, map[string]principal.ObjectGrant{legalReviewEntity: {Read: true}, "deal": {Read: true}})
	got, err := e.store.GetLegalReview(reader, id)
	if err != nil {
		t.Fatalf("reading the pack: %v", err)
	}
	contracts := *got.Packet.Contracts
	if len(contracts) != 1 || ids.UUID(contracts[0].Id) != msa || contracts[0].Title != "Master agreement, renewed" ||
		contracts[0].NoticePeriodDays == nil || *contracts[0].NoticePeriodDays != days || contracts[0].RenewalOn == nil {
		t.Fatalf("the pack showed agreements %+v, want the one still openable with its notice terms", contracts)
	}
	touches := *got.Packet.Touches
	if len(touches) != 1 || ids.UUID(touches[0].ActivityId) != call || touches[0].Subject == nil || *touches[0].Subject != subject {
		t.Fatalf("the pack showed touches %+v, want the readable call alone with no trace of the withheld mail", touches)
	}
	tasks := *got.Packet.Tasks
	if len(tasks) != 1 || ids.UUID(tasks[0].ActivityId) != task || *tasks[0].Subject != taskSubject {
		t.Fatalf("the pack showed tasks %+v, want the readable task alone", tasks)
	}

	// A reviewer without deal.read learns of none of them.
	blind, err := e.store.GetLegalReview(e.asReviewer(principal.ObjectGrant{Read: true}), id)
	if err != nil {
		t.Fatalf("reading the pack without a deal grant: %v", err)
	}
	if blind.Packet.Contracts != nil || blind.Packet.Touches != nil || blind.Packet.Tasks != nil {
		t.Fatalf("a reviewer without deal.read was shown records: %+v", blind.Packet)
	}
}

// A pack asked for before the sections existed has none of their keys. It
// reads as a pack that predates them, never as one that found nothing.
func TestAPackFromBeforeTheRecordSectionsLeavesThemAbsent(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := gatedDeal(t, e)
	id := e.request(t, dealID, nil)
	reader := humanWith(e.ws, e.reviewer, map[string]principal.ObjectGrant{legalReviewEntity: {Read: true}, "deal": {Read: true}})

	fresh, err := e.store.GetLegalReview(reader, id)
	if err != nil || fresh.Packet.Contracts == nil || fresh.Packet.Touches == nil || fresh.Packet.Tasks == nil {
		t.Fatalf("a new pack with nothing to list read %+v (%v), want empty sections", fresh.Packet, err)
	}
	if _, err := e.owner.Exec(t.Context(),
		`UPDATE legal_review SET packet = packet - 'contracts' - 'touches' - 'tasks' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	old, err := e.store.GetLegalReview(reader, id)
	if err != nil {
		t.Fatalf("reading the older pack: %v", err)
	}
	if old.Packet.Contracts != nil || old.Packet.Touches != nil || old.Packet.Tasks != nil {
		t.Fatalf("an older pack showed sections %+v, want them absent", old.Packet)
	}
}
