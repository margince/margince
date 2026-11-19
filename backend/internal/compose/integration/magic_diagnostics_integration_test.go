// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/compose/magic"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/auditentity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// The unplaceable count is every machine action in the installation that no arm
// can place, not the reader's own, so it reaches only a diagnostics seat.
func TestTheUnplaceableCountReachesOnlyADiagnosticsSeat(t *testing.T) {
	e := Setup(t)
	since := clocktest.Now(t).Add(-time.Hour)
	seedUnplaceableMachineAction(t, e)
	svc := magic.NewService(e.Pool, nil, func() time.Time { return clocktest.Now(t) })

	rep, err := svc.Read(e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms), &since, 20)
	if err != nil {
		t.Fatalf("reading the receipt as a rep: %v", err)
	}
	if count, ok := unknownEntityCount(rep); ok {
		t.Fatalf("a seat without ai_diagnostics read was told %d workspace-wide unplaceable actions", count)
	}

	if rep.NotShownCountWithheld == nil || !*rep.NotShownCountWithheld {
		t.Fatal("restricted receipt must state its unsupported counts are withheld")
	}

	diagnostics := RepPerms
	diagnostics.Objects = maps.Clone(RepPerms.Objects)
	diagnostics.Objects["ai_diagnostics"] = principal.ObjectGrant{Read: true}
	diag, err := svc.Read(e.As(e.Rep1, []ids.UUID{e.Team1}, diagnostics), &since, 20)
	if err != nil {
		t.Fatalf("reading the receipt as a diagnostics seat: %v", err)
	}
	if diag.NotShownCountWithheld == nil || *diag.NotShownCountWithheld {
		t.Fatal("diagnostics receipt must state its counts were read")
	}
	if count, ok := unknownEntityCount(diag); !ok || count != 1 {
		t.Fatalf("a diagnostics seat must count only the update, excluding creation and archive bookkeeping: %+v", diag.NotShown)
	}
}

// A product price change is business work even while the receipt cannot place it.
func seedUnplaceableMachineAction(t *testing.T, e *Env) {
	t.Helper()
	ctx := principal.SystemActing(e.Admin(), "catalog-sync")
	store := deals.NewStore(e.DB(), installseam.Deals())
	product, err := store.CreateProduct(ctx, deals.CreateProductInput{Name: "Support", UnitPriceMinor: 100, Currency: "EUR"})
	if err != nil {
		t.Fatal(err)
	}
	price := int64(200)
	if _, err := store.UpdateProduct(ctx, ids.From[ids.ProductKind](ids.UUID(product.Id)), deals.UpdateProductInput{UnitPriceMinor: &price}); err != nil {
		t.Fatal(err)
	}
}

func TestInfrastructureUpdatesDoNotMakeAQuietReceiptIncomplete(t *testing.T) {
	e := Setup(t)
	ctx := principal.SystemActing(e.Admin(), "bootstrap")
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		for _, kind := range []string{"knowledge_document", "ai_model_rate", "tag", "list"} {
			if _, err := storekit.Audit(ctx, tx, "update", kind, ids.NewV7(), map[string]string{"state": "old"}, map[string]string{"state": "current"}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	forecast := forecasting.NewStore(e.DB())
	for range 2 {
		if err := forecast.RecordCaptureStatus(ctx, forecasting.Scope{Kind: forecasting.ScopeWorkspace}, nil, clocktest.Now(t), false, textlang.English); err != nil {
			t.Fatal(err)
		}
	}
	svc := magic.NewService(e.Pool, nil, func() time.Time { return clocktest.Now(t) })
	receipt, err := svc.Read(e.Admin(), nil, 20)
	if err != nil {
		t.Fatal(err)
	}
	if count, _ := unknownEntityCount(receipt); count != 0 {
		t.Fatalf("maintenance became unsupported CRM work: %+v", receipt.NotShown)
	}
	if len(receipt.Done) != 0 {
		t.Fatalf("maintenance became completed CRM work: %+v", receipt.Done)
	}
	seedUnplaceableMachineAction(t, e)
	receipt, err = svc.Read(e.Admin(), nil, 20)
	if err != nil {
		t.Fatal(err)
	}
	if count, ok := unknownEntityCount(receipt); !ok || count != 1 {
		t.Fatalf("unsupported CRM change disappeared with maintenance: %+v", receipt.NotShown)
	}
	// Unserved business subjects and future audit kinds must still refuse completeness.
	err = database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		for _, kind := range []string{"contract", "finance_invoice", "finance_payment", "scheduled_send", "future_crm_record"} {
			if _, err := storekit.Audit(ctx, tx, "update", kind, ids.NewV7(), map[string]string{"state": "old"}, map[string]string{"state": "current"}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err = svc.Read(e.Admin(), nil, 20)
	if err != nil {
		t.Fatal(err)
	}
	if count, ok := unknownEntityCount(receipt); !ok || count != 6 {
		t.Fatalf("unsupported business or unknown subjects disappeared: %+v", receipt.NotShown)
	}
}

// A list membership and a tagging are business work under a maintenance subject.
// The breakdown names them; the list's and tag's own edits stay upkeep.
func TestTheUnplacedBreakdownNamesLinksAndAddsUpToTheReceipt(t *testing.T) {
	e := Setup(t)
	since := clocktest.Now(t).Add(-time.Hour)
	seedUnplaceableMachineAction(t, e)
	seedMachineLinkChanges(t, e)
	svc := magic.NewService(e.Pool, nil, func() time.Time { return clocktest.Now(t) })

	changes, err := svc.UnplacedChanges(e.Admin(), since)
	if err != nil {
		t.Fatalf("reading the breakdown as an admin: %v", err)
	}
	got := map[string]int{}
	for _, change := range changes {
		got[change.EntityType+"/"+change.Action] = change.Count
	}
	want := map[string]int{"product/update": 1, "list/added": 1, "list/removed": 1, "tag/applied": 1, "tag/removed": 1}
	if !maps.Equal(got, want) {
		t.Fatalf("breakdown %v, want %v", got, want)
	}
	receipt, err := svc.Read(e.Admin(), &since, 20)
	if err != nil {
		t.Fatal(err)
	}
	if count, ok := unknownEntityCount(receipt); !ok || count != magic.UnplacedTotal(changes) {
		t.Fatalf("receipt counts %+v, breakdown sums to %d", receipt.NotShown, magic.UnplacedTotal(changes))
	}

	if _, err := svc.UnplacedChanges(e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms), since); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a seat without ai_diagnostics read was given the breakdown: %v", err)
	}
}

// seedMachineLinkChanges writes, as a machine and through the real writers, a
// membership and a tagging made and undone, both renames and a document edit.
func seedMachineLinkChanges(t *testing.T, e *Env) {
	t.Helper()
	ctx := principal.SystemActing(e.Admin(), "segment-sync")
	store := collections.NewStore(e.DB())
	contact := e.SeedContact(t, "Linked Contact", &e.Rep1)
	list, err := store.CreateList(e.Admin(), collections.CreateListInput{Name: "Synced", EntityType: "contact", Sharing: "workspace"})
	if err != nil {
		t.Fatal(err)
	}
	change := collections.MemberChange{EntityType: "contact", EntityID: contact, Reason: collections.ReasonAutomation}
	if _, err := store.AddMember(ctx, list.ID, change); err != nil {
		t.Fatal(err)
	}
	renamed := "Synced weekly"
	if _, err := store.UpdateList(ctx, list.ID, collections.UpdateListInput{Name: &renamed, IfVersion: &list.Version}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RemoveMember(ctx, list.ID, change); err != nil {
		t.Fatal(err)
	}
	tag, err := store.NewTag(e.Admin(), "synced", "")
	if err != nil {
		t.Fatal(err)
	}
	tagID := ids.From[ids.TagKind](tag.TagID)
	if _, err := store.ApplyTag(ctx, tagID, "contact", contact); err != nil {
		t.Fatal(err)
	}
	weekly := "synced weekly"
	if _, err := store.UpdateTag(ctx, tagID, collections.TagUpdate{Name: &weekly}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RemoveTag(ctx, tagID, "contact", contact); err != nil {
		t.Fatal(err)
	}
	err = database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := storekit.Audit(ctx, tx, "update", auditentity.KnowledgeDocument, ids.NewV7(), map[string]string{"state": "old"}, map[string]string{"state": "current"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func unknownEntityCount(receipt crmcontracts.MagicReceipt) (int, bool) {
	for _, entry := range receipt.NotShown {
		if entry.Reason == crmcontracts.MagicNotShownReasonMagicNotShownUnknownEntityType {
			return entry.Count, true
		}
	}
	return 0, false
}
