// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/magic"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The unplaceable count is every machine action in the installation that no arm
// can place, not the reader's own, so it reaches only a diagnostics seat.
func TestTheUnplaceableCountReachesOnlyADiagnosticsSeat(t *testing.T) {
	e := Setup(t)
	since := time.Now().Add(-time.Hour)
	seedUnplaceableMachineAction(t, e)
	svc := magic.NewService(e.Pool, nil, time.Now)

	rep, err := svc.Read(e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms), &since, 20)
	if err != nil {
		t.Fatalf("reading the receipt as a rep: %v", err)
	}
	if count, ok := unknownEntityCount(rep); ok {
		t.Fatalf("a seat without ai_diagnostics read was told %d workspace-wide unplaceable actions", count)
	}

	diagnostics := RepPerms
	diagnostics.Objects = maps.Clone(RepPerms.Objects)
	diagnostics.Objects["ai_diagnostics"] = principal.ObjectGrant{Read: true}
	diag, err := svc.Read(e.As(e.Rep1, []ids.UUID{e.Team1}, diagnostics), &since, 20)
	if err != nil {
		t.Fatalf("reading the receipt as a diagnostics seat: %v", err)
	}
	if count, ok := unknownEntityCount(diag); !ok || count < 1 {
		t.Fatalf("a diagnostics seat was not told of the unplaceable action: %+v", diag.NotShown)
	}
}

// seedUnplaceableMachineAction writes an agent's update to a tag, an entity type
// no arm of the done lane joins.
func seedUnplaceableMachineAction(t *testing.T, e *Env) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, occurred_at)
			VALUES ('agent', 'agent:auto-apply', 'update', 'tag', $1, now())`, ids.NewV7())
		return err
	})
	if err != nil {
		t.Fatalf("seeding the unplaceable machine action: %v", err)
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
