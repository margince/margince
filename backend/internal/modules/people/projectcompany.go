// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The companies a project is worked by, as `relationship` rows of kind
// project_company.
//
// A project is work several companies do together — a customer, a partner, a
// subcontractor — so the companies are edges rather than one column on the
// project. They ride this table for the same reason the stakeholders do: it
// already carries a role, a source, the audit columns and the archive
// semantics an edge needs, and a second table would be a second answer to "who
// is on this project".
//
// Both entry points take the CALLER's transaction rather than opening their
// own, because both are halves of somebody else's write: the attach is part of
// creating a project, and the read is part of assembling a page that must
// describe one instant. modules/projects reaches them through the ports in its
// companyseam.go, which compose binds — a module never imports a sibling.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// projectCompanySource marks an edge written as part of a project's own
// lifecycle rather than through the relationship surface directly.
const projectCompanySource = "project"

// projectCompanyDefaultRole is what a company is to a project when the caller
// names no role: its client. The same role the migration backfill used, so a
// project created before the edge existed and one created after read alike.
const projectCompanyDefaultRole = "customer"

// relationshipRoleField is the audit key the edge's role is recorded under,
// spelled once so the three writers cannot disagree about it.
// Held by: TestAClaimedSpellingIsTheOnlySpellingWhereItIsUsed (backend/gates/claimedspelling_test.go)
const relationshipRoleField = "role"

// AttachCompanyToProjectTx puts one company on a project, inside the caller's
// transaction.
//
// Idempotent by the uniqueness index rather than by a pre-check: two concurrent
// attaches can both read "no edge", and the index is the only thing that can
// settle which one wins. ON CONFLICT DO NOTHING makes the loser's answer the
// state it wanted — the company IS on the project — instead of a failure that
// depends on timing.
//
// The caller's authority over the project is taken by the caller: this runs
// inside a create the project store has already gated, and inside an attach the
// handler gates. Taking it again here would re-probe a row the caller already
// holds a lock on.
func AttachCompanyToProjectTx(
	ctx context.Context, tx pgx.Tx,
	projectID ids.ProjectID, organizationID ids.OrganizationID, role, by string,
) error {
	// The company is a client-supplied reference to a row-scoped record, so
	// naming it is a read of it: a caller who cannot see a company may not put
	// a project's work on it.
	if err := auth.EnsureLinkTarget(ctx, tx, "organization", organizationID.UUID); err != nil {
		return err
	}
	// The same one statement the attach surface uses, so a project's first
	// company and a company added later are written one way — including the
	// audit and outbox rows, which ride the caller's transaction.
	return setCompanyRoleTx(ctx, tx, projectID, organizationID, role, by, false)
}

// CompaniesOnProjectTx lists the companies on a project in the order they were
// attached, inside the caller's transaction.
//
// Every row carries the caller's organization row scope: a project readable
// across the workspace does not license reading every company on it, and a
// company the reader cannot see is omitted rather than named. That is the same
// rule the stakeholder roster keeps for people.
func CompaniesOnProjectTx(
	ctx context.Context, tx pgx.Tx, projectID ids.ProjectID,
) ([]ProjectCompany, error) {
	// Reading an edge discloses its endpoints AS A PAIR — that this project and
	// this company are working together — which is what relationship.read
	// governs; the endpoints' own grants do not cover it.
	//
	// A caller without that grant gets an EMPTY list rather than a refusal.
	// This read is one section of a project, and refusing it would refuse the
	// project: a seat that may read a project but holds no relationship grant
	// would lose the record entirely over a section it was never entitled to.
	// Withholding the section is the answer the rest of this tree gives — an
	// omitted 360 section, a masked field — and it is the one that leaves the
	// caller with what they may see.
	if err := auth.Require(ctx, "relationship", principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil, nil
		}
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	projectPos := arg(projectID)
	edge, err := auth.EdgeReadScope(ctx, "r", arg)
	if err != nil {
		return nil, err
	}
	if edge == "" {
		edge = sqlAlwaysVisible
	}
	// And the company itself is a row-scoped record, so one the caller may not
	// open is omitted rather than named.
	scope, err := auth.ScopeClauseFor(ctx, "organization", "o", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = sqlAlwaysVisible
	}
	rows, err := tx.Query(ctx, storekit.SQLf(`
		SELECT r.organization_id, o.display_name, coalesce(r.role, '')
		  FROM relationship r
		  JOIN organization o ON o.id = r.organization_id
		 WHERE r.kind = '`+ProjectCompanyKind+`' AND r.project_id = $%d
		   AND r.archived_at IS NULL AND o.archived_at IS NULL
		   AND (%s) AND (%s)
		 ORDER BY r.created_at, r.id`, projectPos, edge, scope), args...)
	if err != nil {
		return nil, fmt.Errorf("read the companies on the project: %w", err)
	}
	defer rows.Close()
	var out []ProjectCompany
	for rows.Next() {
		var one ProjectCompany
		if err := rows.Scan(&one.OrganizationID, &one.DisplayName, &one.Role); err != nil {
			return nil, fmt.Errorf("read one company on the project: %w", err)
		}
		out = append(out, one)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the companies on the project: %w", err)
	}
	return out, nil
}

// ProjectCompany is one company's place on a project.
type ProjectCompany struct {
	OrganizationID ids.OrganizationID
	DisplayName    string
	Role           string
}

// SetProjectCompanyInput is one company's place on a project.
type SetProjectCompanyInput struct {
	ProjectID      ids.ProjectID
	OrganizationID ids.OrganizationID
	Role           string
}

// SetProjectCompany puts a company on a project, or re-roles the edge that
// already exists, and answers the companies on the project afterwards.
//
// The write and the read share ONE transaction so the answer describes the
// state this call produced: two attaches racing would otherwise each report a
// list assembled after the other's write, and neither reader would see what
// they did.
func (s *Store) SetProjectCompany(ctx context.Context, in SetProjectCompanyInput) ([]ProjectCompany, error) {
	if err := httperr.RequireBodyID(siteReadOrgKey, in.OrganizationID.UUID); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, "relationship", principal.ActionCreate); err != nil {
		return nil, err
	}
	// The edge annotates its anchor: without the project's write grant, an edge
	// would be an RBAC side door onto it. A project is readable across the
	// workspace, so the row half is what stops any seat staffing any team's
	// project.
	if err := auth.Require(ctx, projectObjectName, principal.ActionUpdate); err != nil {
		return nil, err
	}
	role := in.Role
	if role == "" {
		role = projectCompanyDefaultRole
	}
	// captured_by is stamped from the authenticated principal, never from the
	// request body — the write shape's rule, taken before the transaction opens
	// because a refusal here is about the caller, not about the row.
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return nil, err
	}
	var out []ProjectCompany
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureWritableLive(ctx, tx, projectObjectName, in.ProjectID.UUID); err != nil {
			return err
		}
		if err := setCompanyRoleTx(ctx, tx, in.ProjectID, in.OrganizationID, role, by, true); err != nil {
			return err
		}
		var readErr error
		out, readErr = CompaniesOnProjectTx(ctx, tx, in.ProjectID)
		return readErr
	})
	return out, err
}

// setCompanyRoleTx writes the edge: attach when it is absent, re-role when it
// is present. One statement does both, because a pre-check and a write are two
// instants and a concurrent attach lands between them.
// announce says whether the edge announces itself as a change to the project.
// The attach surface does; the project's own create does not, because
// project.created already says a project appeared and names its company.
func setCompanyRoleTx(
	ctx context.Context, tx pgx.Tx,
	projectID ids.ProjectID, organizationID ids.OrganizationID, role, by string, announce bool,
) error {
	if err := auth.EnsureLinkTarget(ctx, tx, "organization", organizationID.UUID); err != nil {
		return err
	}
	// RETURNING carries the written row into the write shape below: the audit
	// and outbox rows commit with the edge, in this transaction, like every
	// other mutation in this tree.
	// The CTE reads the edge as it stands BEFORE the upsert, from the same
	// statement: it is what says whether this attach created the edge or moved
	// an existing one's role, and what the moved role was. Both are unknowable
	// afterwards — the row looks identical either way.
	//
	// The kind is a LITERAL in both the insert and the conflict predicate, not a
	// bind parameter. uq_rel_project_company is a PARTIAL index, and Postgres
	// infers a partial index only from a predicate it can prove matches — a
	// parameter is opaque to that proof, so the same words with $1 in them raise
	// 42P10, "no unique or exclusion constraint matching the ON CONFLICT
	// specification", and every attach fails.
	var priorRole *string
	var inserted bool
	scan := tx.QueryRow(ctx,
		`WITH was AS (
		   SELECT role FROM relationship
		    WHERE kind = '`+ProjectCompanyKind+`' AND project_id = $1
		      AND organization_id = $2 AND archived_at IS NULL
		 )
		 INSERT INTO relationship (kind, project_id, organization_id, role, source, captured_by)
		 VALUES ('`+ProjectCompanyKind+`', $1, $2, $3, $4, $5)
		 ON CONFLICT (project_id, organization_id)
		   WHERE kind = '`+ProjectCompanyKind+`' AND archived_at IS NULL
		   DO UPDATE SET role = EXCLUDED.role
		 RETURNING `+relationshipColumns+`, (SELECT was.role FROM was), xmax = 0`,
		projectID, organizationID, role, projectCompanySource, by)
	row, err := scanRelationshipWithPrior(scan, &priorRole, &inserted)
	if err != nil {
		return fmt.Errorf("put the company on the project: %w", err)
	}
	if !announce {
		// The project's own create already announces itself, and its
		// project.created payload carries the company. A second event saying
		// the project changed in the same transaction that made it is noise a
		// consumer has to learn to ignore.
		// Audited without a second event: the audit row is what makes the edge
		// answerable later, and project.created is the announcement.
		if _, err := storekit.Audit(ctx, tx, "create", "relationship", row.ID, nil,
			map[string]any{relationshipKindField: row.Kind, relationshipRoleField: row.Role}); err != nil {
			return fmt.Errorf("audit the project's company: %w", err)
		}
		return nil
	}
	// An attach that found no edge MADE one, and saying "update" of a row that
	// did not exist puts a change into the project's history that never
	// happened. xmax answers which branch ran, from the statement itself: the
	// CTE's snapshot cannot, because an edge another transaction committed
	// after it was taken is one this insert still resolves as a conflict.
	if inserted {
		return emitRelationshipChange(ctx, tx, "create", nil, row)
	}
	// The edge as it stood: every column this write leaves alone still holds
	// what the returned row holds, and role is the one it moved. Handed over
	// whole so the seam narrows ONCE — a pair pre-narrowed to role alone would
	// be diffed again against the full image, and the columns missing from it
	// would read as changes from nothing.
	before := relationshipFieldImage(row)
	before[relationshipRoleField] = priorRole
	return emitRelationshipChange(ctx, tx, "update", before, row)
}

// RemoveProjectCompany takes a company off a project by archiving the edge.
//
// The LAST company is refused. A project no company is on is work nobody is
// doing, and it disappears from every company page that could lead a reader
// back to it — so the refusal is the only answer that leaves the record
// reachable. The count and the archive share one transaction, or two
// concurrent removals each see two companies and both proceed.
func (s *Store) RemoveProjectCompany(ctx context.Context, projectID ids.ProjectID, organizationID ids.OrganizationID) error {
	if err := auth.Require(ctx, "relationship", principal.ActionDelete); err != nil {
		return err
	}
	if err := auth.Require(ctx, projectObjectName, principal.ActionUpdate); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureWritableLive(ctx, tx, projectObjectName, projectID.UUID); err != nil {
			return err
		}
		// Whether this company is on the project at all comes FIRST: a company
		// that was never here is not-found, and answering "you cannot take off
		// the last one" instead tells the caller something false about a
		// project they have not touched.
		//
		// FOR UPDATE on the project's edges, so a second removal waits rather
		// than counting the same two companies this one is about to reduce.
		var live, mine int
		if err := tx.QueryRow(ctx,
			`SELECT count(*), count(*) FILTER (WHERE organization_id = $3) FROM (
			   SELECT organization_id FROM relationship
			    WHERE kind = $1 AND project_id = $2 AND archived_at IS NULL
			    FOR UPDATE) held`,
			ProjectCompanyKind, projectID, organizationID).Scan(&live, &mine); err != nil {
			return fmt.Errorf("count the companies on the project: %w", err)
		}
		if mine == 0 {
			return apperrors.ErrNotFound
		}
		if live <= 1 {
			return &LastProjectCompanyError{}
		}
		// A deal names a company AND a project, and the two must agree — the
		// deal_project_same_org trigger enforces that on deal writes, but a
		// company leaving the project is not a deal write, so nothing would stop
		// this from stranding those deals in a state the trigger forbids.
		//
		// Refusing names the count: the fix is to move those deals or leave the
		// company on, and both are decisions a person makes.
		var stranded int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM deal
			  WHERE project_id = $1 AND organization_id = $2 AND archived_at IS NULL`,
			projectID, organizationID).Scan(&stranded); err != nil {
			return fmt.Errorf("count the deals this company holds on the project: %w", err)
		}
		if stranded > 0 {
			return &CompanyHasDealsOnProjectError{Deals: stranded}
		}
		row, err := scanRelationship(tx.QueryRow(ctx,
			`UPDATE relationship SET archived_at = now()
			  WHERE kind = $1 AND project_id = $2 AND organization_id = $3 AND archived_at IS NULL
			  RETURNING `+relationshipColumns,
			ProjectCompanyKind, projectID, organizationID))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("take the company off the project: %w", err)
		}
		return emitRelationshipChange(ctx, tx, "archive", nil, row)
	})
}

// LastProjectCompanyError maps to 422: a project keeps at least one company.
type LastProjectCompanyError struct{}

func (e *LastProjectCompanyError) Error() string {
	return "a project keeps at least one company; add another before taking this one off"
}

// FieldFault names the company the caller tried to remove.
func (e *LastProjectCompanyError) FieldFault() (field, code, message string) {
	return siteReadOrgKey, "last_project_company", e.Error()
}

// Company answers which company this row names. It, Name and OnProjectAs
// together satisfy modules/projects' CompanyRow: that module asks for the three
// facts a project needs about a company, and this row supplies them without
// either module importing the other's type.
func (p ProjectCompany) Company() ids.OrganizationID { return p.OrganizationID }

// Name answers what the company is called, for a reader.
func (p ProjectCompany) Name() string { return p.DisplayName }

// OnProjectAs answers what the company is TO the project — its role.
func (p ProjectCompany) OnProjectAs() string { return p.Role }

// CompanyHasDealsOnProjectError maps to 422: the company still holds deals on
// this project, and a deal must name a company the project is worked by.
type CompanyHasDealsOnProjectError struct{ Deals int }

func (e *CompanyHasDealsOnProjectError) Error() string {
	return fmt.Sprintf("this company still has %d deal(s) on the project; "+
		"move or close them before taking the company off", e.Deals)
}

// FieldFault names the company the caller tried to take off.
func (e *CompanyHasDealsOnProjectError) FieldFault() (field, code, message string) {
	return siteReadOrgKey, "company_has_deals_on_project", e.Error()
}
