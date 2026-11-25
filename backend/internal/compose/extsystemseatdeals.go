// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The seat-bound forms of the deal, authority and directory system reads. Each
// runs as the unit's service member through seatRead, so the seat's own grants,
// row scope and field masks narrow it the way the app narrows that seat.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/fieldmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

func (d systemDeals) GetAs(ctx context.Context, actAs extension.UserID, dealID string) (extension.DealSummary, error) {
	seatCtx, seat, err := d.rt.seatRead(ctx, extension.SystemReadDeals, actAs)
	if err != nil {
		return extension.DealSummary{}, err
	}
	id, err := ids.ParseAs[ids.DealKind](dealID)
	if err != nil {
		return extension.DealSummary{}, fmt.Errorf("%w: the deal id is not a canonical UUID", extension.ErrInvalid)
	}
	if err := seat.trace(fieldmask.Deal, dealID, nil); err != nil {
		return extension.DealSummary{}, err
	}
	var (
		got   crmcontracts.Deal
		stage string
	)
	err = database.WithWorkspaceTx(seatCtx, d.rt.deps.pool, func(tx pgx.Tx) error {
		var err error
		// The deals store is the app's own read, so its object grant, row scope
		// and field mask decide what the seat is given.
		got, err = deals.NewStore(InstallationDB(d.rt.deps.pool), DealsInstallation()).
			GetDealTx(seatCtx, tx, id, storekit.IncludeArchived, deals.CustomColumns{})
		if err != nil || got.StageId == nil {
			return err
		}
		return tx.QueryRow(seatCtx, `SELECT name FROM stage WHERE id = $1`, ids.UUID(*got.StageId)).Scan(&stage)
	})
	if err != nil {
		return extension.DealSummary{}, seat.seatFault(err)
	}
	return seatDealSummary(got, stage), nil
}

// seatDealSummary is the summary of a deal the store already masked for the
// seat: a withheld amount arrives nil and stays empty here.
func seatDealSummary(d crmcontracts.Deal, stage string) extension.DealSummary {
	out := extension.DealSummary{
		ID: ids.UUID(d.Id).String(), Name: d.Name, StageName: stage, Status: string(d.Status),
		AmountMinor: d.AmountMinor, Currency: d.Currency, Archived: d.ArchivedAt != nil,
	}
	if d.AmountMinor == nil || d.Currency == nil {
		out.AmountMinor, out.Currency = nil, nil
	} else {
		out.AmountDisplay = mailcopy.For(string(mailcopy.English)).Money(*d.AmountMinor, *d.Currency)
	}
	if d.ExpectedCloseDate != nil {
		closes := time.Date(d.ExpectedCloseDate.Year(), d.ExpectedCloseDate.Month(), d.ExpectedCloseDate.Day(), 0, 0, 0, 0, time.UTC)
		out.ExpectedClose = &closes
	}
	if d.OwnerId != nil {
		out.OwnerID = extension.UserID(ids.UUID(*d.OwnerId).String())
	}
	if d.MergedIntoId != nil {
		out.MergedIntoID = ids.UUID(*d.MergedIntoId).String()
	}
	return out
}

func (d systemDeals) ResponsibleAs(ctx context.Context, actAs extension.UserID, dealID string) ([]extension.Responsible, error) {
	seatCtx, seat, err := d.rt.seatRead(ctx, extension.SystemReadDeals, actAs)
	if err != nil {
		return nil, err
	}
	id, err := unitID(dealID, "deal")
	if err != nil {
		return nil, err
	}
	if err := seat.trace(fieldmask.Deal, dealID, nil); err != nil {
		return nil, err
	}
	if err := seat.opensDeal(seatCtx, id); err != nil {
		return nil, err
	}
	responsible, err := readDealResponsible(seatCtx, d.rt.deps.pool, id)
	if err != nil && !errors.Is(err, extension.ErrNotFound) {
		return nil, seat.seatFault(err)
	}
	return responsible, err
}

// opensDeal answers ErrNotFound unless the seat may open the deal in the app,
// so a deal outside its scope cannot be told from one that does not exist.
func (s systemSeat) opensDeal(seatCtx context.Context, deal ids.UUID) error {
	if err := auth.Require(seatCtx, fieldmask.Deal, principal.ActionRead); err != nil {
		return extension.ErrNotFound
	}
	var visible bool
	err := database.WithWorkspaceTx(seatCtx, s.rt.deps.pool, func(tx pgx.Tx) error {
		var err error
		visible, err = auth.VisibleToExisting(seatCtx, tx, fieldmask.Deal, deal)
		return err
	})
	if err != nil {
		return s.seatFault(err)
	}
	if !visible {
		return extension.ErrNotFound
	}
	return nil
}

// colleagueQuestion admits a seat's question about user and deal. The seat
// must open the deal, and a question about anyone but itself takes the roster
// read an administrator holds, as ByRole does.
func (s systemSeat) colleagueQuestion(seatCtx context.Context, user extension.UserID, record extension.EntityRef) (ids.UUID, ids.UUID, error) {
	userID, dealID, err := dealQuestion(user, record)
	if err != nil {
		return ids.Nil, ids.Nil, err
	}
	if userID != s.member && !auth.Allows(seatCtx, "user_admin", principal.ActionRead) {
		return ids.Nil, ids.Nil, fmt.Errorf("%w: the seat may ask about a colleague only with the roster read of an administrator", extension.ErrForbidden)
	}
	if err := s.trace(record.Type, record.ID, nil); err != nil {
		return ids.Nil, ids.Nil, err
	}
	if err := s.opensDeal(seatCtx, dealID); err != nil {
		return ids.Nil, ids.Nil, err
	}
	return userID, dealID, nil
}

func (a systemAuthority) CanReadAs(ctx context.Context, actAs, user extension.UserID, record extension.EntityRef, fields []string) (bool, error) {
	seatCtx, seat, err := a.rt.seatRead(ctx, extension.SystemReadAuthority, actAs)
	if err != nil {
		return false, err
	}
	if err := knownDealFields(fields); err != nil {
		return false, err
	}
	userID, dealID, err := seat.colleagueQuestion(seatCtx, user, record)
	if err != nil {
		return false, err
	}
	shown, err := dealShownTo(seat.sys, a.rt.deps.pool, userID, dealID, fields)
	if err != nil {
		return false, a.rt.systemReadFault(seat.sys, err)
	}
	return shown, nil
}

func (a systemAuthority) CanWriteAs(ctx context.Context, actAs, user extension.UserID, record extension.EntityRef) (bool, error) {
	seatCtx, seat, err := a.rt.seatRead(ctx, extension.SystemReadAuthority, actAs)
	if err != nil {
		return false, err
	}
	userID, dealID, err := seat.colleagueQuestion(seatCtx, user, record)
	if err != nil {
		return false, err
	}
	writable, err := dealWritableBy(seat.sys, a.rt.deps.pool, userID, dealID)
	if err != nil {
		return false, a.rt.systemReadFault(seat.sys, err)
	}
	return writable, nil
}

func (d systemDirectory) EmailsAs(ctx context.Context, actAs extension.UserID, users []extension.UserID) (map[extension.UserID]string, error) {
	seatCtx, seat, err := d.rt.seatRead(ctx, extension.SystemReadDirectory, actAs)
	if err != nil {
		return nil, err
	}
	wanted, err := colleagueIDs(users)
	if err != nil {
		return nil, err
	}
	// The gate of the app's member list: a member sees the active seats, and
	// readColleagueEmails answers only those.
	if err := auth.RequireMember(seatCtx); err != nil {
		return nil, fmt.Errorf("%w: the seat may not read the member list", extension.ErrForbidden)
	}
	if len(wanted) == 0 {
		return map[extension.UserID]string{}, nil
	}
	if err := seat.trace("user", fmt.Sprintf("%d users", len(wanted)), nil); err != nil {
		return nil, err
	}
	emails, err := readColleagueEmails(seatCtx, d.rt.deps.pool, wanted)
	if err != nil {
		return nil, seat.seatFault(err)
	}
	return emails, nil
}

// knownDealFields refuses a field a deal does not have.
func knownDealFields(fields []string) error {
	known := dealFieldNames()
	for _, field := range fields {
		if !slices.Contains(known, field) {
			return fmt.Errorf("%w: a deal has no field %q", extension.ErrInvalid, field)
		}
	}
	return nil
}

// colleagueIDs reads the users an Emails call asks about, at most
// MaxResponsible of them.
func colleagueIDs(users []extension.UserID) ([]ids.UUID, error) {
	if len(users) > extension.MaxResponsible {
		return nil, fmt.Errorf("%w: %d users in one call, and the most is %d",
			extension.ErrInvalid, len(users), extension.MaxResponsible)
	}
	wanted := make([]ids.UUID, 0, len(users))
	for _, user := range users {
		id, err := unitID(string(user), "user")
		if err != nil {
			return nil, err
		}
		wanted = append(wanted, id)
	}
	return wanted, nil
}
