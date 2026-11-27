// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package languages owns the language table: the languages stored data may
// carry. Core seeds the three it ships (textlang.Shipped) in its migration.
// The composed units add theirs here at boot, validate first and apply after.
// It never deletes a row, because stored rows still name the language.
package languages

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/pkg/extension"
)

// Preflight refuses a set whose language declarations the table could not hold,
// before anything is registered. It also refuses a code core already ships. A
// silent skip would hide a unit that believes it ships that language.
func Preflight(exts []extension.Extension) error {
	for _, unit := range exts {
		for _, language := range unit.Languages {
			if err := language.Validate(); err != nil {
				return fmt.Errorf("compose: extension %q: %w", unit.Name, err)
			}
			if textlang.Known(language.Code) {
				return fmt.Errorf("compose: extension %q declares language %q, which core already ships", unit.Name, language.Code)
			}
		}
	}
	return nil
}

// declared lists the languages the units add, one per code in the order of the
// units' names. Two units may declare one code (two jurisdiction packs that both
// write Arabic); the first unit's label is the one stored.
func declared(exts []extension.Extension) []extension.Language {
	units := slices.Clone(exts)
	slices.SortFunc(units, func(a, b extension.Extension) int { return cmp.Compare(a.Name, b.Name) })
	var out []extension.Language
	for _, unit := range units {
		for _, language := range unit.Languages {
			if !slices.ContainsFunc(out, func(l extension.Language) bool { return l.Code == language.Code }) {
				out = append(out, language)
			}
		}
	}
	return out
}

// Reconcile writes the units' languages into the table. Every write is an
// idempotent upsert, and a core row is never touched: a label change reaches a
// unit row only.
//
// It runs over database.WithInfraTx like channel_provider's reconcile. The table
// has no workspace_id, and a fresh install has no workspace at its first boot.
func Reconcile(ctx context.Context, pool *pgxpool.Pool, exts []extension.Extension) error {
	if err := Preflight(exts); err != nil {
		return err
	}
	return database.WithInfraTx(ctx, pool, func(tx pgx.Tx) error {
		for _, language := range declared(exts) {
			if _, err := tx.Exec(ctx, `
				INSERT INTO language (code, label, source) VALUES ($1, $2, 'unit')
				ON CONFLICT (code) DO UPDATE SET label = EXCLUDED.label
				WHERE language.source = 'unit'`, language.Code, language.Label); err != nil {
				return fmt.Errorf("compose: registering language %q: %w", language.Code, err)
			}
		}
		return nil
	})
}
