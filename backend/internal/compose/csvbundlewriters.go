// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// bundleWriters hands each row to the writer of its own type. A single CSV is
// a bundle of one part, so every run reaches the engine through this.
type bundleWriters struct {
	reviewPlan *migration.ReviewPlan
	parts      map[string]*csvWriters
	// links is the one resolver every part writer shares, so a row of one
	// file finds a row of another.
	links *linkResolver
	// fallback reverses a row whose type the run's parts do not name: a run
	// staged before parts existed records rows under its one object only.
	fallback *csvWriters
	// owners and tags resolve the run's owner and tag cells, read once and
	// shared by every part writer.
	owners *ownerResolver
	tags   *tagResolver
	// tagFirstNamedBy is the part that first names each tag, keyed by tagKey.
	tagFirstNamedBy map[string]string
	// grantProblems are the tag and owner grants the importer lacks, for the
	// test run's report.
	grantProblems []migration.SkippedRow
}

var (
	_ migration.Writers     = (*bundleWriters)(nil)
	_ migration.UndoWriters = (*bundleWriters)(nil)
)

// runOptions is what a run decides for every part alike.
type runOptions struct {
	SourceName, OnDuplicate, ContextTag, OnBrokenLink string
	// FromZip is a run staged from a ZIP, whose every link follows OnBrokenLink.
	FromZip bool
	// PresetLinks file every activity of the run under the same records.
	PresetLinks []migration.PresetLink
}

// runOptionsOf reads the run-wide decisions off a stored mapping.
func runOptionsOf(m migration.RunMapping) runOptions {
	return runOptions{
		SourceName: m.SourceName, OnDuplicate: m.OnDuplicate, ContextTag: m.ContextTag,
		OnBrokenLink: m.OnBrokenLink, FromZip: len(m.Parts) > 0, PresetLinks: m.PresetLinks,
	}
}

// newBundleWriters is where a run's run-wide reads happen, once, before the
// engine walks any part; it builds one writer per part. A nil source is an
// undo, which resolves no owner and no tag.
func newBundleWriters(
	ctx context.Context, db *database.DB, runID migration.RunID, mapping *migration.RunMapping, src migration.Source,
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
	if err := b.resolveOwnersAndTags(ctx, db, src); err != nil {
		return nil, err
	}
	stages, err := runDealStages(ctx, db, parts, src)
	if err != nil {
		return nil, err
	}
	zone, err := runInstallationZone(ctx, db)
	if err != nil {
		return nil, err
	}
	for _, part := range parts {
		cols, err := readImportColumns(ctx, db, part.Object)
		if err != nil {
			return nil, err
		}
		w := newCSVWriters(db, runID, part, options, b.links, cols)
		w.owners, w.tagNames, w.dealStages, w.zone = b.owners, b.tags, stages, zone
		b.parts[part.Object] = w
		if b.fallback == nil {
			b.fallback = w
		}
	}
	return b, nil
}

// runInstallationZone reads the zone that says which date is today for a run.
func runInstallationZone(ctx context.Context, db *database.DB) (*time.Location, error) {
	var zone *time.Location
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		zone, err = installationZoneTx(ctx, tx)
		return err
	})
	return zone, err
}

// runDealStages reads the pipelines a run's deal rows are checked against. An
// undo, or a run with no deals file, checks no deal row and reads nothing.
func runDealStages(ctx context.Context, db *database.DB, parts []migration.PartMapping, src migration.Source) (dealStages, error) {
	if src == nil || !slices.ContainsFunc(parts, func(p migration.PartMapping) bool { return p.Object == migration.ObjectDeal }) {
		return dealStages{}, nil
	}
	return readDealStages(ctx, db)
}

// resolveOwnersAndTags reads every owner and tag cell of the run in import
// order. It resolves each distinct address and name once, then checks the
// grants they take. Without tag read the tag resolver is the empty one, and the
// dry run reports the missing grant.
func (b *bundleWriters) resolveOwnersAndTags(ctx context.Context, db *database.DB, src migration.Source) error {
	b.owners, b.tags, b.tagFirstNamedBy = &ownerResolver{seats: map[string]ids.UUID{}}, &tagResolver{}, map[string]string{}
	if src == nil {
		return nil
	}
	emails, names, err := ownerAndTagCells(ctx, src, b.tagFirstNamedBy)
	if err != nil {
		return err
	}
	if b.owners, err = newOwnerResolver(ctx, db.Pool(), emails); err != nil {
		return err
	}
	if auth.Allows(ctx, tagObject, principal.ActionRead) {
		if b.tags, err = newTagResolver(ctx, collections.NewStore(db), names); err != nil {
			return err
		}
	}
	return b.checkTagAndOwnerGrants(ctx, db.Pool())
}

// cellCollector gathers the distinct owner addresses and tag names of the rows
// it is given, and the part that first names each tag.
type cellCollector struct {
	emails, names []string
	seenEmail     map[string]bool
	firstNamedBy  map[string]string
}

func (c *cellCollector) add(object string, row migration.Row) {
	fields := textFields(row.Fields)
	if email := foldedEmail(fields[csvTargetOwner]); email != "" && !c.seenEmail[email] {
		c.seenEmail[email] = true
		c.emails = append(c.emails, email)
	}
	if tagCellProblem(fields[csvTargetTags]) != nil {
		return
	}
	for _, name := range splitTags(fields[csvTargetTags]) {
		if _, named := c.firstNamedBy[tagKey(name)]; !named {
			c.firstNamedBy[tagKey(name)] = object
			c.names = append(c.names, name)
		}
	}
}

// ownerAndTagCells is the run's distinct owner addresses and tag names, and
// the part that first names each tag.
func ownerAndTagCells(ctx context.Context, src migration.Source, firstNamedBy map[string]string) ([]string, []string, error) {
	c := &cellCollector{seenEmail: map[string]bool{}, firstNamedBy: firstNamedBy}
	for _, object := range src.Objects() {
		for offset := 0; ; offset += importPredictPage {
			rows, err := src.Rows(ctx, object, offset, importPredictPage)
			if err != nil {
				return nil, nil, err
			}
			for _, row := range rows {
				c.add(object, row)
			}
			if len(rows) < importPredictPage {
				break
			}
		}
	}
	return c.emails, c.names, nil
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
	if err := w.validateReviewedLinks(ctx, a.FromID); err != nil {
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
