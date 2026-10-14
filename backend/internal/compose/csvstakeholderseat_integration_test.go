// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
)

func TestAContactAlreadySeatedOnADealIsNotSeatedAgainWhateverTheCallerCanListOf(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Seated once", pipeline, stage, nil)
	contact := e.SeedContact(t, "Already seated", nil)
	w := &csvWriters{pool: e.DB().Pool(), contacts: contacts.NewStore(e.DB())}
	typed := map[string]any{dealTargetContactRef: contact.String()}
	ctx := e.Admin()
	if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		return w.seatStakeholder(ctx, tx, deal, typed, "crm:deal:D-1")
	}); err != nil {
		t.Fatalf("seating the contact: %v", err)
	}
	e.MakeCapturePrivate(t, "contact", contact, e.Rep3)
	again, err := w.stakeholderToSeat(ctx, deal, typed)
	if err != nil || again != nil {
		t.Fatalf("stakeholderToSeat = %v, %v, want nothing to add: the seat is held, whether or not the caller's scope lists its edge", again, err)
	}
}
