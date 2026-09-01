// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// What a merge card needs to NAME a record, read a set at a time.
//
// The decision lane renders up to ten duplicate pairs and has to name twenty
// records. Through the ordinary gets that is twenty transactions, each a full
// composite read — every email, every domain, every column — to produce a
// display name, one line of detail and a count. This reads one entity type in
// one transaction, selecting only what the card shows.
//
// What does NOT change is the gate. A dedupe queue row proves a pair was
// detected, never that this reader may see what it points at, so every id here
// goes through the same object grant and the same row scope the single read
// applies. The difference is that the scope is asked SET-WISE (auth.VisibleSubset)
// rather than once per row, and a record the reader may not see is simply absent
// from the answer — which is the same thing its refusal meant.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MergeFace is one record as a merge card shows it: what to call it, one line
// that tells it from its twin, when it arrived, and how much hangs off it.
//
// Detail is the field a reader actually uses to tell two near-identical records
// apart — a company's domain, a person's address — never an id.
type MergeFace struct {
	Label        string
	Detail       string
	CreatedAt    time.Time
	RelatedCount *int
}

// DescribeForMerge names records of ONE entity type, in one transaction.
//
// The returned map holds an entry for every id the reader may see and NO entry
// for the rest: a row-scope miss is an absence, which is what its not-found
// refusal meant — existence stays hidden either way.
//
// The OBJECT grant is still an error, because it is not about these ids. It
// reaches the caller as ErrPermissionDenied, the same answer the single read
// gives, and the caller maps it the same way.
func (s *Store) DescribeForMerge(
	ctx context.Context, entityType string, rowIDs []ids.UUID,
) (map[ids.UUID]MergeFace, error) {
	if len(rowIDs) == 0 {
		return map[ids.UUID]MergeFace{}, nil
	}
	if !isMergeFaceEntity(entityType) {
		return nil, fmt.Errorf("people: %q has no merge face: %w", entityType, apperrors.ErrNotFound)
	}
	if err := auth.Require(ctx, entityType, principal.ActionRead); err != nil {
		return nil, err
	}
	faces := make(map[ids.UUID]MergeFace, len(rowIDs))
	err := s.tx(ctx, func(tx pgx.Tx) error {
		visible, err := auth.VisibleSubset(ctx, tx, entityType, rowIDs)
		if err != nil {
			return err
		}
		allowed := make([]ids.UUID, 0, len(rowIDs))
		for _, id := range rowIDs {
			if visible[id] {
				allowed = append(allowed, id)
			}
		}
		if len(allowed) == 0 {
			return nil
		}
		return readMergeFaces(ctx, tx, entityType, allowed, faces)
	})
	if err != nil {
		return nil, err
	}
	return faces, nil
}

// isMergeFaceEntity is the closed set this read serves. Named rather than left
// to the switch below so an unknown type is refused before a transaction is
// opened, and refused the same way whatever the caller asked for.
func isMergeFaceEntity(entityType string) bool {
	switch entityType {
	case entityPerson, entityOrganization, entityLead:
		return true
	default:
		return false
	}
}

func readMergeFaces(
	ctx context.Context, tx pgx.Tx, entityType string, rowIDs []ids.UUID, into map[ids.UUID]MergeFace,
) error {
	switch entityType {
	case entityPerson:
		return readPersonFaces(ctx, tx, rowIDs, into)
	case entityOrganization:
		return readOrganizationFaces(ctx, tx, rowIDs, into)
	default:
		return readLeadFaces(ctx, tx, rowIDs, into)
	}
}

// readPersonFaces names people, with the first email as the detail line.
//
// ORDER BY position, created_at, and DISTINCT ON the person: the same order
// attachPersonEmails imposes, so the address a card shows is the one the record
// page calls first. A different pick here would make the merge screen name a
// person by an address they are not otherwise known by.
func readPersonFaces(ctx context.Context, tx pgx.Tx, rowIDs []ids.UUID, into map[ids.UUID]MergeFace) error {
	rows, err := tx.Query(ctx, `
		SELECT p.id, p.full_name, p.created_at, e.email
		FROM person p
		LEFT JOIN LATERAL (
			SELECT pe.email FROM person_email pe
			WHERE pe.person_id = p.id AND pe.archived_at IS NULL
			ORDER BY pe.position, pe.created_at
			LIMIT 1
		) e ON true
		WHERE p.id = ANY($1) AND p.archived_at IS NULL`, rowIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var face MergeFace
		var email *string
		if err := rows.Scan(&id, &face.Label, &face.CreatedAt, &email); err != nil {
			return err
		}
		if email != nil {
			face.Detail = *email
		}
		into[id] = face
	}
	return rows.Err()
}

// readOrganizationFaces names accounts, with the first domain as the detail and
// the visible contact count as the roll-up.
//
// ORDER BY is_primary DESC, created_at matches attachOrgDomains, for the reason
// the person read matches its own. The count is the caller's to see: it runs
// under the same person row scope fillContactCounts applies, since a number that
// moves when a colleague captures a private contact discloses that contact.
func readOrganizationFaces(ctx context.Context, tx pgx.Tx, rowIDs []ids.UUID, into map[ids.UUID]MergeFace) error {
	args := []any{rowIDs}
	arg := func(v any) int { args = append(args, v); return len(args) }
	edgeBound, err := auth.RelationshipEndpointScope(ctx, "rel", arg)
	if err != nil {
		return err
	}
	if edgeBound != "" {
		edgeBound = " AND " + edgeBound
	}
	scope, err := auth.ScopeClauseFor(ctx, "person", "cp", arg)
	if err != nil {
		return err
	}
	if scope != "" {
		scope = " AND " + scope
	}
	rows, err := tx.Query(ctx, `
		SELECT o.id, o.display_name, o.created_at, d.domain,
		       (SELECT count(*)
		          FROM relationship rel
		          JOIN person cp ON cp.id = rel.person_id AND cp.archived_at IS NULL
		         WHERE rel.organization_id = o.id
		           AND rel.kind = 'employment'
		           AND `+CurrentPrimaryEmploymentSQL("rel")+`
		           AND rel.archived_at IS NULL`+edgeBound+scope+`)
		FROM organization o
		LEFT JOIN LATERAL (
			SELECT od.domain FROM organization_domain od
			WHERE od.organization_id = o.id AND od.archived_at IS NULL
			ORDER BY od.is_primary DESC, od.created_at
			LIMIT 1
		) d ON true
		WHERE o.id = ANY($1) AND o.archived_at IS NULL`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var face MergeFace
		var domain *string
		var contacts int
		if err := rows.Scan(&id, &face.Label, &face.CreatedAt, &domain, &contacts); err != nil {
			return err
		}
		if domain != nil {
			face.Detail = *domain
		}
		count := contacts
		face.RelatedCount = &count
		into[id] = face
	}
	return rows.Err()
}

// readLeadFaces names leads. The detail prefers the email and falls back to the
// company: a lead often carries one and not the other, and either tells it from
// its twin better than nothing does.
func readLeadFaces(ctx context.Context, tx pgx.Tx, rowIDs []ids.UUID, into map[ids.UUID]MergeFace) error {
	rows, err := tx.Query(ctx, `
		SELECT id, full_name, email, company_name, created_at
		FROM lead WHERE id = ANY($1) AND archived_at IS NULL`, rowIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var face MergeFace
		var fullName, email, company *string
		if err := rows.Scan(&id, &fullName, &email, &company, &face.CreatedAt); err != nil {
			return err
		}
		if fullName != nil {
			face.Label = *fullName
		}
		switch {
		case email != nil:
			face.Detail = *email
		case company != nil:
			face.Detail = *company
		}
		into[id] = face
	}
	return rows.Err()
}
