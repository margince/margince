// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Creating a relationship edge, in the two shapes a caller needs it: opening
// its own transaction, or borrowing one whose other writes must land with the
// edge or not at all. Both take the same gates in the same order and then meet
// in one writer, because the current-primary-employer rules below are the kind
// that drift silently — a second spelling would keep inserting rows while
// answering a different question about which employment is current.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CreateRelationshipInput is one edge to write: its kind, the endpoints that
// kind anchors, and the optional facts an employment carries.
type CreateRelationshipInput struct {
	Kind              string
	PersonID          *ids.PersonID
	OrganizationID    *ids.OrganizationID
	CounterpartyOrgID *ids.OrganizationID
	DealID            *ids.DealID
	ProjectID         *ids.ProjectID
	Role              *string
	// IsCurrentPrimary is TRI-STATE, and the third state is what makes the rule
	// in the insert safe: nil means the caller expressed no opinion and the
	// store decides, false means they said this is NOT the person's current
	// primary employment. Collapsing the two would silently invert a choice the
	// caller can see themselves making — the person rail's "current employer"
	// checkbox sends exactly that false.
	IsCurrentPrimary *bool
	StartedAt        *time.Time
	EndedAt          *time.Time
	Source           string
}

// CreateRelationship writes one edge in a transaction of its own.
func (s *Store) CreateRelationship(ctx context.Context, in CreateRelationshipInput) (relationshipRow, error) {
	// A SUPPLIED kind outside the vocabulary is a different fault from an omitted
	// one, and they used to answer the same sentence: a caller who sent
	// kind="EMPLOYMENT" was told `kind` is required, which is factually wrong about
	// a field they can see in their own request. The case-sensitivity trap makes
	// that land in practice, so the refusal names the allowed set.
	if in.Kind == "" {
		return relationshipRow{}, &RequiredFieldError{Field: relationshipKindField}
	}
	if !relationshipKinds[in.Kind] {
		return relationshipRow{}, &RelationshipKindError{Kind: in.Kind}
	}
	anchorObject, _ := relationshipAnchor(in.Kind)
	if err := auth.Require(ctx, "relationship", principal.ActionCreate); err != nil {
		return relationshipRow{}, err
	}
	if err := auth.Require(ctx, anchorObject, principal.ActionUpdate); err != nil {
		// The edge annotates its anchor: without the anchor's write
		// grant, an edge would be an RBAC side door onto it.
		return relationshipRow{}, err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return relationshipRow{}, err
	}

	var out relationshipRow
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = writeRelationshipInTx(ctx, tx, in, capturedBy)
		return err
	})
	return out, err
}

// CreateRelationshipTx is CreateRelationship for a caller that already opened a
// transaction — one whose own write must land with this edge or not at all.
// Same gates in the same order; only the transaction is borrowed, exactly as
// CreatePersonTx borrows one.
func (s *Store) CreateRelationshipTx(ctx context.Context, tx pgx.Tx, in CreateRelationshipInput) (relationshipRow, error) {
	if in.Kind == "" {
		return relationshipRow{}, &RequiredFieldError{Field: relationshipKindField}
	}
	if !relationshipKinds[in.Kind] {
		return relationshipRow{}, &RelationshipKindError{Kind: in.Kind}
	}
	anchorObject, _ := relationshipAnchor(in.Kind)
	if err := auth.Require(ctx, "relationship", principal.ActionCreate); err != nil {
		return relationshipRow{}, err
	}
	if err := auth.Require(ctx, anchorObject, principal.ActionUpdate); err != nil {
		return relationshipRow{}, err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return relationshipRow{}, err
	}
	return writeRelationshipInTx(ctx, tx, in, capturedBy)
}

// writeRelationshipInTx is the edge's write, with the gates already taken by
// whichever entry point above the caller came through. It is one function
// rather than two copies because the primary-employer rules below are the kind
// that drift silently: a second spelling would keep inserting rows while
// answering a different question about which employment is current.
func writeRelationshipInTx(
	ctx context.Context,
	tx pgx.Tx,
	in CreateRelationshipInput,
	capturedBy string,
) (relationshipRow, error) {
	var out relationshipRow
	// Before the endpoints are checked, because the check is what this lock
	// makes true: an archive in flight either commits first and LiveOnly
	// refuses this attach, or waits and sweeps the edge this writes with
	// everything else. Without it the two can interleave into a live
	// relationship on an archived person.
	if in.PersonID != nil {
		if err := lockPersonForAttach(ctx, tx, *in.PersonID); err != nil {
			return out, err
		}
	}
	if err := ensureRelationshipEndpoints(ctx, tx, in); err != nil {
		return out, err
	}
	// Both rules below read the person's OTHER employments and then write
	// against what they read, so they are one unit per person or they are a
	// race: two concurrent unmarked employments at different companies would
	// each see no primary, each claim it, and one would come back 409 naming
	// a flag its caller never sent.
	if in.Kind == employmentKind && in.PersonID != nil {
		if err := storekit.LockWriteIdentity(ctx, tx, employmentKind, in.PersonID.String()); err != nil {
			return out, err
		}
	}
	// One current primary employer per person: demote the incumbent
	// inside the same transaction rather than failing the write. An
	// employment that arrives already OVER claims nothing, so it displaces
	// nobody — see the insert below, which refuses it the flag. A future
	// end date is a notice period and DOES displace: they work there.
	//
	// That last test is in the statement, not in Go, so it reads the same
	// clock the insert below reads. A Go-side comparison would answer a
	// different question on a server in a different timezone from the
	// database, and the two would disagree about exactly one day.
	if in.Kind == employmentKind && in.IsCurrentPrimary != nil && *in.IsCurrentPrimary &&
		in.PersonID != nil {
		if _, err := tx.Exec(ctx, `
				UPDATE relationship SET is_current_primary = false
				WHERE person_id = $1 AND `+CurrentPrimarySlotSQL("")+`
				  AND `+EmploymentIsCurrentSQL("$2::date"),
			*in.PersonID, in.EndedAt); err != nil {
			return out, err
		}
	}
	// Two rules about is_current_primary, spelled in the insert so the
	// returned row is the row that landed — a follow-up UPDATE would bump
	// the version under the caller about to read it back.
	//
	// A person's ONLY current employment is their current primary one, WHEN
	// THE CALLER SAID NOTHING ($8 IS NULL). The column defaults to false and
	// nothing else ever promotes, so without this a person with exactly one
	// employer has none marked: a state no reader of the column expects and
	// none of them can repair. A caller who sent the field keeps their
	// answer, including an explicit false — deriving over it would invert a
	// choice they can see themselves making. The subquery excludes an
	// ended-but-still-primary row as well as a current one, because
	// promoting past either would violate uq_rel_current_primary_employer.
	//
	// And an employment that arrives already ended never holds the flag,
	// however it was asked for — history being backfilled is not where
	// somebody works today. That is the same rule the UPDATE below applies,
	// and both read it off the row rather than off the request.
	row := tx.QueryRow(ctx, `
			INSERT INTO relationship (kind, person_id, organization_id, counterparty_org_id,
			                          deal_id, project_id, role, is_current_primary, started_at, ended_at, source, captured_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7,
			        coalesce($8, $1 = 'employment' AND NOT EXISTS (
			          SELECT 1 FROM relationship
			           WHERE kind = 'employment' AND person_id = $2 AND archived_at IS NULL
			             AND (`+EmploymentIsCurrentSQL("ended_at")+` OR is_current_primary)))
			          AND ($1 <> 'employment' OR `+EmploymentIsCurrentSQL("$10::date")+`),
			        $9, $10, $11, $12)
			RETURNING `+relationshipColumns,
		in.Kind, in.PersonID, in.OrganizationID, in.CounterpartyOrgID, in.DealID, in.ProjectID,
		in.Role, in.IsCurrentPrimary, in.StartedAt, in.EndedAt, in.Source, capturedBy)
	var err error
	if out, err = scanRelationship(row); err != nil {
		return out, mapRelationshipConstraint(err, in.Kind)
	}
	return out, emitRelationshipChange(ctx, tx, "create", nil, out)
}

// ensureRelationshipEndpoints validates every supplied endpoint as a
// client-supplied FK argument (H1): each named target must be visible
// under the caller's row scope before the edge lands.
func ensureRelationshipEndpoints(ctx context.Context, tx pgx.Tx, in CreateRelationshipInput) error {
	for _, ref := range []struct {
		table string
		id    *ids.UUID
	}{
		{anchorPerson, untypedPtr(in.PersonID)},
		{"organization", untypedPtr(in.OrganizationID)},
		{"organization", untypedPtr(in.CounterpartyOrgID)},
		{anchorDeal, untypedPtr(in.DealID)},
		{projectObjectName, untypedPtr(in.ProjectID)},
	} {
		if ref.id == nil {
			continue
		}
		if err := auth.EnsureLinkTarget(ctx, tx, ref.table, *ref.id); err != nil {
			return err
		}
	}
	return nil
}

// untypedPtr narrows an optional typed id back to the kernel UUID for
// the platform seams (auth, storekit) that speak untyped ids.
func untypedPtr[K ids.EntityKind](id *ids.ID[K]) *ids.UUID {
	if id == nil {
		return nil
	}
	return &id.UUID
}
