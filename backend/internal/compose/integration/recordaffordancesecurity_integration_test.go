// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestRecordAffordancesKeepArchivedUpdateAuthorityForMasks(t *testing.T) {
	e := Setup(t)
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	e.Deals.SetClock(func() time.Time { return now })
	pipeline, open, _ := DealFixture(t, e)
	amount, arr, currency := int64(250000), int64(90000), "EUR"
	owner := ids.From[ids.UserKind](e.Rep1)
	made, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Archived authority", PipelineID: pipeline, StageID: open, OwnerID: &owner,
		AmountMinor: &amount, ExpectedArrMinor: &arr, Currency: &currency,
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.DealKind](ids.UUID(made.Id))
	for _, grants := range []recordWriteGrants{{update: true}, {deleteGrant: true}, {}} {
		perms := recordAffordancePerms(grants)
		perms.FieldMasks = []principal.FieldMask{{Object: objDeal, Field: "amount_minor", Condition: principal.MaskOutsideWriteAuthority}}
		ctx := asTeam1Member(e, e.Rep1, perms)
		got, err := e.Deals.GetDeal(ctx, id, storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		requireRecordAffordances(t, got.Writable, got.Archivable, expectedRecordActions{writable: grants.update, archivable: grants.update || grants.deleteGrant})
		if !grants.update {
			assertMoneyWithheld(t, got)
			if _, err := e.Deals.UpdateDeal(ctx, id, deals.UpdateDealInput{}); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("update without update grant = %v, want permission denied", err)
			}
		}
	}
	updatePerms := recordAffordancePerms(recordWriteGrants{update: true})
	updatePerms.FieldMasks = []principal.FieldMask{{Object: objDeal, Field: "amount_minor", Condition: principal.MaskOutsideWriteAuthority}}
	ownerCtx := asTeam1Member(e, e.Rep1, updatePerms)
	echo, err := e.Deals.UpdateDeal(ownerCtx, id, deals.UpdateDealInput{})
	if err != nil {
		t.Fatal(err)
	}
	requireRecordAffordances(t, echo.Writable, echo.Archivable, expectedRecordActions{writable: true, archivable: true})
	if echo.AmountMinor == nil || *echo.AmountMinor != amount {
		t.Fatal("no-op update lost the owner's amount")
	}
	readSeat, err := e.Deals.GetDeal(OnSeat(ownerCtx, principal.SeatRead), id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	requireRecordAffordances(t, readSeat.Writable, readSeat.Archivable, expectedRecordActions{writable: true})
	deleteOnly := asTeam1Member(e, e.Rep1, recordAffordancePerms(recordWriteGrants{deleteGrant: true}))
	if _, err := e.Deals.ArchiveDeal(deleteOnly, id, nil); err != nil {
		t.Fatal(err)
	}
	archived, err := e.Deals.GetDeal(ownerCtx, id, storekit.IncludeArchived)
	if err != nil {
		t.Fatal(err)
	}
	requireRecordAffordances(t, archived.Writable, archived.Archivable, expectedRecordActions{})
	if archived.AmountMinor == nil || *archived.AmountMinor != amount ||
		archived.ExpectedArrMinor == nil || *archived.ExpectedArrMinor != arr ||
		archived.Currency == nil || *archived.Currency != currency || archived.MaskedFields != nil {
		t.Fatal("archiving removed the owner's raw update authority from money masks")
	}
	other, err := e.Deals.GetDeal(asTeam1Member(e, e.Rep3, updatePerms), id, storekit.IncludeArchived)
	if err != nil {
		t.Fatal(err)
	}
	requireRecordAffordances(t, other.Writable, other.Archivable, expectedRecordActions{})
	assertMoneyWithheld(t, other)
}

func TestRecordAffordancesKeepSelfTransferArchiveRefusalInEcho(t *testing.T) {
	e := Setup(t)
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	e.Contacts.SetClock(func() time.Time { return now })
	id := ids.From[ids.ContactKind](e.SeedContact(t, "Transferred contact", &e.Rep2))
	perms := ownRecordArchivePerms()
	ctx := asTeam1Member(e, e.Rep1, perms)
	owner := ids.From[ids.UserKind](e.Rep1)
	echo, err := e.Contacts.UpdateContact(ctx, id, contacts.UpdateContactInput{OwnerID: &owner})
	if err != nil {
		t.Fatal(err)
	}
	requireRecordAffordances(t, echo.Writable, echo.Archivable, expectedRecordActions{writable: true})
	got, err := e.Contacts.GetContact(ctx, id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	requireRecordAffordances(t, got.Writable, got.Archivable, expectedRecordActions{writable: true})
	if _, err := e.Contacts.ArchiveContact(ctx, id, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("self-transfer archive = %v, want permission denied", err)
	}
}

func TestRecordAffordancesReadArchiveStateAfterProjection(t *testing.T) {
	e := Setup(t)
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	e.Deals.SetClock(func() time.Time { return now })
	pipeline, open, _ := DealFixture(t, e)
	id := ids.From[ids.DealKind](e.SeedDeal(t, "Archive between reads", pipeline, open, &e.Rep1))
	ctx := asTeam1Member(e, e.Rep1, recordAffordancePerms(recordWriteGrants{update: true, deleteGrant: true}))
	stale, err := e.Deals.GetDeal(ctx, id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	requireRecordAffordances(t, stale.Writable, stale.Archivable, expectedRecordActions{writable: true, archivable: true})
	if stale.ArchivedAt != nil {
		t.Fatal("fixture already archived")
	}
	err = database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var one int
		if err := tx.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
			return err
		}
		if one != 1 {
			t.Fatal("transaction did not return its sentinel")
		}
		// Commit on another connection before stamping the earlier projection.
		if _, err := e.Deals.ArchiveDeal(e.Admin(), id, nil); err != nil {
			return err
		}
		return requireFreshArchiveAffordances(ctx, t, tx, stale)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func requireFreshArchiveAffordances(ctx context.Context, t *testing.T, tx pgx.Tx, stale crmcontracts.Deal) error {
	t.Helper()
	rows := []crmcontracts.Deal{stale}
	metered, meter := database.WithStatementMeter(ctx)
	raw, err := auth.StampRecordAffordances(metered, tx, auth.DealArchive, rows,
		func(d crmcontracts.Deal) ids.UUID { return ids.UUID(d.Id) },
		func(d *crmcontracts.Deal, may bool) { d.Writable = &may },
		func(d *crmcontracts.Deal, may bool) { d.Archivable = &may })
	if err != nil {
		return err
	}
	requireRecordAffordances(t, rows[0].Writable, rows[0].Archivable, expectedRecordActions{})
	if !raw[ids.UUID(stale.Id)] {
		t.Error("archive removed raw update authority")
	}
	if cost := meter.Cost(); cost.Statements != 2 || cost.Failed != 0 {
		t.Errorf("bounded archived authority cost = %+v, want one authority and one archive-state statement", cost)
	}
	return nil
}

func TestRecordAffordancesEmptyPageKeepsANonNilMaskMap(t *testing.T) {
	e := Setup(t)
	ctx := asTeam1Member(e, e.Rep1, recordAffordancePerms(recordWriteGrants{update: true}))
	rows := []crmcontracts.Deal{}
	raw, err := auth.StampRecordAffordances(ctx, nil, auth.DealArchive, rows,
		func(d crmcontracts.Deal) ids.UUID { return ids.UUID(d.Id) },
		func(d *crmcontracts.Deal, may bool) { d.Writable = &may },
		func(d *crmcontracts.Deal, may bool) { d.Archivable = &may })
	if err != nil {
		t.Fatal(err)
	}
	if raw == nil || len(raw) != 0 {
		t.Fatalf("empty mask authority = %v, want a non-nil empty map", raw)
	}
}
