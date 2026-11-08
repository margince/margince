// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

type internalHistoryFixture struct {
	owner, admin context.Context
	registry     *capture.Registry
	settings     *capture.SettingsStore
	pool         *pgxpool.Pool
	source       capture.InternalMailSelection
	user         ids.UserID
	records      []connector.NormalizedRecord
	captured     []datasource.EntityRef
}

type internalHistoryProvider struct {
	internalMailConnector
	fixture *internalHistoryFixture
}

func (p internalHistoryProvider) EstimateBackfill(context.Context, connector.Auth, time.Time) (connector.BackfillEstimate, error) {
	return connector.BackfillEstimate{Messages: len(p.fixture.records)}, nil
}

func (p internalHistoryProvider) BackfillPage(ctx context.Context, _ connector.Auth, _ time.Time, _ string, sink connector.Sink) (connector.BackfillPageResult, error) {
	result := connector.BackfillPageResult{Scanned: len(p.fixture.records)}
	judge, ok := sink.(connector.PreStoreJudge)
	if !ok {
		return result, errors.New("sink has no preflight")
	}
	for _, record := range p.fixture.records {
		drop, err := judge.DropBeforeStore(ctx, record)
		if err != nil {
			return result, err
		}
		ref, err := sink.Upsert(ctx, record)
		if drop {
			if !errors.Is(err, connector.ErrSkip) {
				return result, fmt.Errorf("preflight drop disagrees with storage: %w", err)
			}
			result.Skipped++
		} else {
			if err != nil {
				return result, err
			}
			p.fixture.captured = append(p.fixture.captured, ref)
			result.Captured++
		}
	}
	return result, nil
}

func newInternalHistoryFixture(t *testing.T) *internalHistoryFixture {
	t.Helper()
	ctx, connecting, vault, ws, pool := newInternalMailRegistryFixture(t)
	connection, err := connecting.Connect(ctx, "gmail", connector.Auth("fixture-token"))
	if err != nil {
		t.Fatal(err)
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("fixture has no actor")
	}
	user := ids.From[ids.UserKind](actor.UserID)
	actor.SeatType = principal.SeatFull
	actor.Permissions = principal.Permissions{
		RoleKeys: []string{"admin"}, RowScope: principal.RowScopeAll,
		Objects: map[string]principal.ObjectGrant{"capture_settings": {Read: true, Update: true}},
	}
	admin := principal.WithActor(ctx, actor)
	seedInternalMailAnchor(admin, t, pool, user.UUID)
	raw, cfg := configtest.StoredOverWith(t, pool, capture.Definitions(), capture.ConfigSettings())
	store := capture.NewSettings(raw, cfg)
	source := capture.InternalMailSelection{ConnectionID: connection, Containers: []string{}}
	policy, err := store.SetInternalMailPolicy(admin, true, []capture.InternalMailSelection{source})
	if err != nil {
		t.Fatal(err)
	}
	since := policy.Since.Add(-7 * 24 * time.Hour).UTC().Truncate(24 * time.Hour)
	source.HistorySince = &since
	if _, err := store.SetInternalMailPolicy(admin, true, []capture.InternalMailSelection{source}); err != nil {
		t.Fatal(err)
	}
	fixture := &internalHistoryFixture{owner: ctx, admin: admin, settings: store, pool: pool, source: source, user: user}
	db := database.BindTo(pool, ws)
	sink := capture.NewSink(db).WithInternalMailRecorder(activities.MarkInternalMailTx).WithAudienceRecompute(activities.RecomputeAudienceTx)
	fixture.registry = capture.NewRegistry(db, sink, internalMailAuthority{}, vault)
	fixture.registry.Register(internalHistoryProvider{fixture: fixture})
	fixture.records = []connector.NormalizedRecord{
		internalHistoryRecord("eligible", policy.Since.Add(-24*time.Hour)),
		internalHistoryRecord("before-approval", since.Add(-24*time.Hour)),
		internalHistoryRecord("after-start", policy.Since.Add(24*time.Hour)),
	}
	return fixture
}

func internalHistoryRecord(name string, received time.Time) connector.NormalizedRecord {
	record := operationalMailRecord(name+"-"+ids.NewV7().String(), "colleague@example.test", "colleague@example.test", fixtureOwnerEmail)
	record.CapturedBy, record.Source, record.DeliveredTo = "connector:gmail", "gmail:operational", fixtureOwnerEmail
	record.ProviderReceivedAt = received
	return record
}

func historyEnqueue(context.Context, pgx.Tx, ids.UUID) error { return nil }

func (f *internalHistoryFixture) complete(t *testing.T, run capture.BackfillRun) {
	t.Helper()
	done, completed, _, err := f.registry.RunBackfillStep(f.owner, run.ID)
	if err != nil || !done || !completed {
		t.Fatalf("backfill step: done=%v completed=%v err=%v", done, completed, err)
	}
}

func TestInternalHistoryNeedsAdministratorScopeAndSeparateOwnerConsent(t *testing.T) {
	f := newInternalHistoryFixture(t)
	normal, err := f.registry.StartBackfill(f.owner, "gmail", f.user, 3, connector.BackfillEstimate{}, historyEnqueue)
	if err != nil {
		t.Fatal(err)
	}
	f.complete(t, normal)
	if len(f.captured) != 0 {
		t.Fatal("ordinary history import captured internal mail")
	}
	preview, err := f.registry.EstimateBackfill(f.owner, "gmail", f.user, 3)
	if err != nil || preview.InternalMail == nil {
		t.Fatalf("history preview: %+v %v", preview.InternalMail, err)
	}
	if _, err := f.registry.StartBackfillWithInternalHistory(f.owner, "gmail", f.user, 6, preview.BackfillEstimate, preview.InternalMail.Token, historyEnqueue); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("different window accepted preview: %v", err)
	}
	if _, err := f.registry.StartBackfillWithInternalHistory(f.owner, "gmail", f.user, 3, preview.BackfillEstimate, preview.InternalMail.Token, func(context.Context, pgx.Tx, ids.UUID) error { return errors.New("queue unavailable") }); err == nil {
		t.Fatal("failed scheduling authorized history")
	}
	assertInternalHistoryWrites(t, f, 1, 0)
	authorized, err := f.registry.StartBackfillWithInternalHistory(f.owner, "gmail", f.user, 3, preview.BackfillEstimate, preview.InternalMail.Token, historyEnqueue)
	if err != nil {
		t.Fatal(err)
	}
	f.complete(t, authorized)
	if len(f.captured) != 1 {
		t.Fatalf("captured %d messages, want only the one within the approved bounds", len(f.captured))
	}
	assertInternalHistoryWrites(t, f, 2, 1)
	if err := database.WithWorkspaceTx(f.admin, f.pool, func(tx pgx.Tx) error {
		var receipt string
		if err := tx.QueryRow(f.admin, `SELECT internal_mail_authorization->'history'->>'backfill_id' FROM activity WHERE id = $1`, f.captured[0].ID).Scan(&receipt); err != nil {
			return err
		}
		if receipt != authorized.ID.String() {
			t.Errorf("missing durable history authorization: %q", receipt)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestInternalHistoryRejectsStalePreviewAndRevokedSourceDuringImport(t *testing.T) {
	f := newInternalHistoryFixture(t)
	preview, err := f.registry.EstimateBackfill(f.owner, "gmail", f.user, 3)
	if err != nil || preview.InternalMail == nil {
		t.Fatalf("preview: %+v %v", preview.InternalMail, err)
	}
	f.source.HistorySince = nil
	if _, err := f.settings.SetInternalMailPolicy(f.admin, true, []capture.InternalMailSelection{f.source}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.registry.StartBackfillWithInternalHistory(f.owner, "gmail", f.user, 3, preview.BackfillEstimate, preview.InternalMail.Token, historyEnqueue); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("stale scope accepted: %v", err)
	}
	assertInternalHistoryWrites(t, f, 0, 0)
	since := f.records[0].ProviderReceivedAt.Add(-24 * time.Hour).UTC().Truncate(24 * time.Hour)
	f.source.HistorySince = &since
	if _, err := f.settings.SetInternalMailPolicy(f.admin, true, []capture.InternalMailSelection{f.source}); err != nil {
		t.Fatal(err)
	}
	preview, err = f.registry.EstimateBackfill(f.owner, "gmail", f.user, 3)
	if err != nil || preview.InternalMail == nil {
		t.Fatalf("fresh preview: %+v %v", preview.InternalMail, err)
	}
	run, err := f.registry.StartBackfillWithInternalHistory(f.owner, "gmail", f.user, 3, preview.BackfillEstimate, preview.InternalMail.Token, historyEnqueue)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.settings.SetInternalMailPolicy(f.admin, false, []capture.InternalMailSelection{f.source}); err != nil {
		t.Fatal(err)
	}
	f.complete(t, run)
	if len(f.captured) != 0 {
		t.Fatal("revoked source imported internal history")
	}
}

func assertInternalHistoryWrites(t *testing.T, f *internalHistoryFixture, runs, consents int) {
	t.Helper()
	if err := database.WithWorkspaceTx(f.admin, f.pool, func(tx pgx.Tx) error {
		var gotRuns, gotConsents int
		if err := tx.QueryRow(f.admin, `SELECT count(*) FROM capture_backfill`).Scan(&gotRuns); err != nil {
			return err
		}
		if err := tx.QueryRow(f.admin, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'internal_mail_history.created'`).Scan(&gotConsents); err != nil {
			return err
		}
		if gotRuns != runs || gotConsents != consents {
			t.Errorf("runs/consents = %d/%d, want %d/%d", gotRuns, gotConsents, runs, consents)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
