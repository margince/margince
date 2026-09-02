// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type tagRow struct {
	ID          ids.TagID
	Name        string
	Color       *string
	Description *string
	Version     int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ArchivedAt  *time.Time
}

const tagColumns = `id, name, color, description, version, created_at, updated_at, archived_at`

func scanTag(r pgx.Row) (tagRow, error) {
	var t tagRow
	err := r.Scan(&t.ID, &t.Name, &t.Color, &t.Description, &t.Version,
		&t.CreatedAt, &t.UpdatedAt, &t.ArchivedAt)
	return t, err
}

// Tags are workspace-shared vocabulary (no owner column): object RBAC
// gates them, row scope does not apply. The read is bounded by the same
// catalogCap as lists — see the constant for why the contract has no
// cursor here.
func (s *Store) ListTags(ctx context.Context, archived storekit.ArchivedFilter) ([]tagRow, bool, error) {
	if err := auth.Require(ctx, "tag", principal.ActionRead); err != nil {
		return nil, false, err
	}
	var out []tagRow
	truncated := false
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		sql := "SELECT " + tagColumns + " FROM tag"
		if archived != storekit.IncludeArchived {
			sql += " WHERE archived_at IS NULL"
		}
		rows, err := tx.Query(ctx, sql+" ORDER BY lower(name) LIMIT $1", catalogCap+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			t, err := scanTag(rows)
			if err != nil {
				return err
			}
			out = append(out, t)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(out) > catalogCap {
			out = out[:catalogCap]
			truncated = true
		}
		return nil
	})
	return out, truncated, err
}

func (s *Store) CreateTag(ctx context.Context, name string, color *string) (tagRow, error) {
	if err := auth.Require(ctx, "tag", principal.ActionCreate); err != nil {
		return tagRow{}, err
	}
	name, err := ValidateTagName(name)
	if err != nil {
		return tagRow{}, err
	}
	var out tagRow
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO tag (name, color)
			VALUES ($1, $2)
			RETURNING `+tagColumns, name, color)
		var err error
		if out, err = scanTag(row); err != nil {
			if constraint, ok := storekit.UniqueViolation(err); ok && constraint == "uq_tag_name" {
				return fmt.Errorf("tag %q: %w", name, apperrors.ErrConflict)
			}
			return err
		}
		_, err = storekit.Audit(ctx, tx, "create", "tag", out.ID.UUID, nil, map[string]any{"name": out.Name})
		return err
	})
	return out, err
}

func (s *Store) ArchiveTag(ctx context.Context, id ids.TagID) (tagRow, error) {
	if err := auth.Require(ctx, "tag", principal.ActionDelete); err != nil {
		return tagRow{}, err
	}
	var out tagRow
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			"UPDATE tag SET archived_at = now() WHERE id = $1 AND archived_at IS NULL RETURNING "+tagColumns, id)
		var err error
		if out, err = scanTag(row); errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		} else if err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "archive", "tag", id.UUID, nil, nil)
		return err
	})
	return out, err
}

type taggableRow struct {
	// ID is the taggable link-row id — a join row, not a first-class
	// entity, so it stays untyped.
	ID    ids.UUID
	TagID ids.TagID
	// EntityType + EntityID are the polymorphic tag target (any entity),
	// so the id stays untyped (rule 6).
	EntityType string
	EntityID   ids.UUID
	CreatedAt  time.Time
}

// assigningPrincipal names who is putting this tag on this record, for the
// taggable row's own columns. Both are nullable and both stay nil for a
// principal the auth layer never bound: an assignment whose author is unknown
// has to READ as unknown, because "system" would be a claim nobody made.
//
// The kind is the coarse one the record page shows — a reader needs to know a
// tag arrived from an import rather than from a colleague, and does not need
// the passport behind it. Connector and system principals are deliberately
// unmapped: neither applies tags today, and inventing a name for a caller
// that does not exist is how a wrong label gets believed later.
func assigningPrincipal(ctx context.Context) (*ids.UUID, *string) {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return nil, nil
	}
	var kind string
	switch actor.Type {
	case principal.PrincipalHuman:
		kind = "human"
	case principal.PrincipalAgent:
		kind = "agent"
	default:
		return nil, nil
	}
	// OnBehalfOf is the human authority behind an agent action; an agent
	// tagging a record is that human's reach, so the row names them and the
	// kind says the hand was an agent's.
	who := actor.UserID
	if actor.Type == principal.PrincipalAgent && actor.OnBehalfOf != (ids.UUID{}) {
		who = actor.OnBehalfOf
	}
	if who == (ids.UUID{}) {
		return nil, &kind
	}
	return &who, &kind
}

func (s *Store) ApplyTag(ctx context.Context, tagID ids.TagID, entityType string, entityID ids.UUID) (taggableRow, error) {
	// Required by the contract, true only if checked: the zero UUID would reach
	// the row-scope link gate and answer not-found for a record nobody named.
	if err := httperr.RequireBodyID(entityIDField, entityID); err != nil {
		return taggableRow{}, err
	}
	// READ on the vocabulary, not update: `tag` authority now governs the
	// VOCABULARY — who may coin, rename or retire a word — and only Admin and
	// Ops hold it. Applying an existing word is not a change to the
	// vocabulary, so requiring tag.update here would have made every rep who
	// tags a company also able to invent tags.
	if err := auth.Require(ctx, "tag", principal.ActionRead); err != nil {
		return taggableRow{}, err
	}
	if !memberEntityTables[entityType] {
		return taggableRow{}, &BadInputError{Field: entityTypeField, Reason: "must be " + memberEntityVocabulary}
	}
	// UPDATE on the target, not merely read. A tag is part of the record: it
	// steers who sees it in a filter and what an automation does with it, so
	// writing one onto a record a caller may only READ would let them change
	// a record they cannot edit. This is the OBJECT grant only; the ROW gate
	// is applyTagTx's EnsureWritableLive, which still answers not-found for a
	// record that does not exist or is archived, exactly as EnsureLinkTarget
	// did — every taggable type reads workspace-wide (tableclass.go), so
	// existence-hiding here is about the row's own state, not the caller's
	// scope.
	if err := auth.Require(ctx, entityType, principal.ActionUpdate); err != nil {
		return taggableRow{}, err
	}
	var out taggableRow
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var applyErr error
		out, applyErr = applyTagTx(ctx, tx, tagID, entityType, entityID)
		return applyErr
	})
	return out, err
}

// ApplyTagTx is ApplyTag inside a caller-opened transaction.
//
// The importer needs it: a record it creates and the tag that files it must
// land together, or a crash between the two leaves a row in the estate that
// the batch it belongs to cannot find. Same gates in the same order — the
// caller runs them before opening its transaction, which is where the object
// grants belong.
func (s *Store) ApplyTagTx(ctx context.Context, tx pgx.Tx, tagID ids.TagID, entityType string, entityID ids.UUID) (taggableRow, error) {
	if err := httperr.RequireBodyID(entityIDField, entityID); err != nil {
		return taggableRow{}, err
	}
	if err := auth.Require(ctx, "tag", principal.ActionRead); err != nil {
		return taggableRow{}, err
	}
	if !memberEntityTables[entityType] {
		return taggableRow{}, &BadInputError{Field: entityTypeField, Reason: "must be " + memberEntityVocabulary}
	}
	if err := auth.Require(ctx, entityType, principal.ActionUpdate); err != nil {
		return taggableRow{}, err
	}
	return applyTagTx(ctx, tx, tagID, entityType, entityID)
}

// applyTagTx is the write itself, shared by both entry points above so a change
// to what an application records reaches the HTTP caller and the importer
// together.
//
// The gates are the callers' — each runs them before reaching here, because a
// transaction is the wrong place to discover a caller had no authority.
func applyTagTx(ctx context.Context, tx pgx.Tx, tagID ids.TagID, entityType string, entityID ids.UUID) (taggableRow, error) {
	var out taggableRow
	err := func() error {
		var archived *time.Time
		err := tx.QueryRow(ctx, `SELECT archived_at FROM tag WHERE id = $1`, tagID).Scan(&archived)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && archived != nil) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		// Tagging a record CHANGES it: a caller whose only authority over the
		// row is READ must not be able to re-file it. EnsureWritableLive keeps
		// the existence-hiding EnsureLinkTarget gave — it runs the same
		// live-visibility probe first and only narrows with the write arm —
		// so a record that does not exist or is archived still answers
		// not-found rather than confirming it exists by refusing differently.
		if err := auth.EnsureWritableLive(ctx, tx, entityType, entityID); err != nil {
			return err
		}
		by, byKind := assigningPrincipal(ctx)
		row := tx.QueryRow(ctx, `
			INSERT INTO taggable (tag_id, entity_type, entity_id, assigned_by, assigned_by_kind)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (tag_id, entity_type, entity_id) DO NOTHING
			RETURNING id, tag_id, entity_type, entity_id, created_at`,
			tagID, entityType, entityID, by, byKind)
		err = row.Scan(&out.ID, &out.TagID, &out.EntityType, &out.EntityID, &out.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("already tagged: %w", apperrors.ErrConflict)
		}
		if err != nil {
			return err
		}
		_, err = storekit.AuditEvent(ctx, tx, "update", "tag", tagID.UUID, map[string]any{
			"applied": map[string]any{"entity_type": entityType, "entity_id": entityID},
		})
		return err
	}()
	return out, err
}

// RemoveTag takes ONE tag off ONE record, leaving the tag itself alone.
//
// The vocabulary had no way back. ApplyTag added a tagging and ArchiveTag
// retired a tag from the whole workspace, so the only way to undo a mistaken
// tag was to retire the tag for everybody — which is not undo, it is a second
// mistake with a wider blast radius.
//
// Same gates as applying, for the same reasons: the target's own UPDATE
// object grant, and EnsureWritableLive because taking a tag off a record
// changes it — a caller who may only see the row must not be able to. A
// record that does not exist or is archived still answers not-found rather
// than confirming it exists by refusing differently.
// Removing a tagging that is not there is NOT an error — the caller asked for
// a state, and the state is already true (idempotent by intent, which is what
// makes a retry safe).
func (s *Store) RemoveTag(ctx context.Context, tagID ids.TagID, entityType string, entityID ids.UUID) error {
	if err := httperr.RequireBodyID(entityIDField, entityID); err != nil {
		return err
	}
	if err := auth.Require(ctx, "tag", principal.ActionRead); err != nil {
		return err
	}
	if !memberEntityTables[entityType] {
		return &BadInputError{Field: entityTypeField, Reason: "must be " + memberEntityVocabulary}
	}
	// UPDATE on the target, the same gate applying holds: taking a tag off a
	// record changes the record, and a caller who may only read it must not be
	// able to. This is the OBJECT grant only; the ROW scope is
	// EnsureWritableLive below.
	if err := auth.Require(ctx, entityType, principal.ActionUpdate); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		var archived *time.Time
		err := tx.QueryRow(ctx, `SELECT archived_at FROM tag WHERE id = $1`, tagID).Scan(&archived)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && archived != nil) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		// Same reasoning as applyTagTx: removing a tag CHANGES the record too,
		// so the gate is write authority, not merely visibility.
		if err := auth.EnsureWritableLive(ctx, tx, entityType, entityID); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			DELETE FROM taggable WHERE tag_id = $1 AND entity_type = $2 AND entity_id = $3`,
			tagID, entityType, entityID)
		if err != nil {
			return err
		}
		// Audited only when something was actually removed: an audit row for a
		// tagging that was never there describes an event that did not happen.
		if tag.RowsAffected() == 0 {
			return nil
		}
		_, err = storekit.AuditEvent(ctx, tx, "update", "tag", tagID.UUID, map[string]any{
			"removed": map[string]any{"entity_type": entityType, "entity_id": entityID},
		})
		return err
	})
}

// EnsureTaggable refuses a record this caller may not tag.
//
// Split out so a caller can ask BEFORE it creates anything: apply-by-name used
// to mint the tag first and check the record second, leaving a live audited
// word behind when the record turned out not to exist or to sit outside the
// caller's row scope.
//
// The gate is exactly ApplyTag's own: UPDATE on the target's object type (not
// merely read — tagging changes the record) and EnsureWritableLive for the
// row. Asking a weaker question here than the real write asks would make this
// pre-flight worse than useless: it would admit a caller apply-by-name then
// spends a ResolveTag lookup on, only to have the write itself refuse them
// afterward.
func (s *Store) EnsureTaggable(ctx context.Context, entityType string, entityID ids.UUID) error {
	if !memberEntityTables[entityType] {
		return &BadInputError{Field: entityTypeField, Reason: "must be " + memberEntityVocabulary}
	}
	if err := auth.Require(ctx, entityType, principal.ActionUpdate); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		return auth.EnsureWritableLive(ctx, tx, entityType, entityID)
	})
}

// maxTagNameRunes bounds a tag name. Counted in RUNES, not bytes: the limit is
// about what a person can read on a badge, and a byte cap would let an English
// name run half again as long as a German one.
const maxTagNameRunes = 64

// NormalizeTagName is the ONE spelling rule every write path shares: create,
// rename, and the name an agent resolves against. Trim the ends, collapse any
// run of inner whitespace to one space.
//
// It exists because a vocabulary that holds "Key Account" and "Key  Account"
// has stopped being one, and the two are indistinguishable to a reader. The
// uniqueness index folds case; nothing folded spacing, so the collision it is
// there to prevent could still be typed.
//
// Held by: TestNormalizeTagName (collections/tagname_test.go)
func NormalizeTagName(name string) string {
	return strings.Join(strings.Fields(name), " ")
}

// ValidateTagName applies the rule and says why a name is refused. An empty
// name and an over-long one are the caller's to fix, so both name the field.
func ValidateTagName(name string) (string, error) {
	normalized := NormalizeTagName(name)
	if normalized == "" {
		return "", &BadInputError{Field: "name", Reason: "must not be empty"}
	}
	if utf8.RuneCountInString(normalized) > maxTagNameRunes {
		return "", &BadInputError{
			Field:  "name",
			Reason: fmt.Sprintf("must be at most %d characters", maxTagNameRunes),
		}
	}
	return normalized, nil
}

func tagSummary(t tagRow) TagSummary {
	out := TagSummary{TagID: t.ID.UUID, Name: t.Name, Archived: t.ArchivedAt != nil}
	if t.Color != nil {
		out.Color = *t.Color
	}
	return out
}

// TagVocabulary lists the workspace's tags for a caller outside this module.
//
// It reports TRUNCATION, because the read is capped at catalogCap and the
// caller's whole reason for asking is to find out whether a word already
// exists. A capped list handed over as if it were the vocabulary answers "no
// such tag" for every word past the cap — the exact false negative that makes
// a caller coin a duplicate, which is what reading the vocabulary was meant to
// prevent.
func (s *Store) TagVocabulary(ctx context.Context, includeArchived bool) ([]TagSummary, bool, error) {
	filter := storekit.LiveOnly
	if includeArchived {
		filter = storekit.IncludeArchived
	}
	rows, truncated, err := s.ListTags(ctx, filter)
	if err != nil {
		return nil, false, err
	}
	out := make([]TagSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, tagSummary(r))
	}
	return out, truncated, nil
}

// NewTag creates one and answers it in the cross-module shape.
func (s *Store) NewTag(ctx context.Context, name, color string) (TagSummary, error) {
	var colorArg *string
	if color != "" {
		colorArg = &color
	}
	row, err := s.CreateTag(ctx, name, colorArg)
	if err != nil {
		return TagSummary{}, err
	}
	return tagSummary(row), nil
}
