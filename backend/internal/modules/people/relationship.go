// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Relationship edges (data-model §5): employment (person↔org), deal
// stakeholders (deal↔person), and org↔org partner edges. An edge's
// visibility derives from its ENDPOINTS — every non-null endpoint must
// be visible to the caller, on read exactly as on write, so an edge can
// never leak a record its ends would hide. Mutations emit the anchor
// entity's .updated event (the catalog has no relationship.* family;
// an employment change IS a person-profile change).

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// relationshipAnchor names the endpoint whose lifecycle a kind
// annotates — the entity whose .updated event a mutation emits and
// whose RBAC object gates it.
//
// The four anchor objects are named here because this is what produces them:
// every switch that reads an anchor is reading one of these four answers.
const (
	anchorPerson = "person"
	anchorDeal   = "deal"
	anchorOrg    = "organization"
)

func relationshipAnchor(kind string) (object, column string) {
	switch kind {
	case employmentKind, worksWithKind:
		return anchorPerson, "person_id"
	case "deal_stakeholder":
		return anchorDeal, "deal_id"
	case ProjectStakeholderKind, ProjectCompanyKind:
		return projectObjectName, "project_id"
	default: // partner_of, referred_by, co_sell_with
		return anchorOrg, "organization_id"
	}
}

// The kinds the GENERIC relationship surface admits.
//
// project_company is deliberately absent. A company's place on a project
// carries two rules this surface cannot keep: the caller needs write authority
// over the PROJECT ROW, not merely the project.update object grant this file
// takes, and a project must keep at least one company. Both live on the
// dedicated endpoints (projectcompany.go), so admitting the kind here would be
// a side door around them — a caller could attach a company to a project they
// cannot write, or archive the last one, through POST /v1/relationships.
var relationshipKinds = map[string]bool{
	"employment": true, "deal_stakeholder": true, "project_stakeholder": true,
	"partner_of": true, "referred_by": true, "co_sell_with": true,
	worksWithKind: true,
}

// worksWithKind is the one person↔person kind: two external contacts a rep
// asserts work together. Undirected in fact — person_id and
// counterparty_person_id carry no order, and the unique index keeps one pair
// one row whichever way it was recorded.
const worksWithKind = "works_with"

// refuseGenericProjectCompany is the same rule at the WRITE paths, because a
// kind is only checked on create: an update or an archive names an existing
// row, and a project_company row exists whatever this map says.
func refuseGenericProjectCompany(kind string) error {
	if kind != ProjectCompanyKind {
		return nil
	}
	return &RelationshipKindError{Kind: kind}
}

const relationshipColumns = `id, kind, person_id, organization_id, counterparty_org_id, counterparty_person_id, deal_id, project_id,
	role, is_current_primary, started_at, ended_at, source, captured_by, version, created_at, updated_at, archived_at`

type relationshipRow struct {
	ID                 ids.UUID // no RelationshipKind in the kernel vocabulary: edges stay untyped
	Kind               string
	PersonID           *ids.PersonID
	OrganizationID     *ids.OrganizationID
	CounterpartyOrgID  *ids.OrganizationID
	CounterpartyPerson *ids.PersonID
	DealID             *ids.DealID
	ProjectID          *ids.ProjectID
	Role               *string
	IsCurrentPrimary   bool
	StartedAt          *time.Time
	EndedAt            *time.Time
	Source             string
	CapturedBy         string
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
	ArchivedAt         *time.Time
}

func scanRelationship(r pgx.Row) (relationshipRow, error) {
	return scanRelationshipWithPrior(r, nil, nil)
}

// scanRelationshipWithPrior reads an edge, and optionally one trailing column: a
// value the same statement carried out of the row as it stood BEFORE the write.
// An upsert cannot be asked afterwards whether it inserted or replaced — the row
// looks identical either way — so the answer has to travel with it.
//
// prior is a **string because the column is nullable twice over: there may have
// been no prior row at all, which is exactly the case the caller reads it for.
// inserted takes the statement's own answer to which branch of an upsert ran.
func scanRelationshipWithPrior(r pgx.Row, prior **string, inserted *bool) (relationshipRow, error) {
	var out relationshipRow
	targets := []any{
		&out.ID, &out.Kind, &out.PersonID, &out.OrganizationID, &out.CounterpartyOrgID,
		&out.CounterpartyPerson, &out.DealID, &out.ProjectID, &out.Role, &out.IsCurrentPrimary, &out.StartedAt, &out.EndedAt,
		&out.Source, &out.CapturedBy, &out.Version, &out.CreatedAt, &out.UpdatedAt, &out.ArchivedAt,
	}
	if prior != nil {
		targets = append(targets, prior)
	}
	if inserted != nil {
		targets = append(targets, inserted)
	}
	return out, r.Scan(targets...)
}

type UpdateRelationshipInput struct {
	Role             *string
	IsCurrentPrimary *bool
	StartedAt        *time.Time
	EndedAt          *time.Time
	IfVersion        *int64
	// Evidence lands on the audit row as context ABOUT the write, never as a
	// field image. The reversal path names the entry it put back there.
	Evidence map[string]any
}

// lockPersonForEmployment serializes every writer of one person's employment
// flags, which is what the demote-then-grant pair below needs to be one unit.
// Without it two patches on DIFFERENT employments of the same person each read
// "no primary elsewhere" and each grant the flag, and the second commit answers
// 409 on uq_rel_current_primary_employer — the same race the create path already
// closed, reached through the other verb.
//
// Silent for anything that is not an employment: a deal stakeholder or a partner
// edge shares none of this state, and taking a person lock for one would
// serialize writes that never contend.
func lockPersonForEmployment(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	var personID *ids.PersonID
	err := tx.QueryRow(ctx,
		`SELECT person_id FROM relationship WHERE id = $1 AND kind = 'employment'`, id).Scan(&personID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Not an employment, or gone. Either way the row lock below is what
		// reports it, and it reports it the same way it always has.
		return nil
	case err != nil:
		return fmt.Errorf("people: reading the employment's person for the write lock: %w", err)
	case personID == nil:
		return nil
	}
	return storekit.LockWriteIdentity(ctx, tx, "employment", personID.String())
}

func (s *Store) UpdateRelationship(ctx context.Context, id ids.UUID, in UpdateRelationshipInput) (relationshipRow, error) {
	if err := auth.Require(ctx, "relationship", principal.ActionUpdate); err != nil {
		return relationshipRow{}, err
	}
	var out relationshipRow
	err := s.tx(ctx, func(tx pgx.Tx) error {
		// The row lock makes the state read and the update below one
		// race-free unit.
		// The per-person lock comes FIRST, before the row lock, and that order is
		// the whole reason it is taken from an unlocked peek rather than from the
		// row this patch is about. Create takes the advisory lock and then touches
		// rows; if this path took the row lock first and the advisory second, a
		// create holding the advisory and waiting on a row would deadlock against
		// an update holding that row and waiting on the advisory.
		//
		// An unlocked read is enough to supply the KEY because a patch cannot move
		// an edge's endpoints — person_id is immutable here, so the value cannot
		// change under the lock it selects. Anything else about the row is read
		// again below, under the row lock, which is what makes that read
		// authoritative.
		if err := lockPersonForEmployment(ctx, tx, id); err != nil {
			return err
		}
		if _, err := storekit.LockRow(ctx, tx, "relationship", id, storekit.LiveOnly); err != nil {
			return err
		}
		current, err := s.visibleRelationship(ctx, tx, id)
		if err != nil {
			return err
		}
		// A kind is only checked on CREATE, and this path names an existing row
		// — so the exclusion has to be re-stated here or the generic surface
		// becomes the side door the vocabulary closed.
		if err := refuseGenericProjectCompany(current.Kind); err != nil {
			return err
		}
		// Same rule as create: editing an edge is editing its anchor.
		anchorObject, _ := relationshipAnchor(current.Kind)
		if err := auth.Require(ctx, anchorObject, principal.ActionUpdate); err != nil {
			return err
		}
		if in.IfVersion != nil && *in.IfVersion != current.Version {
			return apperrors.ErrVersionSkew
		}
		// The incumbent is demoted only when the patched row will actually HOLD
		// the flag, so this statement asks the SAME question as the UPDATE below
		// that grants it — one predicate, EmploymentIsCurrentSQL, read against the
		// database's clock in both. Spell the rule a second time here (a Go-side
		// `current.EndedAt == nil`, say) and the two answers part company over a
		// notice period: this row keeps the flag, the incumbent keeps it too, and
		// uq_rel_current_primary_employer 409s a patch the create path honours.
		if in.IsCurrentPrimary != nil && *in.IsCurrentPrimary &&
			current.Kind == "employment" && current.PersonID != nil {
			if _, err := tx.Exec(ctx, `
				UPDATE relationship SET is_current_primary = false
				WHERE person_id = $1 AND id <> $2 AND `+CurrentPrimarySlotSQL("")+`
				  AND EXISTS (
					SELECT 1 FROM relationship patched
					 WHERE patched.id = $2
					   AND `+EmploymentIsCurrentSQL("coalesce($3, patched.ended_at)")+`)`,
				*current.PersonID, id, in.EndedAt); err != nil {
				return err
			}
		}
		row := tx.QueryRow(ctx, `
			UPDATE relationship SET
			  role = coalesce($2, role),
			  -- An employment somebody has LEFT is not their CURRENT primary
			  -- one, whichever half of the patch makes it so: ending the job
			  -- clears the flag, and setting the flag on a job already over
			  -- does not take. Written against the row rather than as a Go
			  -- condition, so the two halves cannot drift apart. LEFT, not
			  -- "has a date" — see EmploymentIsCurrentSQL.
			  is_current_primary = coalesce($3, is_current_primary)
			    AND (kind <> 'employment' OR `+EmploymentIsCurrentSQL("coalesce($5, ended_at)")+`),
			  started_at = coalesce($4, started_at),
			  ended_at = coalesce($5, ended_at)
			WHERE id = $1
			RETURNING `+relationshipColumns,
			id, in.Role, in.IsCurrentPrimary, in.StartedAt, in.EndedAt)
		if out, err = scanRelationship(row); err != nil {
			// Through the SAME constraint mapping the insert uses. A patch can
			// violate rel_dates exactly as a create can — moving ended_at behind
			// started_at — and without this the two verbs answered one rule two
			// ways: a named refusal on create, the generic constraint net on
			// update. current.Kind, because a patch cannot change the kind.
			return mapRelationshipConstraint(err, current.Kind)
		}
		return emitRelationshipChangeWithEvidence(ctx, tx, "update",
			relationshipFieldImage(current), out, in.Evidence)
	})
	return out, err
}

// RefuseArchiveRelationship answers every authority refusal
// ArchiveRelationship would answer with, and writes nothing — the stage-time
// half, so a staged approval is never spent on an edge the store was always
// going to refuse. It runs BOTH of the archive's probes, because an edge's
// authority is two questions: the edge must be visible through its endpoints,
// and removing it is editing its anchor.
func (s *Store) RefuseArchiveRelationship(ctx context.Context, id ids.UUID) error {
	if err := auth.Require(ctx, "relationship", principal.ActionDelete); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		current, err := s.visibleRelationship(ctx, tx, id)
		if err != nil {
			return err
		}
		anchorObject, _ := relationshipAnchor(current.Kind)
		return auth.Require(ctx, anchorObject, principal.ActionUpdate)
	})
}

// ArchiveRelationship retires one edge, conditioned on ifVersion wherever the
// caller's authority named a version.
//
// The write moved off `UPDATE … RETURNING` and onto the guarded patch, so the
// row is read back rather than returned by the statement. That costs one SELECT
// and buys the version clause — and IncludeArchived is required on the read
// back for the reason the statement's RETURNING never had to think about: the
// row this reads is the one just archived.
func (s *Store) ArchiveRelationship(ctx context.Context, id ids.UUID, ifVersion *int64) (relationshipRow, error) {
	return s.archiveRelationshipWithEvidence(ctx, id, ifVersion, nil)
}

// archiveRelationship is the archive carrying audit evidence — the reversal path
// names the entry it put back, and every other caller records none.
func (s *Store) archiveRelationshipWithEvidence(ctx context.Context, id ids.UUID, ifVersion *int64,
	evidence map[string]any,
) (relationshipRow, error) {
	if err := auth.Require(ctx, "relationship", principal.ActionDelete); err != nil {
		return relationshipRow{}, err
	}
	var out relationshipRow
	err := s.tx(ctx, func(tx pgx.Tx) error {
		current, err := s.visibleRelationship(ctx, tx, id)
		if err != nil {
			return err
		}
		// The same re-statement as the update path, and it matters more here:
		// archiving through this surface would take a company off a project
		// with no last-company check at all.
		if err := refuseGenericProjectCompany(current.Kind); err != nil {
			return err
		}
		// Same rule as create: removing an edge is editing its anchor.
		anchorObject, _ := relationshipAnchor(current.Kind)
		if err := auth.Require(ctx, anchorObject, principal.ActionUpdate); err != nil {
			return err
		}
		p := storekit.NewPatch()
		p.Set("archived_at", nil, time.Now().UTC())
		if err := p.ApplyGuarded(ctx, tx, "relationship", id, ifVersion); err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `SELECT `+relationshipColumns+` FROM relationship WHERE id = $1`, id)
		if out, err = scanRelationship(row); errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		} else if err != nil {
			return err
		}
		return emitRelationshipChangeWithEvidence(ctx, tx, "archive", nil, out, evidence)
	})
	return out, err
}
