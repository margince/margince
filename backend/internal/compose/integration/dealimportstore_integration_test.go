// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The deals store's two doors an import uses, asked directly: who may reach
// them, and what the archive re-asks under its own lock.

import (
	"context"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestOnlyADeclaredImporterReachesTheImportBirth(t *testing.T) {
	e := Setup(t)
	pipeline, _, won := DealFixture(t, e)
	closed := time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC)
	birth := func(ctx context.Context) error {
		return e.Deals.Tx(ctx, func(tx pgx.Tx) error {
			_, err := e.Deals.ImportDealTx(ctx, tx, deals.CreateDealInput{Name: "Imported", PipelineID: pipeline, ExpectedClose: &closed},
				&deals.ImportTerminal{StageID: won, ClosedAt: &closed}, deals.CustomColumns{})
			return err
		})
	}
	if err := birth(e.Admin()); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a seat without the importer grant → %v, want permission denied: the live admin may not backdate a close", err)
	}
	objects := maps.Clone(AdminPerms.Objects)
	objects["import_run"] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	importer := e.As(e.AdminUser, nil, principal.Permissions{RoleKeys: AdminPerms.RoleKeys, Objects: objects, RowScope: principal.RowScopeAll})
	if err := birth(importer); err != nil {
		t.Fatalf("a declared importer → %v, want the closed deal born", err)
	}
}

func TestTheDealArchiveReAsksUntouchedInsideItsTransaction(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	pipeline, open, _ := DealFixture(t, e)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Imported deal", pipeline, open, nil))
	imported := seedInstant(t)

	description := "A rep called them"
	if _, err := e.Deals.UpdateDeal(admin, deal, deals.UpdateDealInput{Description: &description}); err != nil {
		t.Fatalf("the human edit: %v", err)
	}
	var touched *deals.HumanTouchedError
	if _, err := e.Deals.ArchiveDeal(admin, deal, nil, deals.NotTouchedByHumanSince(imported)); !errors.As(err, &touched) {
		t.Fatalf("archiving a deal a rep edited → %v, want the touched refusal", err)
	}
	if !errors.Is(touched, apperrors.ErrConflict) {
		t.Errorf("the refusal = %v, want a conflict so an undo records the deal as kept rather than failing", touched)
	}
	kept, err := e.Deals.GetDeal(admin, deal, storekit.LiveOnly)
	if err != nil || kept.ArchivedAt != nil {
		t.Fatalf("deal after the refusal = %+v, %v, want it live", kept, err)
	}
	if _, err := e.Deals.ArchiveDeal(admin, deal, nil); err != nil {
		t.Errorf("archiving with no precondition → %v, want the behaviour every other caller has", err)
	}
}
