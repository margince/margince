// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/network"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
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
	err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
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

// The undo of seating a stakeholder takes the deal seat's removal rule, so the
// Member role may undo it. The undo of an employment link still needs delete.
func TestARepMayUndoSeatingAStakeholder(t *testing.T) {
	e := Setup(t)
	rep := memberSeat(t, e)
	if err := e.Contacts.RefuseEdgeWrite(rep, "deal_stakeholder", "create"); err != nil {
		t.Errorf("the undo of seating a stakeholder is refused to a Member: %v", err)
	}
	if err := e.Contacts.RefuseEdgeWrite(rep, "employment", "create"); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the undo of an employment link is offered without the delete grant: %v", err)
	}
}

// A seat that may write a deal may decide the staged archive of its seat,
// without the delete grant. Every other kind of edge
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
