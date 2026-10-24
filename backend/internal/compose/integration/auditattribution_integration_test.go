// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// AuditAttributionsTx answers "who acted" for a stored audit link.
// It must agree with the record History on every actor shape, because the two
// are the same fact shown on two screens.

import (
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAuditAttributionsMatchTheRecordHistoryForEveryActorShape(t *testing.T) {
	e := Setup(t)
	contact := e.SeedContact(t, "History Subject", nil)
	human := seedWorkspaceUser(t, e, "Ada Authority")
	at := clocktest.Now(t).Add(time.Hour).UTC().Truncate(time.Microsecond)

	seedDelegatedAuditRow(t, e, contact, human, seedGrantedPassport(t, e, human, "Claude"), at)
	seedDelegatedAuditRow(t, e, contact, human, seedUngrantedPassport(t, e, human), at.Add(time.Minute))

	page := readRecordHistory(t, e, contact)
	var auditIDs []ids.UUID
	for _, entry := range page.Entries {
		auditIDs = append(auditIDs, entry.ID)
	}
	var got map[ids.UUID]privacy.AuditAttribution
	ctx := principal.WithWorkspaceID(t.Context(), e.WS)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		got, err = privacy.AuditAttributionsTx(ctx, tx, "contact", contact, auditIDs)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(page.Entries) || len(page.Entries) < 3 {
		t.Fatalf("resolved %d attributions for %d history entries, want one each and at least 3", len(got), len(page.Entries))
	}
	for _, entry := range page.Entries {
		want := privacy.AuditAttribution{
			ActorType: entry.ActorType, ActorID: entry.ActorID, ActorName: entry.ActorName,
			OnBehalfOf: entry.OnBehalfOf, OnBehalfOfName: entry.OnBehalfOfName,
			PassportID: entry.PassportID, AgentClient: entry.AgentClient,
		}
		if !reflect.DeepEqual(got[entry.ID], want) {
			t.Errorf("entry %s (%s): attribution %+v differs from the record history's %+v", entry.ID, entry.Action, got[entry.ID], want)
		}
	}
}
