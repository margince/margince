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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestInternalMailUsesTheSameSourceDecisionInPreflightAndStorage(t *testing.T) {
	ctx, connecting, vault, ws, pool := newInternalMailRegistryFixture(t)
	connection, err := connecting.Connect(ctx, "gmail", connector.Auth("fixture-token"))
	if err != nil {
		t.Fatal(err)
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("fixture has no actor")
	}
	actor.SeatType = principal.SeatFull
	actor.Permissions = principal.Permissions{
		RoleKeys: []string{"admin"}, RowScope: principal.RowScopeAll,
		Objects: map[string]principal.ObjectGrant{"capture_settings": {Read: true, Update: true}},
	}
	admin := principal.WithActor(ctx, actor)
	seedInternalMailAnchor(admin, t, pool, actor.UserID)
	raw, cfg := configtest.StoredOverWith(t, pool, capture.Definitions(), capture.ConfigSettings())
	store := capture.NewSettings(raw, cfg)
	rec := operationalMailRecord("operational-"+ids.NewV7().String(), "colleague@example.test", "colleague@example.test", fixtureOwnerEmail)
	rec.CapturedBy = "connector:gmail"
	rec.Source = "gmail:operational"
	rec.Containers = []string{"gmail:INBOX"}
	rec.DeliveredTo = fixtureOwnerEmail
	wantDrop := true
	var result ids.UUID
	db := database.BindTo(pool, ws)
	sink := capture.NewSink(db).WithInternalMailRecorder(activities.MarkInternalMailTx).WithAudienceRecompute(activities.RecomputeAudienceTx)
	reg := capture.NewRegistry(db, sink, internalMailAuthority{}, vault)
	reg.Register(internalMailLiveProvider{act: func(runCtx context.Context, target connector.Sink) error {
		judge, ok := target.(connector.PreStoreJudge)
		if !ok {
			return fmt.Errorf("sink has no preflight")
		}
		drop, err := judge.DropBeforeStore(runCtx, rec)
		if err != nil {
			return err
		}
		if drop != wantDrop {
			return fmt.Errorf("preflight drop = %v, want %v", drop, wantDrop)
		}
		ref, err := target.Upsert(runCtx, rec)
		if wantDrop {
			if !errors.Is(err, connector.ErrSkip) {
				return fmt.Errorf("storage should skip: %w", err)
			}
			return nil
		}
		if err != nil {
			return err
		}
		result = ref.ID
		return nil
	}})
	run := func() {
		t.Helper()
		if err := reg.SyncOnce(principal.WithWorkspaceID(context.Background(), ws.UUID), connection); err != nil {
			t.Fatal(err)
		}
	}
	run()
	policy, err := store.SetInternalMailPolicy(admin, true, []capture.InternalMailSelection{{ConnectionID: connection, Containers: []string{"gmail:INBOX"}}})
	if err != nil {
		t.Fatal(err)
	}
	rec.ProviderReceivedAt = policy.Since.Add(time.Second)
	wantDrop = false
	run()
	if result.IsZero() {
		t.Fatal("no activity captured")
	}
	if err := database.WithWorkspaceTx(admin, pool, func(tx pgx.Tx) error {
		var reason, audience string
		var marked bool
		if err := tx.QueryRow(admin, `SELECT audience, audience_reason, internal_mail_authorization IS NOT NULL FROM activity WHERE id = $1`, result).Scan(&audience, &reason, &marked); err != nil {
			return err
		}
		if !marked || audience != "participants" || reason != activities.ReasonInternalOperational {
			t.Errorf("internal restriction = %s %s %v", audience, reason, marked)
		}
		var contacts, links int
		if err := tx.QueryRow(admin, `SELECT count(*) FROM contact`).Scan(&contacts); err != nil {
			return err
		}
		if err := tx.QueryRow(admin, `SELECT count(*) FROM activity_link WHERE activity_id = $1`, result).Scan(&links); err != nil {
			return err
		}
		if contacts != 0 || links != 0 {
			t.Errorf("automatic employee records/links = %d/%d", contacts, links)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	run()
	rec.Containers = []string{"gmail:Other"}
	wantDrop = true
	run()
	rec.Containers = []string{"gmail:INBOX"}
	rec.ProviderReceivedAt = policy.Since.Add(-time.Second)
	run()
	rec.ProviderReceivedAt = policy.Since.Add(time.Second)
	exclusions := capture.NewExclusionStore(db)
	rule, err := exclusions.Add(admin, capture.ExclusionScopeWorkspace, capture.ExclusionKindAddress, "colleague@example.test")
	if err != nil {
		t.Fatal(err)
	}
	run()
	if err := exclusions.Remove(admin, rule.ID); err != nil {
		t.Fatal(err)
	}
	wantDrop = false
	run()
	wantDrop = true
	if _, err := store.SetInternalMailPolicy(admin, false, policySelections(policy)); err != nil {
		t.Fatal(err)
	}
	run()
}

func policySelections(policy capture.InternalMailPolicy) []capture.InternalMailSelection {
	selections := make([]capture.InternalMailSelection, len(policy.Sources))
	for i, source := range policy.Sources {
		selections[i] = capture.InternalMailSelection{ConnectionID: source.ConnectionID, Containers: source.Containers, HistorySince: source.HistorySince}
	}
	return selections
}

func seedInternalMailAnchor(admin context.Context, t *testing.T, pool *pgxpool.Pool, user ids.UUID) {
	t.Helper()
	actor, ok := principal.Actor(admin)
	if !ok {
		t.Fatal("fixture has no actor")
	}
	if err := database.WithWorkspaceTx(admin, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(admin, `UPDATE app_user SET status = 'active' WHERE id = $1`, user); err != nil {
			return err
		}
		company := ids.NewV7()
		if _, err := tx.Exec(admin, `INSERT INTO company (id, display_name, is_anchor, source, captured_by)
   VALUES ($1, 'Our Company', true, 'manual', $2)`, company, actor.ID); err != nil {
			return err
		}
		_, err := tx.Exec(admin, `INSERT INTO company_domain (company_id, domain, is_primary, source, captured_by)
   VALUES ($1, 'example.test', true, 'manual', $2)`, company, actor.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
