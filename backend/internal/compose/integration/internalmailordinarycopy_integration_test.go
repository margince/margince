// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestInternalMailboxCopyDoesNotReclassifyCustomerMail(t *testing.T) {
	ctx, connecting, vault, ws, pool := newInternalMailRegistryFixture(t)
	connection, err := connecting.Connect(ctx, "gmail", connector.Auth("fixture-token"))
	if err != nil {
		t.Fatal(err)
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing fixture actor")
	}
	actor.SeatType = principal.SeatFull
	actor.Permissions = principal.Permissions{RoleKeys: []string{"admin"}, RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{"capture_settings": {Read: true, Update: true}}}
	admin := principal.WithActor(ctx, actor)
	seedInternalMailAnchor(admin, t, pool, actor.UserID)
	db := database.BindTo(pool, ws)
	sink := capture.NewSink(db).WithInternalMailRecorder(activities.MarkInternalMailTx).WithAudienceRecompute(activities.RecomputeAudienceTx)
	registry := capture.NewRegistry(db, sink, internalMailAuthority{}, vault)
	record := internalHistoryRecord("external-bcc-copy", time.Time{})
	record.DeliveredTo = fixtureOwnerEmail
	record.Addresses = append(record.Addresses, "customer@outside.test")
	record.Raw = []byte("From: colleague@example.test\r\nTo: " + fixtureOwnerEmail + "\r\nBcc: customer@outside.test\r\nSubject: Salary review\r\n\r\nBody.")
	var captured []ids.UUID
	registry.Register(operationalMailbox{provider: "gmail", label: fixtureOwnerEmail, capture: func(runCtx context.Context, target connector.Sink) error {
		ref, err := target.Upsert(runCtx, record)
		if err == nil {
			captured = append(captured, ref.ID)
		}
		return err
	}})
	raw, cfg := configtest.StoredOverWith(t, pool, capture.Definitions(), capture.ConfigSettings())
	settings := capture.NewSettings(raw, cfg)
	policy, err := settings.SetInternalMailPolicy(admin, true, []capture.InternalMailSelection{{ConnectionID: connection}})
	if err != nil {
		t.Fatal(err)
	}
	record.ProviderReceivedAt = policy.Sources[0].EnrolledAt.Add(time.Second)
	if err := registry.SyncOnce(ctx, connection); err != nil {
		t.Fatal(err)
	}
	if len(captured) != 1 {
		t.Fatalf("initial capture=%v", captured)
	}
	if err := database.WithWorkspaceTx(admin, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(admin, `UPDATE activity SET audience='selected',audience_reason='manual' WHERE id=$1`, captured[0])
		if err != nil {
			return err
		}
		_, err = tx.Exec(admin, `INSERT INTO activity_audience_member(activity_id,subject_type,subject_id,created_by) VALUES ($1,'user',$2,$3)`, captured[0], actor.UserID, actor.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	record.Addresses = []string{"colleague@example.test", fixtureOwnerEmail}
	record.Raw = []byte("From: colleague@example.test\r\nTo: " + fixtureOwnerEmail + "\r\nSubject: Salary review\r\n\r\nBody.")
	if err := registry.SyncOnce(ctx, connection); err != nil {
		t.Fatal(err)
	}
	if len(captured) != 2 || captured[0] != captured[1] {
		t.Fatalf("copy changed identity=%v", captured)
	}
	if err := database.WithWorkspaceTx(admin, pool, func(tx pgx.Tx) error {
		var unchanged bool
		err := tx.QueryRow(admin, `SELECT internal_mail_authorization IS NULL AND audience='selected' AND audience_reason='manual'
   AND EXISTS (SELECT 1 FROM activity_audience_member WHERE activity_id=activity.id AND subject_id=$2)
   FROM activity WHERE id=$1`, captured[0], actor.UserID).Scan(&unchanged)
		if err == nil && !unchanged {
			t.Error("later copy changed the existing customer classification or grants")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
