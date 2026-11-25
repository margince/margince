// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The seat-bound deal, authority and directory reads over real Postgres, from a
// subscription delivery. Each answers as the unit's service member sees the
// app, where the old form answers as the system actor.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/fieldmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

const (
	// seatOwnDealGrants limits a seat to the deals it owns.
	seatOwnDealGrants = `{"objects":{"deal":{"read":true}},"row_scope":"own"}`
	// seatDealGrants lets a seat read every deal, and nothing of the roster.
	seatDealGrants = `{"objects":{"deal":{"read":true}},"row_scope":"all"}`
	// seatAdminDealGrants adds the administrators' roster read.
	seatAdminDealGrants = `{"objects":{"deal":{"read":true},"user_admin":{"read":true}},"row_scope":"all"}`
)

func TestASeatBoundDealReadAnswersOnlyWhatTheSeatMayOpen(t *testing.T) {
	s := setupSystemRead(t)
	seat := s.serviceSeat(t, seatOwnDealGrants)
	as := extension.UserID(seat.String())
	var (
		old                  extension.DealSummary
		getAs, responsibleAs error
	)
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		var err error
		if old, err = rt.Deals().Get(ctx, s.deal.String()); err != nil {
			return err
		}
		_, getAs = rt.Deals().GetAs(ctx, as, s.deal.String())
		_, responsibleAs = rt.Deals().ResponsibleAs(ctx, as, s.deal.String())
		return nil
	})
	if old.Name != "Fleet renewal" {
		t.Errorf("the system actor's Get = %+v, want the deal", old)
	}
	for read, err := range map[string]error{"GetAs": getAs, "ResponsibleAs": responsibleAs} {
		if !errors.Is(err, extension.ErrNotFound) {
			t.Errorf("%s of a deal outside the seat's row scope: err = %v, want ErrNotFound", read, err)
		}
	}
}

// The seat opens the deal it names, which was merged into one it may not open.
// The colleagues are the survivor's, so the read is refused.
func TestResponsibleAsRefusesTheSurvivorOfAMergeTheSeatCannotOpen(t *testing.T) {
	s := setupSystemRead(t)
	seat := s.serviceSeat(t, seatOwnDealGrants)
	var pipeline, stage ids.UUID
	s.read(t, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT pipeline_id, stage_id FROM deal WHERE id = $1`, s.deal).Scan(&pipeline, &stage)
	})
	source, err := s.Deals.CreateDeal(s.Admin(), deals.CreateDealInput{
		Name: "Fleet renewal, second copy", PipelineID: ids.From[ids.PipelineKind](pipeline),
		StageID: ids.From[ids.StageKind](stage), Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	sourceID := ids.From[ids.DealKind](ids.UUID(source.Id))
	if _, err := s.Deals.MergeDeal(s.Admin(), sourceID, ids.From[ids.DealKind](s.deal), nil); err != nil {
		t.Fatalf("merging the copy into the deal: %v", err)
	}
	s.WsExec(t, `UPDATE deal SET owner_id = $1 WHERE id = $2`, seat, sourceID.UUID)
	var refused error
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		_, refused = rt.Deals().ResponsibleAs(ctx, extension.UserID(seat.String()), sourceID.String())
		return nil
	})
	if !errors.Is(refused, extension.ErrNotFound) {
		t.Errorf("ResponsibleAs through a merge into a deal outside the seat's scope: err = %v, want ErrNotFound", refused)
	}
}

func TestASeatWhoseRoleMasksTheAmountIsGivenNone(t *testing.T) {
	s := setupSystemRead(t)
	seat := s.serviceSeat(t, seatOwnDealGrants)
	s.WsExec(t, `INSERT INTO field_mask (role_key, object, field, condition) VALUES ($1, 'deal', 'amount_minor', 'always')`,
		"deal-reader-"+seat.String())
	s.WsExec(t, `UPDATE deal SET owner_id = $1 WHERE id = $2`, seat, s.deal)
	var (
		got         extension.DealSummary
		responsible []extension.Responsible
	)
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		var err error
		if got, err = rt.Deals().GetAs(ctx, extension.UserID(seat.String()), s.deal.String()); err != nil {
			return err
		}
		responsible, err = rt.Deals().ResponsibleAs(ctx, extension.UserID(seat.String()), s.deal.String())
		return err
	})
	if got.Name != "Fleet renewal" || got.StageName != "Qualified" || got.OwnerID != extension.UserID(seat.String()) {
		t.Errorf("GetAs = %+v, want the seat's own deal with its stage", got)
	}
	if got.AmountMinor != nil || got.Currency != nil || got.AmountDisplay != "" {
		t.Errorf("GetAs gave a masked amount: %v %v %q", got.AmountMinor, got.Currency, got.AmountDisplay)
	}
	if len(responsible) == 0 {
		t.Error("ResponsibleAs of a deal the seat opens answered nobody")
	}
}

func TestASeatAsksAboutAColleagueOnlyWithTheRosterRead(t *testing.T) {
	s := setupSystemRead(t)
	plain, admin := s.serviceSeat(t, seatDealGrants), s.serviceSeat(t, seatAdminDealGrants)
	deal := extension.EntityRef{Type: fieldmask.Deal, ID: s.deal.String()}
	colleague := extension.UserID(s.Rep1.String())
	var refusedRead, refusedWrite error
	var shown bool
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		_, refusedRead = rt.Authority().CanReadAs(ctx, extension.UserID(plain.String()), colleague, deal, []string{"name"})
		_, refusedWrite = rt.Authority().CanWriteAs(ctx, extension.UserID(plain.String()), colleague, deal)
		var err error
		shown, err = rt.Authority().CanReadAs(ctx, extension.UserID(admin.String()), colleague, deal, []string{"name"})
		return err
	})
	for read, err := range map[string]error{"CanReadAs": refusedRead, "CanWriteAs": refusedWrite} {
		if !errors.Is(err, extension.ErrForbidden) {
			t.Errorf("%s about a colleague without the roster read: err = %v, want ErrForbidden", read, err)
		}
	}
	if !shown {
		t.Error("CanReadAs by a seat with the roster read: the colleague who may read the deal was answered no")
	}
}

func TestEmailsAsLeavesOutAUserTheMemberListDoesNotShow(t *testing.T) {
	s := setupSystemRead(t)
	seat := s.serviceSeat(t, seatDealGrants)
	gone := s.seat(t, "gone@example.test", "deactivated", false)
	var got map[extension.UserID]string
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		var err error
		got, err = rt.Directory().EmailsAs(ctx, extension.UserID(seat.String()),
			[]extension.UserID{extension.UserID(s.Rep1.String()), extension.UserID(gone.String())})
		return err
	})
	if _, listed := got[extension.UserID(gone.String())]; listed || len(got) != 1 {
		t.Errorf("EmailsAs = %v, want the active colleague alone", got)
	}
}

func TestEachSeatBoundReadRefusesAnyoneButTheServiceMember(t *testing.T) {
	s := setupSystemRead(t)
	s.connect(t, s.Rep1)
	rep := extension.UserID(s.Rep1.String())
	deal := extension.EntityRef{Type: fieldmask.Deal, ID: s.deal.String()}
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		_, get := rt.Deals().GetAs(ctx, rep, s.deal.String())
		_, responsible := rt.Deals().ResponsibleAs(ctx, rep, s.deal.String())
		_, canRead := rt.Authority().CanReadAs(ctx, rep, rep, deal, nil)
		_, canWrite := rt.Authority().CanWriteAs(ctx, rep, rep, deal)
		_, emails := rt.Directory().EmailsAs(ctx, rep, []extension.UserID{rep})
		for read, err := range map[string]error{
			"GetAs": get, "ResponsibleAs": responsible, "CanReadAs": canRead, "CanWriteAs": canWrite, "EmailsAs": emails,
		} {
			if !errors.Is(err, extension.ErrForbidden) {
				t.Errorf("%s as a connected colleague: err = %v, want ErrForbidden", read, err)
			}
		}
		return nil
	})
}
