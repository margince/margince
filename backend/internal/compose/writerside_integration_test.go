// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The readers that hand a model a row's side straight from SQL name the side
// the projected voice names, on rows written by the real capture, import and
// logging paths.
func TestTheStoredWriterSideIsTheProjectedVoicesSide(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Alma Buyer", "buyer@alpha.test")
	importer := o.e.As(o.e.AdminUser, nil, importerPerms())
	status, imported := postActivity(importer, t, o.e, crmcontracts.CreateActivityRequest{
		Source: "manual", Kind: "email", Subject: new("Pricing"), Body: new(ourAnswer),
		Direction:    new(crmcontracts.CreateActivityRequestDirectionInbound),
		SourceSystem: new("mail_import"), SourceId: new("writer-side-import"), SourceAuthorId: new(openapi_types.UUID(o.e.Rep2)),
	})
	if status != http.StatusCreated {
		t.Fatalf("import status = %d", status)
	}
	logger := o.e.As(o.e.Rep1, []ids.UUID{o.e.Team1}, integration.AdminPerms)
	call, _, err := o.e.Activities.LogActivity(logger, activities.LogActivityInput{
		Kind: "call", Subject: new("Rang about pricing"), Direction: new("inbound"),
	})
	if err != nil {
		t.Fatalf("logging the call: %v", err)
	}
	want := map[string]struct {
		row  ids.UUID
		side string
	}{
		"a contact's mail":              {o.capture(t, pricingMail("buyer@alpha.test", o.seat, pricingRoot, alphaAsks, 0, true)), "inbound"},
		"our reply":                     {o.capture(t, pricingMail(o.seat, "buyer@alpha.test", "ours-1@ws.example", ourAnswer, 10, true)), "outbound"},
		"a stranger's mail":             {o.capture(t, pricingMail("someone@zeta.test", o.seat, "zeta-1@zeta.test", betaWrites, 40, false)), "inbound"},
		"an imported colleague's mail":  {ids.UUID(imported.Id), "outbound"},
		"an inbound call a seat logged": {ids.UUID(call.Id), "inbound"},
	}
	for name, c := range want {
		row, err := o.e.Activities.GetActivity(logger, ids.From[ids.ActivityKind](c.row), storekit.LiveOnly)
		if err != nil {
			t.Fatalf("%s: reading the row: %v", name, err)
		}
		var stored string
		if err := database.WithWorkspaceTx(logger, o.e.Pool, func(tx pgx.Tx) error {
			return tx.QueryRow(context.Background(),
				`SELECT coalesce(`+activities.WriterSideSQL("a")+`, '') FROM activity a WHERE a.id = $1`, c.row).Scan(&stored)
		}); err != nil {
			t.Fatalf("%s: reading the stored side: %v", name, err)
		}
		if projected := activities.WriterSide(row); projected != c.side || stored != c.side {
			t.Errorf("%s: projected side %q, stored side %q, want %q", name, projected, stored, c.side)
		}
	}
}
