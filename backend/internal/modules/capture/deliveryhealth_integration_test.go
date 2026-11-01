// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type deliveryConnector struct {
	fixtureConnector
	deliver    *bool
	failed     *bool
	after      func() error
	account    *string
	receivedAt *time.Time
}

func (c deliveryConnector) Sync(ctx context.Context, _ connector.Auth, cursor connector.Cursor, sink connector.Sink) (connector.Cursor, error) {
	if *c.deliver {
		rec := mailRecord("delivery-health-replay", fixtureOwnerEmail, fixtureOwnerEmail)
		rec.CapturedBy = "connector:gmail"
		rec.ProviderReceivedAt = *c.receivedAt
		if _, err := sink.Upsert(ctx, rec); err != nil && !errors.Is(err, connector.ErrSkip) {
			return nil, err
		}
	}
	if c.after != nil {
		if err := c.after(); err != nil {
			return nil, err
		}
	}
	if *c.failed {
		return nil, connector.ErrUnreachable
	}
	return cursor, nil
}

func TestSuccessfulEmailDeliveryDatesSilenceAndDismissal(t *testing.T) {
	ctx, _, vault, ws := newCaptureRegistryFixture(t)
	owner, pool := setupCaptureDB(t)
	db := database.BindTo(pool, ws)
	at := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	receivedAt := at
	reg := capture.NewRegistry(db, capture.NewSink(db), fixtureAuthority{}, vault)
	delivered, failed := true, false
	reg.Register(deliveryConnector{receivedAt: &receivedAt, deliver: &delivered, failed: &failed})
	id, err := reg.Connect(ctx, "gmail", connector.Auth("fixture-token"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `UPDATE capture_connection SET connected_at = $1 WHERE id = $2`, at.Add(-time.Hour), id); err != nil {
		t.Fatal(err)
	}
	reg.WithClock(func() time.Time { return at })
	if err := reg.SyncOnce(ctx, id); err != nil {
		t.Fatal(err)
	}
	views, err := reg.Connections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if views[0].LastDeliveryAt == nil || !views[0].LastDeliveryAt.Equal(at) {
		t.Fatalf("delivery = %v", views[0].LastDeliveryAt)
	}
	first := at
	delivered = false
	at = at.Add(96 * time.Hour)
	if err := reg.SyncOnce(ctx, id); err != nil {
		t.Fatal(err)
	}
	concerns, err := reg.HealthConcerns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(concerns) != 1 || concerns[0].Kind != capture.ConcernCaptureSilent || !concerns[0].FailingSince.Equal(first) || concerns[0].ChangedAt == nil || !concerns[0].ChangedAt.Equal(first) {
		t.Fatalf("concerns = %+v", concerns)
	}
	if _, err := reg.DismissSilence(ctx, "gmail", first); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.DismissSilence(ctx, "gmail", first); err != nil {
		t.Fatalf("repeated dismissal: %v", err)
	}
	concerns, err = reg.HealthConcerns(ctx)
	if err != nil || len(concerns) != 0 {
		t.Fatalf("dismissed concern = %+v, %v", concerns, err)
	}
	delivered = true
	at = at.Add(time.Hour)
	if err := reg.SyncOnce(ctx, id); err != nil {
		t.Fatal(err)
	}
	replayed, err := reg.Connections(ctx)
	if err != nil || replayed[0].LastDeliveryAt == nil || !replayed[0].LastDeliveryAt.Equal(first) || replayed[0].Concern != "" {
		t.Fatalf("replayed mail reopened silence: %+v, %v", replayed, err)
	}
	at = at.Add(time.Hour)
	receivedAt = at
	if err := reg.SyncOnce(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.DismissSilence(ctx, "gmail", first); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("stale dismissal = %v", err)
	}
	delivered = false
	at = at.Add(96 * time.Hour)
	if err := reg.SyncOnce(ctx, id); err != nil {
		t.Fatal(err)
	}
	concerns, err = reg.HealthConcerns(ctx)
	if err != nil || len(concerns) != 1 || concerns[0].Kind != capture.ConcernCaptureSilent {
		t.Fatalf("new silence = %+v, %v", concerns, err)
	}
	var count int
	if err := owner.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND after ? 'silence_dismissed_since'`, id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("dismissal audits = %d, want one", count)
	}
}

func TestFailedPullDoesNotCommitDelivery(t *testing.T) {
	ctx, _, vault, ws := newCaptureRegistryFixture(t)
	_, pool := setupCaptureDB(t)
	db := database.BindTo(pool, ws)
	at := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	receivedAt := at
	reg := capture.NewRegistry(db, capture.NewSink(db), fixtureAuthority{}, vault)
	delivered, failed := true, true
	reg.Register(deliveryConnector{receivedAt: &receivedAt, deliver: &delivered, failed: &failed})
	id, err := reg.Connect(ctx, "gmail", connector.Auth("fixture-token"))
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.SyncOnce(ctx, id); !errors.Is(err, connector.ErrUnreachable) {
		t.Fatalf("pull = %v", err)
	}
	views, err := reg.Connections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if views[0].LastDeliveryAt != nil {
		t.Fatal("failed pull stamped a delivery")
	}
}

func (c deliveryConnector) AccountLabel(auth connector.Auth) (string, error) {
	if c.account != nil {
		return *c.account, nil
	}
	return c.fixtureConnector.AccountLabel(auth)
}

func TestReauthenticationKeepsSilenceDismissalAndRebindClearsIt(t *testing.T) {
	ctx, _, vault, ws := newCaptureRegistryFixture(t)
	owner, pool := setupCaptureDB(t)
	db := database.BindTo(pool, ws)
	at := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	receivedAt := at
	reg := capture.NewRegistry(db, capture.NewSink(db), fixtureAuthority{}, vault)
	delivered, failed := true, false
	account := fixtureOwnerEmail
	reg.Register(deliveryConnector{receivedAt: &receivedAt, deliver: &delivered, failed: &failed, account: &account})
	id, err := reg.Connect(ctx, "gmail", connector.Auth("first-token"))
	if err != nil {
		t.Fatal(err)
	}
	reg.WithClock(func() time.Time { return at })
	if _, err := owner.Exec(ctx, `UPDATE capture_connection SET connected_at = $1 WHERE id = $2`, at.Add(-time.Hour), id); err != nil {
		t.Fatal(err)
	}
	if err := reg.SyncOnce(ctx, id); err != nil {
		t.Fatal(err)
	}
	first := at
	delivered = false
	at = at.Add(96 * time.Hour)
	if err := reg.SyncOnce(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.DismissSilence(ctx, "gmail", first); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Connect(ctx, "gmail", connector.Auth("replacement-token")); err != nil {
		t.Fatal(err)
	}
	views, err := reg.Connections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if views[0].SilenceDismissedSince == nil || views[0].LastDeliveryAt == nil || !views[0].LastDeliveryAt.Equal(first) {
		t.Fatal("same-account reconnect lost its delivery or dismissal")
	}
	account = "other@example.test"
	if _, err := reg.Connect(ctx, "gmail", connector.Auth("other-account")); err != nil {
		t.Fatal(err)
	}
	views, err = reg.Connections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if views[0].SilenceDismissedSince != nil || views[0].LastDeliveryAt != nil {
		t.Fatal("new mailbox inherited the old mailbox's delivery or dismissal")
	}
	if _, err := reg.DismissSilence(ctx, "gmail", first); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("old-account dismissal = %v", err)
	}
}

func TestSupersededPullCannotPublishDelivery(t *testing.T) {
	ctx, _, vault, ws := newCaptureRegistryFixture(t)
	_, pool := setupCaptureDB(t)
	db := database.BindTo(pool, ws)
	at := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	receivedAt := at
	reg := capture.NewRegistry(db, capture.NewSink(db), fixtureAuthority{}, vault)
	delivered, failed := true, false
	reg.Register(deliveryConnector{receivedAt: &receivedAt, deliver: &delivered, failed: &failed, after: func() error { return reg.Disconnect(ctx, "gmail") }})
	id, err := reg.Connect(ctx, "gmail", connector.Auth("fixture-token"))
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.SyncOnce(ctx, id); err != nil {
		t.Fatal(err)
	}
	views, err := reg.Connections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if views[0].LastDeliveryAt != nil || views[0].Status != "disconnected" {
		t.Fatalf("superseded delivery = %+v", views[0])
	}
}
