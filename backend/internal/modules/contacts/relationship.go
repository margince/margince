// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Relationship edges (data-model §5): employment (contact↔company), deal
// stakeholders (deal↔contact), and company↔company partner edges. An edge's
// visibility derives from its ENDPOINTS — every non-null endpoint must
// be visible to the caller, on read exactly as on write, so an edge can
// never leak a record its ends would hide. Mutations emit the anchor
// entity's .updated event (the catalog has no relationship.* family;
// an employment change IS a contact-profile change).

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
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
	anchorContact     = "contact"
	anchorDeal        = "deal"
	anchorCompanyKind = "company"
)

func relationshipAnchor(kind string) (object, column string) {
	switch kind {
	case employmentKind, worksWithKind:
		return anchorContact, "contact_id"
	case "deal_stakeholder":
		return anchorDeal, "deal_id"
	case ProjectStakeholderKind, ProjectCompanyKind:
		return projectObjectName, "project_id"
	default: // partner_of, referred_by, co_sell_with, billing_contact
		return anchorCompanyKind, "company_id"
	}
}

// relationshipEndpoints is the set of records an edge hangs on, in the one
// shape both of its writers already hold: the create input, and the stored row
// an update or an archive names. It exists so the anchor rule below is asked
// once for all three verbs — two spellings of "which record does this edge
// annotate" would drift, and the half that drifted would be an ungated write.
type relationshipEndpoints struct {
	contact *ids.ContactID
	company *ids.CompanyID
	deal    *ids.DealID
	project *ids.ProjectID
}

func (r relationshipRow) endpoints() relationshipEndpoints {
	return relationshipEndpoints{contact: r.ContactID, company: r.CompanyID, deal: r.DealID, project: r.ProjectID}
}

func (in CreateRelationshipInput) endpoints() relationshipEndpoints {
	return relationshipEndpoints{contact: in.ContactID, company: in.CompanyID, deal: in.DealID, project: in.ProjectID}
}

// relationshipAnchorRow names the ROW an edge's authority is taken on: the
// anchor object of its kind, and the endpoint id identifying the record the
// edge annotates.
//
// This is the ONLY place the four endpoint columns are mapped to their kinds.
// The gate below, the audit subject and the reversal's edge facts all read the
// anchor through it, because they used to spell the same switch three times —
// and a rule about which record an edge belongs to, held in three places, is
// how the gate came to reason about an anchor the writers had already chosen
// differently.
//
// A nil id means the edge's endpoints do not match its kind; every kind's shape
// CHECK (migration 0001, rel_*_shape) makes its anchor endpoint NOT NULL, so
// callers treat it as the fault the database would have answered.
func relationshipAnchorRow(kind string, e relationshipEndpoints) (object string, id *ids.UUID) {
	anchor, _ := relationshipAnchor(kind)
	switch anchor {
	case anchorContact:
		return anchor, untypedPtr(e.contact)
	case anchorDeal:
		return anchor, untypedPtr(e.deal)
	case projectObjectName:
		return anchor, untypedPtr(e.project)
	default:
		return anchor, untypedPtr(e.company)
	}
}

// anchorIDOf names the record whose authority governs a STORED edge, and the
// object it is. An edge with no anchor id is a row no kind's shape admits, and
// answering about a record that is not there would put an audit row on nothing
// and light a button on a write that cannot happen.
//
// It answers an internal fault where ensureRelationshipAnchorWritable answers
// RelationshipShapeError on the same nil, and the difference is where the nil
// CAME FROM rather than two minds about one rule. There the endpoints arrived
// on a request and the caller can correct them. Here they came off a stored row
// whose rel_*_shape CHECK already guarantees the anchor endpoint is NOT NULL —
// so a nil is the database disagreeing with itself, and a refusal telling the
// caller to fix a field they never sent would send them after the wrong thing.
func anchorIDOf(row relationshipRow) (object string, id ids.UUID, err error) {
	object, found := relationshipAnchorRow(row.Kind, row.endpoints())
	if found == nil {
		return object, ids.UUID{}, fmt.Errorf("contacts: %s edge %s names no %s to anchor on", row.Kind, row.ID, object)
	}
	return object, *found, nil
}

// ensureRelationshipAnchorWritable is the ROW half of the anchor gate, and the
// half the generic surface used to leave out.
//
// auth.Require(anchor, update) says this seat may change CONTACTS — not that it
// may change THIS contact. Every anchor object is an identity table, read by
// every seat in the workspace, so the endpoint probe an edge already takes is
// existence-only for all of them and the write arm is the only thing that
// scopes the record at all. Without this an ordinary seat could demote another
// team's primary-employer edge, forge a partner edge on their company, or
// staff their project, through POST/PATCH/DELETE /v1/relationships — reaching
// past exactly the authority the dedicated verbs demand (ensureProjectWritable
// on the stakeholder roster, auth.EnsureWritable in UpdateContact).
//
// Live, not merely visible: an edge is something NEW on the anchor, and
// archived means frozen.
//
// A nil id is an edge whose endpoints do not match its kind. Every kind's shape
// CHECK makes its anchor endpoint NOT NULL, so that is the fault the database
// would answer, refused here before the gate can silently not run.
//
// Held by: TestEveryGenericRelationshipVerbTakesTheAnchorRowGate
// (backend/internal/modules/contacts/relationshipanchorgate_test.go)
func ensureRelationshipAnchorWritable(
	ctx context.Context, tx pgx.Tx, kind string, e relationshipEndpoints,
) error {
	object, anchorID := relationshipAnchorRow(kind, e)
	if anchorID == nil {
		return &RelationshipShapeError{Kind: kind}
	}
	return auth.EnsureWritableLive(ctx, tx, object, *anchorID)
}

// The kinds the GENERIC relationship surface admits.
//
// project_company is deliberately absent. A company's place on a project
// carries a rule this surface cannot keep: a project must keep at least one
// company, and only the dedicated endpoints (projectcompany.go) count them, so
// admitting the kind here would let a caller archive the last one through
// DELETE /v1/relationships.
//
// The other rule that used to keep it out — write authority over the project
// ROW rather than the project.update object grant — is kept for every kind now,
// by ensureRelationshipAnchorWritable. That is where it belonged: an anchor is
// an anchor whatever the kind, and stating it per-kind is what let
// project_stakeholder through the same door this paragraph closed.
var relationshipKinds = map[string]bool{
	employmentKind: true, "deal_stakeholder": true, ProjectStakeholderKind: true,
	"partner_of": true, "referred_by": true, "co_sell_with": true,
	worksWithKind: true, BillingContactKind: true,
}

// worksWithKind is the one contact↔contact kind: two external contacts a rep
// asserts work together. Undirected in fact — contact_id and
// counterparty_contact_id carry no order, and the unique index keeps one pair
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

const relationshipColumns = `id, kind, contact_id, company_id, counterparty_company_id, counterparty_contact_id, deal_id, project_id,
	role, is_current_primary, started_at, ended_at, source, captured_by, version, created_at, updated_at, archived_at`

type relationshipRow struct {
	ID                    ids.UUID // no RelationshipKind in the kernel vocabulary: edges stay untyped
	Kind                  string
	ContactID             *ids.ContactID
	CompanyID             *ids.CompanyID
	CounterpartyCompanyID *ids.CompanyID
	CounterpartyContact   *ids.ContactID
	DealID                *ids.DealID
	ProjectID             *ids.ProjectID
	Role                  *string
	IsCurrentPrimary      bool
	StartedAt             *time.Time
	EndedAt               *time.Time
	Source                string
	CapturedBy            string
	Version               int64
	CreatedAt             time.Time
	UpdatedAt             time.Time
	ArchivedAt            *time.Time
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
		&out.ID, &out.Kind, &out.ContactID, &out.CompanyID, &out.CounterpartyCompanyID,
		&out.CounterpartyContact, &out.DealID, &out.ProjectID, &out.Role, &out.IsCurrentPrimary, &out.StartedAt, &out.EndedAt,
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

func (s *Store) UpdateRelationship(ctx context.Context, id ids.UUID, in UpdateRelationshipInput) (relationshipRow, error) {
	// Who is editing. The row becomes theirs — see the captured_by assignment
	// in the statement below.
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return relationshipRow{}, err
	}
	if err := auth.Require(ctx, "relationship", principal.ActionUpdate); err != nil {
		return relationshipRow{}, err
	}
	var out relationshipRow
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// The row lock makes the state read and the update below one
		// race-free unit.
		// The per-contact lock comes FIRST, before the row lock, and that order is
		// the whole reason it is taken from an unlocked peek rather than from the
		// row this patch is about. Create takes the advisory lock and then touches
		// rows; if this path took the row lock first and the advisory second, a
		// create holding the advisory and waiting on a row would deadlock against
		// an update holding that row and waiting on the advisory.
		//
		// An unlocked read is enough to supply the KEY because a patch cannot move
		// an edge's endpoints — contact_id is immutable here, so the value cannot
		// change under the lock it selects. Anything else about the row is read
		// again below, under the row lock, which is what makes that read
		// authoritative.
		if err := lockContactForEmployment(ctx, tx, id); err != nil {
			return err
		}
		if _, err := storekit.LockRow(ctx, tx, "relationship", id, storekit.LiveOnly); err != nil {
			return err
		}
		current, err := s.visibleRelationship(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := refusePatch(ctx, tx, current, in); err != nil {
			return err
		}
		// The incumbent is demoted only when the patched row will actually HOLD
		// the flag, so this statement asks the SAME question as the UPDATE below
		// that grants it — one predicate, employment.IsCurrentSQL, read against the
		// database's clock in both. Spell the rule a second time here (a Go-side
		// `current.EndedAt == nil`, say) and the two answers part company over a
		// notice period: this row keeps the flag, the incumbent keeps it too, and
		// uq_rel_current_primary_employer 409s a patch the create path honours.
		if in.IsCurrentPrimary != nil && *in.IsCurrentPrimary &&
			current.Kind == employmentKind && current.ContactID != nil {
			if _, err := tx.Exec(ctx, `
				UPDATE relationship SET is_current_primary = false
				WHERE contact_id = $1 AND id <> $2 AND `+employment.CurrentPrimarySlotSQL("")+`
				  AND EXISTS (
					SELECT 1 FROM relationship patched
					 WHERE patched.id = $2
					   AND `+employment.IsCurrentSQL("coalesce($3, patched.ended_at)")+`)`,
				*current.ContactID, id, in.EndedAt); err != nil {
				return err
			}
		}
		row := tx.QueryRow(ctx, `
			UPDATE relationship SET
			  role = coalesce($2, role),
			  -- The edit is the confirmation. A seat an agent proposed carries
			  -- that agent as its captured_by, which is what marks it unconfirmed
			  -- on the page; a contact changing the row has answered the question
			  -- themselves, so the row becomes theirs. Left alone, a corrected
			  -- seat went on claiming to be a machine's unreviewed reading
			  -- forever.
			  captured_by = coalesce($6, captured_by),
			  -- An employment somebody has LEFT is not their CURRENT primary
			  -- one, whichever half of the patch makes it so: ending the job
			  -- clears the flag, and setting the flag on a job already over
			  -- does not take. Written against the row rather than as a Go
			  -- condition, so the two halves cannot drift apart. LEFT, not
			  -- "has a date" — see employment.IsCurrentSQL.
			  is_current_primary = coalesce($3, is_current_primary)
			    AND (kind <> 'employment' OR `+employment.IsCurrentSQL("coalesce($5, ended_at)")+`),
			  started_at = coalesce($4, started_at),
			  ended_at = coalesce($5, ended_at)
			WHERE id = $1
			RETURNING `+relationshipColumns,
			id, in.Role, in.IsCurrentPrimary, in.StartedAt, in.EndedAt, capturedBy)
		if out, err = scanRelationship(row); err != nil {
			// Through the SAME constraint mapping the insert uses. A patch can
			// violate rel_dates exactly as a create can — moving ended_at behind
			// started_at — and without this the two verbs answered one rule two
			// ways: a named refusal on create, the generic constraint net on
			// update. current.Kind, because a patch cannot change the kind.
			return mapRelationshipConstraint(err, current.Kind)
		}
		// Only when the patch ENDED the employment. A caller sending
		// is_current_primary = false is STATING that this is not the contact's
		// primary employer, and promoting the same row straight back would
		// overrule a human with the answer they had just rejected — the one
		// case where leaving the slot empty is the product doing as it was told.
		// Whether the end date has actually arrived is the statement's own
		// question, asked once there: an employment ending next month keeps the
		// flag, keeps the slot, and the promotion below finds nothing to do.
		if out.Kind == employmentKind && out.ContactID != nil && in.EndedAt != nil {
			if err := promoteLoneSurvivingEmployment(ctx, tx, *out.ContactID); err != nil {
				return err
			}
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
		if err := auth.Require(ctx, anchorObject, principal.ActionUpdate); err != nil {
			return err
		}
		return ensureRelationshipAnchorWritable(ctx, tx, current.Kind, current.endpoints())
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
		// The per-contact employment lock, FIRST — before any row this path
		// touches, which is the order create and update take it in. Two writers
		// taking the same pair in opposite orders is a deadlock.
		//
		// This path used to take none, because it freed the current-primary slot
		// and never decided anything from a read of it. It decides now: retiring
		// the primary edge promotes the survivor when exactly one remains, and a
		// create landing between that read and its write would leave two
		// employments and one arbitrary primary.
		if err := lockContactForEmployment(ctx, tx, id); err != nil {
			return err
		}
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
		// Same rule as create: removing an edge is editing its anchor, and it
		// owes the row half for the same reason the other two verbs do.
		anchorObject, _ := relationshipAnchor(current.Kind)
		if err := auth.Require(ctx, anchorObject, principal.ActionUpdate); err != nil {
			return err
		}
		if err := ensureRelationshipAnchorWritable(ctx, tx, current.Kind, current.endpoints()); err != nil {
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
		// The archived edge no longer holds the flag, and the contact may still
		// work somewhere. Same call as the patch path, and the same reason for
		// making it: the statement asks whether there is anything to promote.
		if out.Kind == employmentKind && out.ContactID != nil {
			if err := promoteLoneSurvivingEmployment(ctx, tx, *out.ContactID); err != nil {
				return err
			}
		}
		return emitRelationshipChangeWithEvidence(ctx, tx, "archive", nil, out, evidence)
	})
	return out, err
}
