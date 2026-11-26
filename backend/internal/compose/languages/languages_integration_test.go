// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package languages_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/languages"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

const insertActivity = `INSERT INTO activity (kind, source, captured_by, language) VALUES ('note', 'manual', 'system:test', 'ar')`

func TestAUnitLanguageIsStoredWithNoCoreMigration(t *testing.T) {
	e := integration.Setup(t)
	ctx := context.Background()
	owner := integration.OwnerConn(t)
	t.Cleanup(func() {
		for _, q := range []string{`DELETE FROM activity WHERE language = 'ar'`, `DELETE FROM language WHERE code = 'ar'`} {
			if _, err := owner.Exec(context.Background(), q); err != nil {
				t.Errorf("cleaning up: %v", err)
			}
		}
	})

	if _, err := owner.Exec(ctx, insertActivity); err == nil {
		t.Fatal("an activity in a language no unit declared was stored")
	}
	units := []extension.Extension{{Name: "egypt-pack", Languages: []extension.Language{{Code: "ar", Label: "Arabic"}}}}
	for range 2 {
		if err := languages.Reconcile(ctx, e.Pool, units); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
	}
	if _, err := owner.Exec(ctx, insertActivity); err != nil {
		t.Fatalf("an activity in the declared language was refused: %v", err)
	}
	var source string
	if err := owner.QueryRow(ctx, `SELECT source FROM language WHERE code = 'ar'`).Scan(&source); err != nil || source != "unit" {
		t.Fatalf("the language row = %q, %v; want source unit", source, err)
	}
}

func TestReconcileRefusesAUnitThatDeclaresACoreLanguageAndWritesNothing(t *testing.T) {
	e := integration.Setup(t)
	units := []extension.Extension{{Name: "bad-pack", Languages: []extension.Language{
		{Code: "ar", Label: "Arabic"}, {Code: "de", Label: "Deutsch"},
	}}}
	if err := languages.Reconcile(context.Background(), e.Pool, units); err == nil {
		t.Fatal("a unit redeclaring a core language was admitted")
	}
	var n int
	if err := integration.OwnerConn(t).QueryRow(context.Background(), `SELECT count(*) FROM language WHERE code = 'ar'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("ar rows after a refused set = %d, %v; want 0", n, err)
	}
}

// The report lists a row the new keys would have refused, and changes nothing.
func TestTheReportListsAnOldNoticeLanguageAndChangesNothing(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	owner := integration.OwnerConn(t)

	err := pgx.BeginFunc(ctx, owner, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `ALTER TABLE consent_text_version DROP CONSTRAINT consent_text_version_locale_fkey`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO consent_text_version (key, version, body, locale) VALUES ('old-notice', 'v1', 'Body', 'de-DE')`); err != nil {
			return err
		}
		out := runStep(t, ctx, tx, "consent_text_version_locale")
		if out.Action == nil || out.Action.Reason != languages.ReasonNotRegistered || len(out.Action.Records) != 1 {
			t.Errorf("the report left %+v, want one record under %s", out.Action, languages.ReasonNotRegistered)
		}
		if out.Changed != 0 {
			t.Errorf("the report changed %d rows", out.Changed)
		}
		var locale string
		if err := tx.QueryRow(ctx, `SELECT locale FROM consent_text_version WHERE key = 'old-notice'`).Scan(&locale); err != nil || locale != "de-DE" {
			t.Errorf("the stored locale = %q, %v; the report must not rewrite it", locale, err)
		}
		if clean := runStep(t, ctx, tx, "activity_language"); clean.Action != nil {
			t.Errorf("a column with no bad value reported %+v", clean.Action)
		}
		return rollback{}
	})
	if _, ok := err.(rollback); !ok {
		t.Fatalf("running the report: %v", err)
	}
}

type rollback struct{}

func (rollback) Error() string { return "roll the test's rows back" }

func runStep(t *testing.T, ctx context.Context, tx pgx.Tx, name string) datatask.BatchOut {
	t.Helper()
	for _, step := range languages.DataTask().Steps {
		if step.Name == name {
			out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 100})
			if err != nil {
				t.Fatalf("step %s: %v", name, err)
			}
			return out
		}
	}
	t.Fatalf("no step named %s", name)
	return datatask.BatchOut{}
}
