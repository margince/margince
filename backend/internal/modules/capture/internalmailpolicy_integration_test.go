// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestInternalMailSourceEnrollmentRequiresHumanAdminAndAuditsAtomically(t *testing.T) {
	ctx, _, _, ws := newCaptureRegistryFixture(t)
	_, pool := setupCaptureDB(t)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("fixture has no actor")
	}
	actor.SeatType = principal.SeatFull
	actor.Permissions = principal.Permissions{
		RoleKeys: []string{"admin"},
		RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{
			"capture_settings": {Read: true, Update: true},
		},
	}
	ctx = principal.WithActor(ctx, actor)
	connection := ids.NewV7()
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE app_user SET status = 'active' WHERE id = $1`, actor.UserID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO capture_connection (id, user_id, provider, status, generation)
   VALUES ($1, $2, 'imap', 'connected', 3)`, connection, actor.UserID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	raw, cfg := configtest.StoredOverWith(t, pool, capture.Definitions(), capture.ConfigSettings())
	store := capture.NewSettings(raw, cfg)
	policy, err := store.GetInternalMailPolicy(ctx)
	if err != nil || policy.Enabled || len(policy.Sources) != 0 {
		t.Fatalf("default: %+v, %v", policy, err)
	}
	selection := []capture.InternalMailSelection{{ConnectionID: connection, Containers: []string{"imap:Private Operations"}}}
	for _, role := range []string{"ops", "rep"} {
		deniedActor := actor
		deniedActor.Permissions.RoleKeys = []string{role}
		denied := principal.WithActor(ctx, deniedActor)
		if _, err := store.SetInternalMailPolicy(denied, true, selection); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Fatalf("%s enrolled: %v", role, err)
		}
		if err := raw.SetRaw(denied, "capture.internal_mail_policy", json.RawMessage(`{"enabled":false,"since":null,"sources":[]}`)); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Fatalf("%s raw write: %v", role, err)
		}
		err := raw.WriteTx(denied, func(tx pgx.Tx) error {
			_, err := raw.DeleteRawTx(denied, tx, "capture.internal_mail_policy")
			return err
		})
		if !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Fatalf("%s deleted: %v", role, err)
		}
	}
	policy, err = store.SetInternalMailPolicy(ctx, true, selection)
	if err != nil {
		t.Fatal(err)
	}
	if !policy.Enabled || policy.Since == nil || len(policy.Sources) != 1 {
		t.Fatalf("not enabled: %+v", policy)
	}
	source := policy.Sources[0]
	if source.OwnerUserID == nil || *source.OwnerUserID != actor.UserID || source.Generation != 3 || source.EnrolledBy != actor.ID || source.EnrolledAt.IsZero() {
		t.Fatalf("untrusted provenance: %+v", source)
	}
	assertInternalMailPolicyAudit(ctx, t, pool, 1)
	again, err := store.SetInternalMailPolicy(ctx, true, selection)
	if err != nil || !again.Since.Equal(*policy.Since) {
		t.Fatalf("no-op changed cutover: %+v %v", again, err)
	}
	assertInternalMailPolicyAudit(ctx, t, pool, 1)
	encodedSame, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	if err := raw.SetRaw(ctx, "capture.internal_mail_policy", encodedSame); err != nil {
		t.Fatal(err)
	}
	assertInternalMailPolicyAudit(ctx, t, pool, 1)
	forged := policy
	past := source.EnrolledAt.Add(-24 * time.Hour)
	forged.Sources = append([]capture.InternalMailSource{}, policy.Sources...)
	forged.Sources[0].EnrolledAt = past
	encoded, err := json.Marshal(forged)
	if err != nil {
		t.Fatal(err)
	}
	if err := raw.SetRaw(ctx, "capture.internal_mail_policy", encoded); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("raw write backdated enrollment: %v", err)
	}
	assertInternalMailPolicyAudit(ctx, t, pool, 1)
	if err := raw.WriteTx(ctx, func(tx pgx.Tx) error {
		if _, err := raw.DeleteRawTx(ctx, tx, "capture.internal_mail_policy"); err != nil {
			return err
		}
		return errors.New("cancel transaction")
	}); err == nil {
		t.Fatal("rollback accepted")
	}
	assertInternalMailPolicyAudit(ctx, t, pool, 1)
	if _, err := store.SetInternalMailPolicy(principal.WithWorkspaceID(ctx, ws.UUID), false, selection); err != nil {
		t.Fatal(err)
	}
	assertInternalMailPolicyAudit(ctx, t, pool, 2)
}

func assertInternalMailPolicyAudit(ctx context.Context, t *testing.T, pool *pgxpool.Pool, want int) {
	t.Helper()
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var audits, outbox int
		var images string
		if err := tx.QueryRow(ctx, `SELECT count(*), coalesce(string_agg(after::text, ''), '') FROM audit_log WHERE entity_type = 'capture_settings'`).Scan(&audits, &images); err != nil {
			return err
		}
		if strings.Contains(images, "Private Operations") {
			t.Error("private folder name leaked into audit")
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'capture_settings.updated'`).Scan(&outbox); err != nil {
			return err
		}
		if audits != want || outbox != want {
			t.Errorf("audit/outbox = %d/%d, want %d", audits, outbox, want)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
