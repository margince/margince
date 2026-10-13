// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Tags named in the file as `a; b; c`. A name the workspace has no tag for is
// created when the run commits, never by the test run, and the report lists it
// first. Tags are only ever added: a re-import never takes one off a record.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const csvTargetTags = "tags"

// newTagReason is the report's word for a name the commit will create.
const newTagReason = "no tag has this name yet, so the import creates it"

// splitTags reads a tags cell: semicolon-separated names, spaced as the tag
// store spells them, each once whatever its case.
func splitTags(cell string) []string {
	var names []string
	seen := map[string]bool{}
	for part := range strings.SplitSeq(cell, ";") {
		name := collections.NormalizeTagName(part)
		if name == "" || seen[tagKey(name)] {
			continue
		}
		seen[tagKey(name)] = true
		names = append(names, name)
	}
	return names
}

// tagCellProblem is the first name in a tags cell the tag store would refuse
// to create. The test run then refuses the row, and the approval never meets it.
func tagCellProblem(cell string) *cellProblem {
	for _, name := range splitTags(cell) {
		if _, err := collections.ValidateTagName(name); err != nil {
			reason := err.Error()
			if bad, ok := errors.AsType[*collections.BadInputError](err); ok {
				reason = bad.Reason
			}
			return &cellProblem{
				Target: csvTargetTags, Value: name,
				Message: fmt.Sprintf("the tag %q cannot be created: its name %s", name, reason),
			}
		}
	}
	return nil
}

// tagKey matches uq_tag_name, which is case-insensitive.
func tagKey(name string) string {
	return strings.ToLower(collections.NormalizeTagName(name))
}

// tagResolver holds the tag of every name a run carries, read once. The zero
// value applies nothing and misses nothing: it is what a seat without tag read
// gets.
type tagResolver struct {
	store   *collections.Store
	byName  map[string]ids.TagID
	missing []string
}

// newTagResolver finds the live tag of each distinct name.
func newTagResolver(ctx context.Context, store *collections.Store, names []string) (*tagResolver, error) {
	r := &tagResolver{store: store, byName: map[string]ids.TagID{}}
	for _, name := range names {
		id, live, err := store.FindTag(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("import: finding the tag %q: %w", name, err)
		}
		if !live {
			r.missing = append(r.missing, name)
			continue
		}
		r.byName[tagKey(name)] = ids.TagID{UUID: id}
	}
	return r, nil
}

// createMissing creates every name the run carries and the workspace lacks.
// Called by the commit before any row is written.
func (r *tagResolver) createMissing(ctx context.Context) error {
	for _, name := range r.missing {
		created, err := r.store.CreateTag(ctx, name, nil, nil)
		if errors.Is(err, apperrors.ErrConflict) {
			// Someone created it meanwhile; theirs is the tag the name means.
			id, live, err := r.store.FindTag(ctx, name)
			if err != nil {
				return fmt.Errorf("import: finding the tag %q: %w", name, err)
			}
			if !live {
				return fmt.Errorf("import: the tag %q was created and retired meanwhile: %w", name, apperrors.ErrConflict)
			}
			r.byName[tagKey(name)] = ids.TagID{UUID: id}
			continue
		}
		if err != nil {
			return fmt.Errorf("import: creating the tag %q: %w", name, err)
		}
		r.byName[tagKey(name)] = created.ID
	}
	r.missing = nil
	return nil
}

// applyAll applies a row's tags to the record inside the row's transaction. A
// name with no tag is skipped, and a tag the record already carries is
// convergence. Without the record's update grant nothing is applied; the test
// run reported it.
func (r *tagResolver) applyAll(ctx context.Context, tx pgx.Tx, object string, id ids.UUID, cell string) error {
	entityType, taggable := taggableObjectOf(object)
	if !taggable || !auth.Allows(ctx, entityType, principal.ActionUpdate) {
		return nil
	}
	for _, name := range splitTags(cell) {
		tag, known := r.byName[tagKey(name)]
		if !known {
			continue
		}
		_, err := r.store.ApplyTagTx(ctx, tx, tag, entityType, id)
		if err != nil && !errors.Is(err, apperrors.ErrConflict) {
			return fmt.Errorf("import: tagging %s with %q: %w", id, name, err)
		}
	}
	return nil
}

// tagExisting adds a row's tags to a record the run updates or leaves as it
// is, in a transaction of its own.
func (w *csvWriters) tagExisting(ctx context.Context, id ids.UUID, row migration.Row) error {
	cell := textFields(row.Fields)[csvTargetTags]
	if len(splitTags(cell)) == 0 {
		return nil
	}
	return database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		return w.tagNames.applyAll(ctx, tx, w.object, id, cell)
	})
}

// newTagWarnings are the report's new_tag lines for one part: each missing
// name once per run, on the first part that names it.
func newTagWarnings(missing []string, firstNamedBy map[string]string, object string) []migration.SkippedRow {
	var warnings []migration.SkippedRow
	for _, name := range missing {
		if firstNamedBy[tagKey(name)] == object {
			warnings = append(warnings, migration.SkippedRow{Kind: migration.ProblemNewTag, Value: name, Reason: newTagReason})
		}
	}
	return warnings
}
