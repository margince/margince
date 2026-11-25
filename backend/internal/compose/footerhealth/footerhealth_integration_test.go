// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package footerhealth_test

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/footerhealth"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/database"
)

// A default entity whose company states no name is incomplete, because every
// contact without an entity of its own falls back to it. Naming the company
// ends the warning.
//
// Mutation: drop the EntityName rule from GapsTx and this fails.
func TestAnUnnamedDefaultEntityWarnsUntilTheCompanyIsNamed(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	store := consent.NewStore(compose.InstallationDB(e.Pool))
	gaps := func() []footerhealth.Gap {
		t.Helper()
		var out []footerhealth.Gap
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			var err error
			out, err = footerhealth.GapsTx(ctx, tx, store)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	e.WsExec(t, `DELETE FROM company WHERE is_anchor`)
	if got := gaps(); len(got) != 1 || !got[0].IsDefault || !slices.Equal(got[0].Missing, []string{footerhealth.EntityName}) || got[0].Name != "" {
		t.Fatalf("an unnamed default entity raised %+v, want one name gap", got)
	}

	e.WsExec(t, `INSERT INTO company (display_name, is_anchor, source, captured_by) VALUES ('Beispiel GmbH', true, 'manual', 'human:x')`)
	if got := gaps(); len(got) != 0 {
		t.Errorf("a named company still warns: %+v", got)
	}
}
