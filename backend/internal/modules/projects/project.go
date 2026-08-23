// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

// The project write paths: create, archive, and the typed errors the
// transport maps onto contract codes. A project is the body of work a
// client relationship is made of — the deals in this module hang off it,
// which is why it lives in this bounded context rather than one of its own.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/platform/database/storekit"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
	"github.com/gradionhq/margince/backend/internal/shared/ports/fieldcatalog"
)

// projectObject is this record type's RBAC object and catalog object name,
// spelled once.
const projectObject = "project"

// PhaseInitiative is where every project is born: the ladder's head. A
// project only ever leaves it through AdvanceProjectPhase, which is what
// keeps the phase and its history in one transaction.
const PhaseInitiative = "initiative"

// PhaseClosed is the one phase that demands a reason — closing is a claim
// about the work having ended, and an unexplained claim is not answerable
// later.
const PhaseClosed = "closed"

// CreateProjectInput is one new body of work. Phase and captured_by are
// absent by design: both are the server's to decide.
type CreateProjectInput struct {
	Name string
	// Key is NOT an input: the server mints it from the name (keymint.go) and
	// createProjectTx fills this in for the response. A caller-chosen key is a
	// subject-line matcher a caller can get wrong.
	Key            *string
	OrganizationID ids.OrganizationID
	OwnerID        *ids.UserID
	Description    *string
	StartedAt      *time.Time
	TargetEndDate  *time.Time
	Source         string
	// CustomFields carries the request body's extra top-level keys
	// (additionalProperties); only active cf_* catalog columns land,
	// drop-on-mismatch (storekit customcolumns).
	CustomFields map[string]any
}

// CreateProject opens a project on a company, with its birth phase-history
// row written in the same transaction.
func (s *Store) CreateProject(ctx context.Context, in CreateProjectInput) (crmcontracts.Project, error) {
	if err := auth.Require(ctx, projectObject, principal.ActionCreate); err != nil {
		return crmcontracts.Project{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.Project{}, err
	}
	active, err := s.catalogColumns(ctx)
	if err != nil {
		return crmcontracts.Project{}, err
	}

	var out crmcontracts.Project
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = createProjectTx(ctx, tx, in, by, active, s.attachCompany, s.projectCompanies)
		return err
	})
	return out, err
}

// createProjectTx inserts the project with its birth phase-history row and
// runs the write shape, all inside the caller's transaction.
func createProjectTx(
	ctx context.Context, tx pgx.Tx, in CreateProjectInput, by string,
	active []fieldcatalog.Column, attachCompany AttachCompany, companies ProjectCompanies,
) (crmcontracts.Project, error) {
	// The anchor company is a client-supplied reference to a row-scoped
	// record, so naming it is a read of it: the caller must be able to see
	// the company before a project can be hung off it. The composite FK
	// only proves same-workspace, which is a weaker claim.
	if err := auth.EnsureLinkTarget(ctx, tx, "organization", in.OrganizationID.UUID); err != nil {
		return crmcontracts.Project{}, err
	}

	id := ids.New[ids.ProjectKind]()
	key, err := insertProjectRow(ctx, tx, id, in, by, active)
	if err != nil {
		return crmcontracts.Project{}, err
	}
	in.Key = &key

	// The company rides the SAME transaction as the row, so a project whose
	// company edge failed to land cannot commit alone — a project no company
	// page shows is a project a reader cannot find.
	if err := attachCompany(ctx, tx, id, in.OrganizationID, CompanyRoleCustomer, by); err != nil {
		return crmcontracts.Project{}, fmt.Errorf("put the project's company on it: %w", err)
	}

	// The birth row: from_phase NULL, exactly as deal_stage_history records
	// a deal's first placement. A project's history is complete from row one.
	if _, err := tx.Exec(ctx,
		`INSERT INTO project_phase_history (project_id, from_phase, to_phase, changed_by)
		 VALUES ($1, NULL, $2, $3)`,
		id, PhaseInitiative, by); err != nil {
		return crmcontracts.Project{}, fmt.Errorf("record project phase history: %w", err)
	}

	auditID, err := storekit.Audit(ctx, tx, "create", projectObject, id.UUID, nil, map[string]any{projectNameField: in.Name})
	if err != nil {
		return crmcontracts.Project{}, fmt.Errorf("audit project create: %w", err)
	}
	created := crmcontracts.PublicEventProjectCreated{
		Name:           in.Name,
		OrganizationId: openapi_types.UUID(in.OrganizationID.UUID),
		Phase:          PhaseInitiative,
	}
	if in.Key != nil {
		created.Key = in.Key
	}
	if in.OwnerID != nil {
		owner := openapi_types.UUID(in.OwnerID.UUID)
		created.OwnerId = &owner
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, created); err != nil {
		return crmcontracts.Project{}, fmt.Errorf("emit project.created: %w", err)
	}
	out, err := readProject(ctx, tx, id, storekit.LiveOnly, active)
	if err != nil {
		return crmcontracts.Project{}, fmt.Errorf("read created project: %w", err)
	}
	// The created project answers with its companies like every other single
	// read. The store-bound spelling is unavailable here because this runs
	// inside a caller-opened transaction, so the seam is threaded in.
	one := []crmcontracts.Project{out}
	if err := maskProjects(ctx, tx, one); err != nil {
		return crmcontracts.Project{}, err
	}
	return fillCompanies(ctx, tx, one[0], companies)
}

// RefuseArchiveProject answers every authority refusal ArchiveProject would
// answer with, and writes nothing. Its sibling on deal says why.
func (s *Store) RefuseArchiveProject(ctx context.Context, id ids.ProjectID) error {
	if err := auth.Require(ctx, projectObject, principal.ActionDelete); err != nil {
		return err
	}
	return s.Tx(ctx, func(tx pgx.Tx) error {
		return auth.EnsureWritable(ctx, tx, projectObject, id.UUID)
	})
}

// ArchiveProject soft-deletes a project and the grouping it provided. It
// deliberately does NOT touch the activities or deals it grouped: the
// grouping dies, the history does not. The deal's project_id is cleared by
// the FK's ON DELETE SET NULL only on a hard delete, so an archived
// project keeps its rollup readable — which is what "the history does not
// die" means in practice.
//
// ifVersion pins the row where the caller's authority named a version.
func (s *Store) ArchiveProject(ctx context.Context, id ids.ProjectID, ifVersion *int64) (crmcontracts.Project, error) {
	if err := auth.Require(ctx, projectObject, principal.ActionDelete); err != nil {
		return crmcontracts.Project{}, err
	}
	active, err := s.catalogColumns(ctx)
	if err != nil {
		return crmcontracts.Project{}, err
	}
	var out crmcontracts.Project
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureWritable(ctx, tx, projectObject, id.UUID); err != nil {
			return err
		}
		// A liveness probe, not a wire read — no custom columns needed.
		current, err := readProject(ctx, tx, id, storekit.LiveOnly, nil)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		// The archive rides the same guarded patch every other by-id write
		// uses, so the If-Match the contract accepts is actually honored:
		// archiving a project someone else just re-phased is version skew,
		// not a silent overwrite.
		p := storekit.NewPatch()
		p.Set("archived_at", current.ArchivedAt, now)
		if err := p.ApplyGuarded(ctx, tx, projectObject, id.UUID, ifVersion); err != nil {
			return fmt.Errorf("archive project: %w", err)
		}
		// The stakeholder edges are attributes of the grouping, so they go
		// with it — the people themselves are untouched.
		if _, err := tx.Exec(ctx,
			`UPDATE relationship SET archived_at = $2
			   WHERE project_id = $1 AND kind = 'project_stakeholder' AND archived_at IS NULL`,
			id, now); err != nil {
			return fmt.Errorf("archive project stakeholder edges: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`DELETE FROM list_member WHERE entity_type = 'project' AND entity_id = $1`, id); err != nil {
			return fmt.Errorf("detach list memberships: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`DELETE FROM taggable WHERE entity_type = 'project' AND entity_id = $1`, id); err != nil {
			return fmt.Errorf("detach tags: %w", err)
		}

		auditID, err := storekit.Audit(ctx, tx, "archive", projectObject, id.UUID, p.Before(), p.After())
		if err != nil {
			return fmt.Errorf("audit project archive: %w", err)
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventProjectArchived{}); err != nil {
			return fmt.Errorf("emit project.archived: %w", err)
		}
		archivedRow, err := readProject(ctx, tx, id, storekit.IncludeArchived, active)
		if err != nil {
			return fmt.Errorf("read archived project: %w", err)
		}
		out, err = s.maskProjectForCaller(ctx, tx, archivedRow)
		return err
	})
	return out, err
}

// keyRaceLost answers whether this insert failed because another transaction
// took the minted key in between. It is not a user-facing refusal: the caller
// never chose the key, so losing the race means the mint loop tries the next
// number rather than reporting a conflict.
func keyRaceLost(err error) bool {
	if constraint, ok := storekit.UniqueViolation(err); ok {
		return constraint == "uq_project_key"
	}
	// A lock timeout on the insert is the SAME race seen from the other side:
	// the holder of this key has not committed yet, so waiting longer would only
	// hold a pool connection to learn what the next number already answers.
	return storekit.IsLockTimeout(err)
}

// submittedDateField names the date input a request carried, preferring the
// one most likely to be the mover when several arrived: a caller that sent
// ended_at is closing the project, and that is the date the rule is about.
func submittedDateField(startedAt, targetEnd, endedAt *time.Time) string {
	switch {
	case endedAt != nil:
		return "ended_at"
	case startedAt != nil:
		return "started_at"
	case targetEnd != nil:
		return "target_end_date"
	default:
		return ""
	}
}

// projectCheckError names the schema-side business rules that can still fire
// after the per-path validations, so a breach reads as a 422 about a rule
// rather than an opaque server fault. dateField is the date input this request
// actually carried, so a date-range breach points at the value the caller can
// change; empty when the path submitted none.
//
// A CHECK this module has no message for answers nil, and the caller returns
// the database error itself: httperr's constraint net still answers it as a
// 422 business-rule breach, with the constraint name in the operator's log
// rather than in the client's refusal. A schema identifier tells a caller our
// table's shape and nothing it can act on. TestEveryNamedProjectCheckHasItsOwnRefusal
// is what keeps that net from being the answer for a rule a request can reach.
func projectCheckError(constraint string, dateField string) error {
	switch constraint {
	// Unreachable through a request now that the server mints the key
	// (keymint.go) and TestEveryMintedKeyFitsTheColumnsShape holds every stem
	// this generator can produce to the same shape. It stays because the
	// constraint stays: a CHECK with no refusal of its own answers as an
	// anonymous 422, and the fitness test over the named checks would be
	// satisfied by a net that tells the caller nothing.
	case "project_key_shape":
		return &ProjectKeyShapeError{}
	case "project_closed_reason":
		return &ClosedReasonRequiredError{}
	case "project_dates":
		return &ProjectDateRangeError{Field: dateField}
	case "project_phase_check":
		return &ProjectPhaseError{}
	default:
		return nil
	}
}

// ProjectKeyShapeError maps to 422: the key must be letter-led so it can
// never be a bare number, which would match dates, amounts and order
// numbers in an inbound subject line.
type ProjectKeyShapeError struct{}

func (e *ProjectKeyShapeError) Error() string {
	return "a project key must start with a letter and use only letters, digits, hyphen or underscore (2-24 characters)"
}

// FieldFault refuses a project key outside the contract's shape.
func (e *ProjectKeyShapeError) FieldFault() (field, code, message string) {
	return "key", "invalid_key", e.Error()
}

// ProjectPhaseError maps to 422: the phase is not one the lifecycle admits.
//
// The contract declares `to_phase` as an enum, but nothing between the wire and
// the table checks it — httperr.Decode reads JSON and does not validate, and
// this installation runs no request-validator middleware. So the schema CHECK is
// the first thing to refuse an unknown phase, and without this arm the caller is
// handed the constraint's own name instead of the four values it could have sent.
type ProjectPhaseError struct{}

func (e *ProjectPhaseError) Error() string {
	return "a project phase is one of initiative, pursuing, delivering or closed"
}

// FieldFault names the field the caller actually sent.
func (e *ProjectPhaseError) FieldFault() (field, code, message string) {
	return "to_phase", "invalid_phase", e.Error()
}

// ClosedReasonRequiredError maps to 422 closed_reason_required.
type ClosedReasonRequiredError struct{}

func (e *ClosedReasonRequiredError) Error() string {
	return "closing a project requires a reason"
}

// FieldFault refuses closing a project with no reason recorded.
func (e *ClosedReasonRequiredError) FieldFault() (field, code, message string) {
	return "reason", "closed_reason_required", e.Error()
}

// ProjectDateRangeError maps to 422: a project cannot end before it started.
//
// Field names whichever date the CALLER submitted. The rule is enforced by a
// schema CHECK on the resulting pair, so by the time it fires either date could
// be the one that moved — and attributing it to a constant would point a PATCH
// that moved started_at at an ended_at it never sent.
type ProjectDateRangeError struct{ Field string }

func (e *ProjectDateRangeError) Error() string {
	return "a project's end date cannot precede its start date"
}

// FieldFault refuses a date pair whose end precedes its start, naming the date
// the caller moved.
func (e *ProjectDateRangeError) FieldFault() (field, code, message string) {
	field = e.Field
	if field == "" {
		field = "ended_at"
	}
	return field, "invalid_date_range", e.Error()
}
