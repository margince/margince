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

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

type operationalMailbox struct {
	internalMailConnector
	provider, label string
	capture         func(context.Context, connector.Sink) error
}

func (m operationalMailbox) Descriptor() connector.Descriptor {
	return connector.Descriptor{Name: m.provider, Version: "fixture"}
}
func (m operationalMailbox) AccountLabel(connector.Auth) (string, error) { return m.label, nil }
func (m operationalMailbox) Sync(ctx context.Context, _ connector.Auth, cursor connector.Cursor, sink connector.Sink) (connector.Cursor, error) {
	return cursor, m.capture(ctx, sink)
}

func TestAnInternalCopyInASharedMailboxRetainsItsRestrictionAndAdmitsOnlyAnActualParticipant(t *testing.T) {
	ctx, connecting, vault, ws, pool := newInternalMailRegistryFixture(t)
	first, err := connecting.Connect(ctx, "gmail", connector.Auth("fixture-token"))
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
	secondUser := ids.NewV7()
	if err := database.WithWorkspaceTx(admin, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(admin, `INSERT INTO app_user (id, email, display_name, status) VALUES ($1, 'teammate@example.test', 'Teammate', 'active')`, secondUser)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	secondActor := actor
	secondActor.ID, secondActor.UserID = "human:"+secondUser.String(), secondUser
	secondCtx := principal.WithActor(ctx, secondActor)
	db := database.BindTo(pool, ws)
	sink := capture.NewSink(db).WithInternalMailRecorder(activities.MarkInternalMailTx).WithAudienceRecompute(activities.RecomputeAudienceTx)
	reg := capture.NewRegistry(db, sink, internalMailAuthority{}, vault)
	record := internalHistoryRecord("overlapping-copy", time.Time{})
	record.Addresses = append(record.Addresses, "teammate@example.test")
	record.Raw = []byte("From: colleague@example.test\r\nTo: " + fixtureOwnerEmail + ", teammate@example.test\r\nSubject: Salary review\r\n\r\nBody.")
	var captured []datasource.EntityRef
	land := func(owner, provider string) func(context.Context, connector.Sink) error {
		return func(runCtx context.Context, target connector.Sink) error {
			mailCopy := record
			mailCopy.DeliveredTo, mailCopy.CapturedBy, mailCopy.Source = owner, "connector:"+provider, provider+":operational"
			ref, err := target.Upsert(runCtx, mailCopy)
			if err == nil {
				captured = append(captured, ref)
			}
			return err
		}
	}
	reg.Register(operationalMailbox{provider: "gmail", label: fixtureOwnerEmail, capture: land(fixtureOwnerEmail, "gmail")})
	reg.Register(operationalMailbox{provider: "graph", label: "teammate@example.test", capture: land("teammate@example.test", "graph")})
	second, err := reg.Connect(secondCtx, "graph", connector.Auth("second-fixture-token"))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.WithWorkspaceTx(admin, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(admin, `UPDATE capture_connection SET mail_posture = 'shared' WHERE id = $1`, second)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	raw, cfg := configtest.StoredOverWith(t, pool, capture.Definitions(), capture.ConfigSettings())
	settings := capture.NewSettings(raw, cfg)
	policy, err := settings.SetInternalMailPolicy(admin, true, []capture.InternalMailSelection{{ConnectionID: first}, {ConnectionID: second}})
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range policy.Sources {
		if source.EnrolledAt.After(record.ProviderReceivedAt) {
			record.ProviderReceivedAt = source.EnrolledAt
		}
	}
	record.ProviderReceivedAt = record.ProviderReceivedAt.Add(time.Second)
	if err := reg.SyncOnce(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := reg.SyncOnce(secondCtx, second); err != nil {
		t.Fatal(err)
	}
	if len(captured) != 2 || captured[0].ID != captured[1].ID {
		t.Fatalf("copies did not resolve to the same restricted message: %+v", captured)
	}
	if err := database.WithWorkspaceTx(admin, pool, func(tx pgx.Tx) error {
		var audience, reason string
		var imports int
		if err := tx.QueryRow(admin, `SELECT audience, audience_reason FROM activity WHERE id = $1`, captured[0].ID).Scan(&audience, &reason); err != nil {
			return err
		}
		if err := tx.QueryRow(admin, `SELECT count(*) FROM capture_import WHERE activity_id = $1`, captured[0].ID).Scan(&imports); err != nil {
			return err
		}
		if audience != "participants" || reason != activities.ReasonInternalOperational || imports != 2 {
			t.Errorf("audience=%s reason=%s imports=%d", audience, reason, imports)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	forger := ids.NewV7()
	if err := database.WithWorkspaceTx(admin, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(admin, `INSERT INTO app_user (id, email, display_name, status) VALUES ($1, 'other@example.test', 'Other', 'active')`, forger)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	forgedActor := actor
	forgedActor.UserID, forgedActor.ID = forger, "human:"+forger.String()
	forgedActor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Update: true}
	forgedCtx := principal.WithActor(ctx, forgedActor)
	reg.Register(operationalMailbox{provider: "imap", label: "other@example.test", capture: land("other@example.test", "imap")})
	forgedConnection, err := reg.Connect(forgedCtx, "imap", connector.Auth("other-fixture-token"))
	if err != nil {
		t.Fatal(err)
	}
	selections := policySelections(policy)
	selections = append(selections, capture.InternalMailSelection{ConnectionID: forgedConnection})
	policy, err = settings.SetInternalMailPolicy(admin, true, selections)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range policy.Sources {
		if source.EnrolledAt.After(record.ProviderReceivedAt) {
			record.ProviderReceivedAt = source.EnrolledAt
		}
	}
	record.ProviderReceivedAt = record.ProviderReceivedAt.Add(time.Second)
	fields, ok := record.Fields.(capture.ActivityFields)
	if !ok {
		t.Fatal("fixture has no mail fields")
	}
	fields.Body = "A different message with a copied identifier."
	record.Fields = fields
	record.Addresses = append(record.Addresses, "other@example.test")
	record.Raw = []byte("From: colleague@example.test\r\nTo: other@example.test\r\nSubject: Salary review\r\n\r\nDifferent body.")
	if err := reg.SyncOnce(forgedCtx, forgedConnection); err != nil {
		t.Fatal(err)
	}
	if len(captured) != 3 || captured[2].ID == captured[0].ID {
		t.Fatalf("copied identifier gained the original: %+v", captured)
	}
	activityStore := activities.NewStore(db)
	if _, err := activityStore.GetActivity(forgedCtx, ids.From[ids.ActivityKind](captured[0].ID), storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("forged copy can read original: %v", err)
	}
}
