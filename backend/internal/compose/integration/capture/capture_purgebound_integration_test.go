// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/capturepurgebind"
	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const boundSender = "late@example.test"

func boundSeat(e *integration.Env) context.Context {
	ctx := e.As(e.Rep1, nil, principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"activity": {Read: true, Update: true, Delete: true},
			"contact":  {Read: true, Create: true, Update: true, Delete: true},
		},
		RowScope: principal.RowScopeAll,
	})
	return integration.OnSeat(ctx, principal.SeatFull)
}

func boundMail(t *testing.T, e *integration.Env) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, body, direction, source_system, source_id,
			                      source, captured_by, counterparty_email, audience)
			VALUES ($1, 'email', 'Hello', 'the message body', 'inbound', 'gmail', $2,
			        'gmail:'||$2, 'connector:gmail', $3, 'participants')`,
			id, "pg-"+id.String(), boundSender); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_import (activity_id, user_id, posture_at_import)
			VALUES ($1, $2, 'classified')`, id, e.Rep1)
		return err
	}); err != nil {
		t.Fatalf("seeding mail: %v", err)
	}
	return id
}

func boundExists(t *testing.T, e *integration.Env, id ids.UUID) bool {
	t.Helper()
	var body string
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT coalesce(body, '') FROM activity WHERE id = $1`, id).Scan(&body)
	})
	return err == nil && body != ""
}

func confirmed(ctx context.Context, t *testing.T, seen compose.PurgeOutcome) context.Context {
	t.Helper()
	bound, err := capturepurgebind.Bind(ctx, &seen.Destroyed, &seen.Released, &seen.Anonymised)
	if err != nil {
		t.Fatalf("binding the previewed counts: %v", err)
	}
	return bound
}

// Mail that arrives between the preview and the confirm is not destroyed. The
// caller confirmed one count and was never shown a larger one.
func TestAPurgeBoundToAPreviewRefusesMailThatArrivedAfterIt(t *testing.T) {
	t.Parallel()
	for _, before := range []int{0, 1} {
		e := integration.Setup(t)
		var first ids.UUID
		if before == 1 {
			first = boundMail(t, e)
		}
		rule, err := capturemod.NewExclusionStore(compose.InstallationDB(e.Pool)).Add(
			boundSeat(e), capturemod.ExclusionScopeUser, capturemod.ExclusionKindAddress, boundSender, ids.Nil)
		if err != nil {
			t.Fatalf("adding the rule: %v", err)
		}
		purger := compose.NewCapturePurger(e.Pool, compose.NewRetentionServiceFor(compose.InstallationDB(e.Pool), nil, slog.Default()))
		seen, err := purger.Purge(boundSeat(e), rule.ID, true)
		if err != nil || seen.Destroyed != before {
			t.Fatalf("preview with %d before: %+v, %v", before, seen, err)
		}

		arrived := boundMail(t, e)
		if _, err := purger.Purge(confirmed(boundSeat(e), t, seen), rule.ID, false); !isConflict(err) {
			t.Fatalf("%d before: err = %v, want a conflict because the selection grew", before, err)
		}
		if !boundExists(t, e, arrived) || (first != ids.Nil && !boundExists(t, e, first)) {
			t.Fatalf("%d before: a refused purge destroyed mail", before)
		}

		fresh, err := purger.Purge(boundSeat(e), rule.ID, true)
		if err != nil {
			t.Fatalf("second preview: %v", err)
		}
		if _, err := purger.Purge(confirmed(boundSeat(e), t, fresh), rule.ID, false); err != nil {
			t.Fatalf("a purge bound to the fresh preview was refused: %v", err)
		}
		if boundExists(t, e, arrived) {
			t.Fatalf("%d before: the confirmed purge left the mail", before)
		}
	}
}

func isConflict(err error) bool {
	return err != nil && errors.Is(err, apperrors.ErrConflict)
}
