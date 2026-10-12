// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// bundleWriters hands each row to the writer of its own type. A single CSV is
// a bundle of one part, so every run reaches the engine through this.
type bundleWriters struct {
	parts map[string]*csvWriters
	// links is the one resolver every part writer shares, so a row of one
	// file finds a row of another.
	links *linkResolver
	// fallback reverses a row whose type the run's parts do not name: a run
	// staged before parts existed records rows under its one object only.
	fallback *csvWriters
}

var (
	_ migration.Writers     = (*bundleWriters)(nil)
	_ migration.UndoWriters = (*bundleWriters)(nil)
)

// newBundleWriters is where a run's run-wide reads happen, once, before the
// engine walks any part; it builds one writer per part.
func newBundleWriters(
	_ context.Context, db *database.DB, runID migration.RunID, mapping *migration.RunMapping,
) (*bundleWriters, error) {
	settled := migration.RunMapping{}
	if mapping != nil {
		settled = *mapping
	}
	parts := settled.PartList()
	seen := map[string]bool{}
	for _, part := range parts {
		if seen[part.Object] {
			return nil, fmt.Errorf("import: two files of this run are both %q: %w", part.Object, apperrors.ErrConflict)
		}
		seen[part.Object] = true
	}
	options := runOptionsOf(settled)
	b := &bundleWriters{parts: map[string]*csvWriters{}, links: newLinkResolver(db, importSourceSystem(settled.SourceName))}
	for _, part := range parts {
		w := newCSVWriters(db, runID, part, options, b.links)
		b.parts[part.Object] = w
		if b.fallback == nil {
			b.fallback = w
		}
	}
	return b, nil
}

// bindNamedCompanies refills the ref of every company row the file names by id.
// The binding lives in memory, so a resumed commit has none for the rows its
// checkpoint passed. A contact linking to one would then find nothing.
func (b *bundleWriters) bindNamedCompanies(ctx context.Context, source migration.Source) error {
	w, ok := b.parts[migration.ObjectCompany]
	if !ok {
		return nil
	}
	companies, err := partSourceOf(source, migration.ObjectCompany)
	if err != nil {
		return err
	}
	for offset := 0; ; offset += importPredictPage {
		rows, err := companies.Rows(ctx, migration.ObjectCompany, offset, importPredictPage)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if target, skip := w.namedTarget(ctx, migration.ObjectCompany, row); skip == "" && target.named {
				b.links.markLanded(migration.ObjectCompany, row.ExternalID, target.id)
			}
		}
		if len(rows) < importPredictPage {
			return nil
		}
	}
}

// part is the writer of one type. A run with no file of that type answers a
// conflict, because its source and its mapping disagree.
func (b *bundleWriters) part(object string) (*csvWriters, error) {
	if w, ok := b.parts[object]; ok {
		return w, nil
	}
	return nil, fmt.Errorf("import: this run carries no %q file: %w", object, apperrors.ErrConflict)
}

// Exists asks the writer of the row's type.
func (b *bundleWriters) Exists(ctx context.Context, object, externalID string) (bool, error) {
	w, err := b.part(object)
	if err != nil {
		return false, err
	}
	return w.Exists(ctx, object, externalID)
}

// ReconcileIdentities asks every part, since a resume may walk any of them.
func (b *bundleWriters) ReconcileIdentities(ctx context.Context) error {
	for _, w := range b.parts {
		if err := w.ReconcileIdentities(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Ensure lands a row through the writer of its type.
func (b *bundleWriters) Ensure(ctx context.Context, object string, row migration.Row) (migration.EnsureResult, error) {
	w, err := b.part(object)
	if err != nil {
		return migration.EnsureResult{}, err
	}
	return w.Ensure(ctx, object, row)
}

// Associate applies an edge through the writer of the type it starts from.
func (b *bundleWriters) Associate(ctx context.Context, a migration.Assoc) (migration.AssocResult, error) {
	w, err := b.part(a.FromType)
	if err != nil {
		return migration.AssocResult{}, err
	}
	return w.Associate(ctx, a)
}

// Reverse archives a row through the writer of its type. Reverse reads the
// type from its argument, so the fallback writer serves an older run's rows.
func (b *bundleWriters) Reverse(ctx context.Context, object string, nativeID ids.UUID, importedAt time.Time) error {
	w, ok := b.parts[object]
	if !ok {
		w = b.fallback
	}
	if w == nil {
		return fmt.Errorf("import undo: this run names no part to reverse %q with: %w", object, apperrors.ErrConflict)
	}
	return w.Reverse(ctx, object, nativeID, importedAt)
}
