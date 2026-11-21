// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"

	"github.com/margince/margince/backend/internal/compose/network"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// installationToday is the day dealseat.CurrentSQL compares an End date with,
// read from the same database clock so the fixture cannot straddle midnight.
func installationToday(t *testing.T, e *Env) time.Time {
	t.Helper()
	day, err := time.Parse(time.DateOnly, e.WsScalar(t, `SELECT (`+occurredday.TodaySQL("now()")+`)::text`))
	if err != nil {
		t.Fatalf("reading the installation's today: %v", err)
	}
	return day
}

// seatWithEnd seats a new contact on deal through the real writer, and sets
// its End date through the real edit when end is not nil.
func seatWithEnd(t *testing.T, e *Env, deal ids.DealID, name string, end *time.Time) ids.ContactID {
	t.Helper()
	contact := ids.From[ids.ContactKind](e.SeedContact(t, name, &e.Rep1))
	edge, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "deal_stakeholder", ContactID: &contact, DealID: &deal, Source: "manual",
	})
	if err != nil {
		t.Fatalf("seating %s: %v", name, err)
	}
	if end != nil {
		if _, err := e.Contacts.UpdateRelationship(e.Admin(), edge.ID, contacts.UpdateRelationshipInput{EndedAt: end}); err != nil {
			t.Fatalf("ending %s's seat: %v", name, err)
		}
	}
	return contact
}

// An End date takes a seat off the committee on that day, and not before.
// The committee read, the deal coverage the card draws and the contact page
// must agree, because each once answered this on its own.
func TestADealSeatLeavesTheCommitteeOnItsEndDay(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Seat currency", pipeline, open, &e.Rep1))
	today := installationToday(t, e)
	yesterday, tomorrow := today.AddDate(0, 0, -1), today.AddDate(0, 0, 1)
	openEnded := seatWithEnd(t, e, deal, "Open-ended seat", nil)
	endedYesterday := seatWithEnd(t, e, deal, "Ended yesterday", &yesterday)
	endsToday := seatWithEnd(t, e, deal, "Ends today", &today)
	endsTomorrow := seatWithEnd(t, e, deal, "Ends tomorrow", &tomorrow)
	want := []ids.UUID{openEnded.UUID, endsTomorrow.UUID}
	slices.SortFunc(want, compareUUID)

	ctx := e.Admin()
	var seated, covered []ids.UUID
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		seats, err := deals.StakeholderSeats(ctx, tx, []ids.UUID{deal.UUID})
		if err != nil {
			return err
		}
		for _, seat := range seats[deal.UUID] {
			seated = append(seated, seat.ContactID)
		}
		coverage, err := network.CoverageFor(ctx, tx, deal, clocktest.Now(t))
		for _, seat := range coverage.Stakeholders {
			covered = append(covered, seat.ContactID)
		}
		return err
	})
	if err != nil {
		t.Fatalf("reading the committee: %v", err)
	}
	slices.SortFunc(seated, compareUUID)
	slices.SortFunc(covered, compareUUID)
	if !slices.Equal(seated, want) {
		t.Errorf("StakeholderSeats = %v, want the open-ended seat and the one ending tomorrow %v", seated, want)
	}
	if !slices.Equal(covered, want) {
		t.Errorf("the deal coverage seats %v, want %v", covered, want)
	}

	room := contactRoomService(e)
	for contact, current := range map[ids.ContactID]bool{endsTomorrow: true, endsToday: false, endedYesterday: false} {
		page, err := room.Assemble(ctx, contact)
		if err != nil {
			t.Fatalf("assembling the contact page: %v", err)
		}
		hasDeal := page.Commercial != nil && page.Commercial.Deal != nil
		if hasDeal != current {
			t.Errorf("contact %s: the contact page shows the deal = %v, want %v", contact, hasDeal, current)
		}
	}
}

func compareUUID(a, b ids.UUID) int { return slices.Compare(a[:], b[:]) }

// memberSeat is Rep1 holding the grants the shipped Member role seeds, read
// from the seed itself, so the test proves the role a salesperson has.
func memberSeat(t *testing.T, e *Env) context.Context {
	t.Helper()
	grants := seededRoleGrants(t)[roleRep]
	if !grants["relationship"].Create || grants["relationship"].Delete {
		t.Fatalf("the seeded Member role holds relationship %+v; this test needs create without delete", grants["relationship"])
	}
	return e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{roleRep}, Objects: grants, RowScope: principal.RowScopeTeam,
	})
}

// Whoever may write a deal may take a stakeholder off it, without the
// relationship delete grant; the Member role holds create and not delete.
func TestARepWhoMayWriteTheDealRemovesItsStakeholder(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	rep := memberSeat(t, e)
	contact := ids.From[ids.ContactKind](e.SeedContact(t, "Wrongly seated", &e.Rep1))
	seat := func(deal ids.DealID) ids.UUID {
		edge, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
			Kind: "deal_stakeholder", ContactID: &contact, DealID: &deal, Source: "manual",
		})
		if err != nil {
			t.Fatalf("seating the contact: %v", err)
		}
		return edge.ID
	}

	owned := ids.From[ids.DealKind](e.SeedDeal(t, "Rep's own deal", pipeline, open, &e.Rep1))
	edge := seat(owned)
	if err := e.Contacts.RefuseEdgeWrite(rep, "deal_stakeholder", "create"); err != nil {
		t.Errorf("the undo of seating a stakeholder is refused to the deal's owner: %v", err)
	}
	if err := e.Contacts.RefuseEdgeWrite(rep, "employment", "create"); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the undo of an employment link is offered without the delete grant: %v", err)
	}
	if err := e.Contacts.RefuseArchiveRelationship(rep, edge); err != nil {
		t.Fatalf("the stage-time check refuses the deal's owner: %v", err)
	}
	removed, err := e.Contacts.ArchiveRelationship(rep, edge, nil)
	if err != nil || removed.ArchivedAt == nil {
		t.Fatalf("the deal's owner could not remove its stakeholder: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox o JOIN audit_log a ON a.id::text = o.envelope->'trace'->>'audit_log_id'
		WHERE a.entity_type = 'relationship' AND a.entity_id = $1 AND a.action = 'archive'`, edge); n != 1 {
		t.Errorf("the removal wrote %d audit rows with an outbox event, want 1", n)
	}

	shared := ids.From[ids.DealKind](e.SeedDeal(t, "Colleague's shared deal", pipeline, open, &e.Rep2))
	e.ShareForWriting(t, "deal", shared.UUID, "user", e.Rep1)
	if _, err := e.Contacts.ArchiveRelationship(rep, seat(shared), nil); err != nil {
		t.Fatalf("a write share on the deal could not remove its stakeholder: %v", err)
	}

	readOnly := ids.From[ids.DealKind](e.SeedDeal(t, "Other team's deal", pipeline, open, &e.Rep3))
	if _, err := identity.NewService(e.Pool).CreateRecordGrant(e.Admin(), identity.CreateGrantInput{
		RecordType: "deal", RecordID: readOnly.UUID, SubjectType: "user", SubjectID: e.Rep1, Access: "read",
	}); err != nil {
		t.Fatalf("sharing the deal for reading: %v", err)
	}
	kept := seat(readOnly)
	if _, err := e.Contacts.GetRelationship(rep, kept); err != nil {
		t.Fatalf("the read share does not let the rep see the seat, so the refusal below proves nothing: %v", err)
	}
	if err := e.Contacts.RefuseArchiveRelationship(rep, kept); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the stage-time check admits a rep who may only read the deal: %v", err)
	}
	if _, err := e.Contacts.ArchiveRelationship(rep, kept, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep who may only read the deal removed its stakeholder: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM relationship WHERE id = $1 AND archived_at IS NULL`, kept); n != 1 {
		t.Errorf("the refused removal still archived the edge")
	}
}

// Only the deal seat comes off with the create grant. Every other kind of
// edge still needs delete, and the delete grant removes it.
func TestTheCreateGrantRemovesNoOtherKindOfEdge(t *testing.T) {
	e := Setup(t)
	rep := memberSeat(t, e)
	contact := ids.From[ids.ContactKind](e.SeedContact(t, "Colleague contact", &e.Rep1))
	peer := ids.From[ids.ContactKind](e.SeedContact(t, "Their peer", &e.Rep1))
	edge, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "works_with", ContactID: &contact, CounterpartyContactID: &peer, Source: "manual",
	})
	if err != nil {
		t.Fatalf("linking the two contacts: %v", err)
	}
	if _, err := e.Contacts.ArchiveRelationship(rep, edge.ID, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a seat without the delete grant removed a works_with edge: %v", err)
	}
	withDelete := RepPerms
	withDelete.Objects = maps.Clone(RepPerms.Objects)
	withDelete.Objects["relationship"] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	if _, err := e.Contacts.ArchiveRelationship(e.As(e.Rep1, []ids.UUID{e.Team1}, withDelete), edge.ID, nil); err != nil {
		t.Errorf("a seat with the delete grant could not remove the works_with edge: %v", err)
	}
}

// A seat that may put a contact on a deal it writes may decide the staged
// archive of that seat, without the delete grant. Every other kind of edge
// still needs delete to decide.
func TestARepDecidesTheStagedArchiveOfADealSeat(t *testing.T) {
	e := Setup(t)
	rep := memberSeat(t, e)
	svc := compose.NewApprovalsService(e.DB())
	stage := func(edge ids.UUID) ids.ApprovalID {
		t.Helper()
		raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(`{"record_type":"relationship","id":%q}`, edge)))
		if err != nil {
			t.Fatal(err)
		}
		approval, err := svc.Stage(e.Admin(), approvals.StageInput{
			Kind: "archive_record", TargetType: "relationship", TargetID: edge, ProposedChange: raw, DiffHash: hash,
		})
		if err != nil {
			t.Fatal(err)
		}
		return approval
	}
	pipeline, open, _ := DealFixture(t, e)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Rep's deal", pipeline, open, &e.Rep1))
	contact := ids.From[ids.ContactKind](e.SeedContact(t, "Seated Example", &e.Rep1))
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Employer GmbH", &e.Rep1))
	seat, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "deal_stakeholder", ContactID: &contact, DealID: &deal, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	job, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contact, CompanyID: &company, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Decide(rep, stage(job.ID), true, nil); err == nil {
		t.Error("a rep without delete decided the staged archive of an employment edge")
	}
	decided, err := svc.Decide(rep, stage(seat.ID), true, nil)
	if err != nil || decided.Status == "pending" {
		t.Fatalf("the deal's owner deciding the staged archive of its seat → %v (%s), want it decided", err, decided.Status)
	}
}
