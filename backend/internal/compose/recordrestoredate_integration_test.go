// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Putting back a date-typed field.
//
// Postgres renders a `date` column as "2026-12-01"; a Go time.Time renders as
// "2026-12-01T00:00:00Z". Both name the same day, and an audit image written in
// the second spelling is compared against a live row read in the first — so the
// field reads as MOVED the instant it is written, and Undo refuses a change
// nobody has touched, blaming a supersession that never happened.
//
// This is the whole of the defect, and it needs the real trail: the store's own
// write, the audit row it recorded, and the restore seam's comparison against
// the live row. A unit test over the comparison alone would need a hand-built
// image, which is the spelling under test.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestADateFieldGoesBackWhereItWas(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	pipeline, open, _ := integration.DealFixture(t, e)

	was := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	deal, err := e.Deals.CreateDeal(ctx, deals.CreateDealInput{
		Name: "Dated", PipelineID: pipeline, StageID: open, Source: "manual",
		ExpectedClose: &was,
	})
	if err != nil {
		t.Fatalf("seed the deal through the real writer: %v", err)
	}
	id := ids.UUID(deal.Id)
	dealID := ids.From[ids.DealKind](id)

	moved := time.Date(2027, 3, 15, 0, 0, 0, 0, time.UTC)
	if _, err := e.Deals.UpdateDeal(ctx, dealID, deals.UpdateDealInput{ExpectedClose: &moved}); err != nil {
		t.Fatalf("move the close date: %v", err)
	}
	entry := latestAuditRowID(t, e, "deal", id, "update")

	seam := NewRestoreSeam(e.Pool, NewDispatcher(NewProvider(e.Pool),
		NewOverlayProvider(e.Pool, failClosedOverlayMeter(), nil), e.Pool))
	if _, err := seam.Restore(ctx, "deal", id, entry, currentVersion(t, e, "deal", id)); err != nil {
		t.Fatalf("putting the close date back answered %v — nothing moved under it, "+
			"the audit image and the live row are just spelling the same day differently", err)
	}

	after, err := e.Deals.GetDeal(ctx, dealID, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the deal back: %v", err)
	}
	if after.ExpectedCloseDate == nil {
		t.Fatal("the close date came back empty")
	}
	if got := after.ExpectedCloseDate.Format(time.DateOnly); got != was.Format(time.DateOnly) {
		t.Errorf("the close date is %s, want the %s it held before the change",
			got, was.Format(time.DateOnly))
	}
}

// A link's dates are the same defect on a different write path.
//
// A relationship's image is built by hand rather than through a Patch, so a fix
// that only reaches Patch call sites leaves this one broken — and its reverse
// parses the image back, so the image's spelling and the parser's layout are one
// change.
func TestALinksDateGoesBackWhereItWas(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()

	personID := e.SeedPerson(t, "Ada Dated", nil)
	orgID := e.SeedOrg(t, "Dated GmbH", nil)
	person := ids.From[ids.PersonKind](personID)
	org := ids.From[ids.OrganizationKind](orgID)
	role := "cto"
	edge, err := e.People.CreateRelationship(ctx, people.CreateRelationshipInput{
		Kind: "employment", PersonID: &person, OrganizationID: &org,
		Role: &role, Source: "manual",
	})
	if err != nil {
		t.Fatalf("seed the link through the real writer: %v", err)
	}

	was := time.Date(2024, 5, 6, 0, 0, 0, 0, time.UTC)
	if _, err := e.People.UpdateRelationship(ctx, edge.ID,
		people.UpdateRelationshipInput{StartedAt: &was}); err != nil {
		t.Fatalf("set the link's start date: %v", err)
	}
	moved := time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC)
	if _, err := e.People.UpdateRelationship(ctx, edge.ID,
		people.UpdateRelationshipInput{StartedAt: &moved}); err != nil {
		t.Fatalf("move the link's start date: %v", err)
	}
	auditID := latestAuditRowID(t, e, edgeEntityType, edge.ID, "update")

	if _, err := restoreSeamFor(e).Restore(ctx, "person", personID, auditID,
		currentVersion(t, e, "person", personID)); err != nil {
		t.Fatalf("putting the link's start date back answered %v — nothing moved "+
			"under it, the image and the live row are spelling the same day differently", err)
	}

	var back *time.Time
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT started_at FROM relationship WHERE id = $1`,
			edge.ID).Scan(&back)
	}); err != nil {
		t.Fatalf("reading the link back: %v", err)
	}
	if back == nil {
		t.Fatal("the link's start date came back empty")
	}
	if got := back.Format(time.DateOnly); got != was.Format(time.DateOnly) {
		t.Errorf("the link starts %s, want the %s it held before the change",
			got, was.Format(time.DateOnly))
	}
}
