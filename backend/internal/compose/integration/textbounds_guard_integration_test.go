// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The migration how-to tells an author to take both guards off activity before
// a bounded column goes. This holds the two facts that recipe rests on.
func TestABoundedColumnIsDroppedOnlyAfterItsGuardsAreTakenOff(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tx, err := SchemaPool(t).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(ctx, t, tx)
	guarded, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = guarded.Exec(ctx, `ALTER TABLE public.activity DROP COLUMN agenda_fingerprint`)
	var refusal *pgconn.PgError
	if !errors.As(err, &refusal) || refusal.Code != "2BP01" || !strings.Contains(refusal.Detail, "text_bound_") {
		t.Fatalf("dropping a column its guard names → %v, want a refusal that names the guard", err)
	}
	rollback(ctx, t, guarded)
	if _, err := tx.Exec(ctx, `DROP TRIGGER text_bound_insert ON public.activity; DROP TRIGGER text_bound_update ON public.activity;
		ALTER TABLE public.activity DROP COLUMN agenda_fingerprint`); err != nil {
		t.Fatalf("dropping the column once both guards are off: %v", err)
	}
}

func TestATextListKeepsAnEarlierLongEntryAndRefusesANewOne(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tx, err := SchemaPool(t).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(ctx, t, tx)
	earlier := strings.Repeat("界", values.TextDocumentCharacters+1)
	var id string
	// The fixture models an entry written before the list had a bound.
	if _, err := tx.Exec(ctx, `ALTER TABLE public.activity DISABLE TRIGGER text_bound_insert`); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO public.activity (kind, source, captured_by, subject, occurred_at, redacted_fields)
		VALUES ('note', 'manual', 'human:00000000-0000-0000-0000-000000000001', 'Earlier list entry', now(), ARRAY[$1])
		RETURNING id`, earlier).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `ALTER TABLE public.activity ENABLE TRIGGER text_bound_insert`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE public.activity SET subject = 'Renamed', redacted_fields = ARRAY[$2, 'short'] WHERE id = $1`, id, earlier); err != nil {
		t.Fatalf("an edit that keeps the earlier long entry was refused: %v", err)
	}
	assertListRefused(ctx, t, tx, `UPDATE public.activity SET redacted_fields = ARRAY[$2] WHERE id = $1`, id, earlier+"界")
	assertListRefused(ctx, t, tx, `INSERT INTO public.activity (kind, source, captured_by, subject, occurred_at, redacted_fields)
		VALUES ('note', 'manual', 'human:00000000-0000-0000-0000-000000000001', $1, now(), ARRAY[$2])`, "New list entry", earlier)
}

func assertListRefused(ctx context.Context, t *testing.T, tx pgx.Tx, statement string, args ...any) {
	t.Helper()
	attempt, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(ctx, t, attempt)
	_, err = attempt.Exec(ctx, statement, args...)
	var refusal *pgconn.PgError
	if !errors.As(err, &refusal) || refusal.ConstraintName != storekit.TextBoundConstraint || refusal.ColumnName != "redacted_fields" {
		t.Fatalf("a new long list entry → %v, want the text bound to name the list", err)
	}
}

func rollback(ctx context.Context, t *testing.T, tx pgx.Tx) {
	t.Helper()
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		t.Errorf("roll back: %v", err)
	}
}
