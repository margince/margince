// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/unitsql/unitsqltest"
	"github.com/margince/margince/backend/pkg/extension"
)

// Runtime.Tx hands a unit the confined verbs, and the core door keeps the
// application role between them. The cases of the confinement are in unitsql.
func TestRuntimeTxRefusesACoreTableAndKeepsTheCoreDoor(t *testing.T) {
	e := setupCore(t)
	rt, ctx, cancel := e.runtime(t)
	defer cancel()
	for _, sql := range []string{`SELECT id FROM contact`, `INSERT INTO contact (id) VALUES (gen_random_uuid())`, `SELECT id FROM extension_secret`} {
		if err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
			_, err := tx.Exec(ctx, sql)
			return err
		}); !errors.Is(err, extension.ErrCoreTableRefused) {
			t.Errorf("%s answered %v, want ErrCoreTableRefused", sql, err)
		}
	}

	unitsqltest.ScratchTable(t, "ext.ext_notes_scratch")
	if err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO ext.ext_notes_scratch (id, body) VALUES (gen_random_uuid(), 'x')`); err != nil {
			return err
		}
		_, err := tx.Core().Activities().Create(ctx, e.aNote(e.subject))
		return err
	}); err != nil {
		t.Fatalf("the unit's own write, then the core door: %v", err)
	}
	if n := e.count(t, `SELECT count(*) FROM activity`); n != 1 {
		t.Errorf("%d activities, want the 1 the door filed", n)
	}
}
