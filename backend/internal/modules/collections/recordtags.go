// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The read behind the record page's tag panel — one shape for all three
// advertised types, because the panel that draws them is one component.

// RecordTag is one tag on one record, with the assignment that put it there.
type RecordTag struct {
	TagID       ids.UUID
	Name        string
	Color       *string
	Description *string
	Archived    bool
	AssignedAt  time.Time
	// AssignedBy is zero when the assignment predates the product recording
	// it. A reader sees the date with no name rather than a name nobody chose.
	AssignedBy     ids.UUID
	AssignedByName string
	AssignedByKind string
}

// RecordTags is what one record carries, and whether the caller was allowed to
// see it at all.
type RecordTags struct {
	Data []RecordTag
	// Withheld says the caller may read the RECORD but not the vocabulary.
	// The list is then empty for a reason that is not "this record has no
	// tags", and only the flag can tell the two apart — a panel that showed
	// "No tags" here would state a fact nobody established.
	Withheld bool
}

// recordTagTypes are the types this read serves. `taggable` admits lead and
// project as well; answering for them here would ship a surface no screen
// offers, and the refusal names the field so a caller can see why.
var recordTagTypes = map[string]bool{
	typePerson:       true,
	typeOrganization: true,
	typeDeal:         true,
}

// RecordTagTypesServed answers the types this read serves, in a stable order.
// The tool surface advertises this rather than keeping its own copy: a schema
// that admitted a type the store refuses would offer a call that always fails.
func RecordTagTypesServed() []string {
	return []string{typePerson, typeOrganization, typeDeal}
}

// RecordTagsFor reads the tags on one record.
//
// Two gates, in this order. The RECORD's own read permission and row scope
// come first, so a caller naming a record outside their scope gets not-found
// whatever their tag grants say — existence stays hidden. Only then does the
// vocabulary grant decide whether the words themselves come back.
func (s *Store) RecordTagsFor(ctx context.Context, entityType string, entityID ids.UUID) (RecordTags, error) {
	if !recordTagTypes[entityType] {
		return RecordTags{}, &BadInputError{
			Field:  entityTypeField,
			Reason: "must be person, organization or deal",
		}
	}
	if err := auth.Require(ctx, entityType, principal.ActionRead); err != nil {
		return RecordTags{}, err
	}
	// The vocabulary is a separate grant, and a caller without it still gets
	// an answer about the record — just one that says the words are withheld
	// rather than pretending there are none.
	withheld := auth.Require(ctx, "tag", principal.ActionRead) != nil

	var out RecordTags
	out.Withheld = withheld
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureLinkTarget(ctx, tx, entityType, entityID); err != nil {
			return err
		}
		if withheld {
			return nil
		}
		rows, err := s.recordTagRows(ctx, tx, entityType, entityID)
		if err != nil {
			return err
		}
		out.Data = rows
		return nil
	})
	if err != nil {
		return RecordTags{}, err
	}
	return out, nil
}

// recordTagRows reads the assignments themselves, live words first.
//
// Ordering is deliberate and the panel depends on it: active tags in the order
// they were applied, most recent first, then the archived ones. A retired word
// is history and belongs after what is current, not interleaved by name.
func (s *Store) recordTagRows(ctx context.Context, tx pgx.Tx, entityType string, entityID ids.UUID) ([]RecordTag, error) {
	rows, err := tx.Query(ctx, `
		SELECT t.id, t.name, t.color, t.description, t.archived_at IS NOT NULL,
		       g.assigned_at, g.assigned_by, g.assigned_by_kind,
		       COALESCE(u.display_name, '')
		  FROM taggable g
		  JOIN tag t ON t.id = g.tag_id
		  LEFT JOIN app_user u ON u.id = g.assigned_by
		 WHERE g.entity_type = $1 AND g.entity_id = $2
		 ORDER BY t.archived_at IS NOT NULL, g.assigned_at DESC, t.name`,
		entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("collections: reading record tags: %w", err)
	}
	defer rows.Close()

	var out []RecordTag
	for rows.Next() {
		var r RecordTag
		var assignedBy *ids.UUID
		var kind *string
		if err := rows.Scan(&r.TagID, &r.Name, &r.Color, &r.Description, &r.Archived,
			&r.AssignedAt, &assignedBy, &kind, &r.AssignedByName); err != nil {
			return nil, err
		}
		if assignedBy != nil {
			r.AssignedBy = *assignedBy
		}
		if kind != nil {
			r.AssignedByKind = *kind
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
