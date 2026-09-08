// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The company half of the §1.3 merge (merge.go documents the
// shared collision-aware relink rules): beyond the shared machinery it
// re-homes the company hierarchy, deal/partner attributions, and the 1:1
// partner extension.

package people

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// MergeCompany merges company source→target and returns the
// survivor. The company half additionally re-homes the hierarchy (A's
// children become B's) and the deal/partner attributions.
func (s *Store) MergeCompany(ctx context.Context, sourceID, targetID ids.CompanyID) (crmcontracts.Company, error) {
	// Same rule, same reason as MergePerson: an omitted target_id is not caught
	// by the self-merge check and would answer not-found for a survivor the
	// caller never named.
	if err := httperr.RequireBodyID(targetIDField, targetID.UUID); err != nil {
		return crmcontracts.Company{}, err
	}
	if err := auth.Require(ctx, "company", principal.ActionUpdate); err != nil {
		return crmcontracts.Company{}, err
	}
	if sourceID == targetID {
		return crmcontracts.Company{}, &MergeSelfError{}
	}
	active, err := s.activeColumns(ctx, "company")
	if err != nil {
		return crmcontracts.Company{}, err
	}

	var out crmcontracts.Company
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = mergeCompanyTx(ctx, tx, sourceID, targetID, active)
		return err
	})
	return out, err
}

// mergeCompanyTx takes the name lock and the pair lock, refuses the
// anchor in either direction, relinks every association, fills the survivor's
// gaps and retires the source — all inside the CALLER's transaction.
//
// Split out so a caller that has already written something can commit that
// write and this merge together. The dedupe queue is the one that needs it: it
// marks a candidate 'merged' and then merges, and two transactions there leave
// a candidate claiming a merge that never happened (#1970).
func mergeCompanyTx(
	ctx context.Context, tx pgx.Tx, sourceID, targetID ids.CompanyID, active []fieldcatalog.Column,
) (crmcontracts.Company, error) {
	// A merge fills the survivor's legal_name from the record it retires
	// (fillCompanySurvivorship), so it is a name writer like any other and owes
	// the same two things: the name lock BEFORE any company row lock,
	// and a re-check afterwards. Taking it here rather than beside the fill
	// is what keeps the order — LockPair is next.
	if err := lockCompanyNameWrites(ctx, tx); err != nil {
		return crmcontracts.Company{}, err
	}
	// The pair lock keeps BOTH endpoints held to commit: without it a
	// concurrent merge(target→elsewhere) archives the survivor
	// mid-merge and the relinked children point at a dead record.
	_, tgtLock, err := storekit.LockPair(ctx, tx, "company", sourceID.UUID, targetID.UUID)
	if err != nil {
		return crmcontracts.Company{}, err
	}
	src, tgt, err := mergePair(ctx, tx, "company", sourceID, targetID, readCompanyMergeState)
	if err != nil {
		return crmcontracts.Company{}, err
	}
	// AFTER the pair lock, so the answer cannot change under the merge, and
	// before anything is relinked. Either endpoint: merging the anchor away
	// retires it, and merging a customer INTO it folds their people, deals
	// and history onto the installation's own company with no way to tell
	// them apart afterwards.
	// Neither direction is open when one side is the anchor, so neither
	// message may point at the other direction as the way out: archiving the
	// duplicate is the only move that actually works.
	if err := refuseIfAnchor(ctx, tx, sourceID, "id", "it cannot be merged into another company. Archive the duplicate instead, and edit this one on the company page"); err != nil {
		return crmcontracts.Company{}, err
	}
	if err := refuseIfAnchor(ctx, tx, targetID, "target_id", "nothing can be merged into it. Archive the duplicate instead, and edit this one on the company page"); err != nil {
		return crmcontracts.Company{}, err
	}
	if err := refuseWhenBothCarryProjects(ctx, tx, sourceID, targetID); err != nil {
		return crmcontracts.Company{}, err
	}
	targetIsPartner, err := relinkCompanyAssociations(ctx, tx, sourceID, targetID)
	if err != nil {
		return crmcontracts.Company{}, err
	}
	filled, err := fillCompanySurvivorship(ctx, tx, src, tgt, targetIsPartner, tgtLock)
	if err != nil {
		return crmcontracts.Company{}, err
	}
	out, err := finalizeCompanyMerge(ctx, tx, sourceID, targetID, filled, active)
	if err != nil {
		return crmcontracts.Company{}, err
	}
	// A survivor that just inherited the retired record's legal name can now
	// be the twin of a THIRD company, and resolving one duplicate is no
	// reason to leave that one unfiled. Only when the name actually moved: a
	// merge that filled nothing renames nobody.
	//
	// It runs after finalizeCompanyMerge, which retires the source. Before it,
	// the source still reads as live AND holds the very name it has just
	// donated, so it scores 1.0, wins the ranked list, and the pair filed
	// names a row archived one statement later — a pair no human can ever
	// dispose of, because merging it answers AlreadyMerged and that reopens
	// it. The genuine third record is never reached, since the walk stops at
	// the first unfiled rival.
	if _, renamed := filled[fieldLegalName]; renamed {
		by, err := storekit.CapturedBy(ctx)
		if err != nil {
			return crmcontracts.Company{}, err
		}
		if err := recheckCompanyNameForDuplicates(ctx, tx, targetID, by); err != nil {
			return crmcontracts.Company{}, err
		}
	}
	return out, nil
}

// relinkCompanyAssociations moves every association off the merged-away company
// onto the survivor — domains (demoting a duplicate primary), relationship
// edges, activity/list/tag rows, and the deal/hierarchy/partner references
// — and reports whether the survivor ends up holding a partner row.
func relinkCompanyAssociations(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.CompanyID) (bool, error) {
	if _, err := relinkDemotingPrimary(ctx, tx, `
		UPDATE company_domain a SET company_id = $2,
		  is_primary = a.is_primary AND NOT EXISTS (
		    SELECT 1 FROM company_domain b
		    WHERE b.company_id = $2 AND b.is_primary AND b.archived_at IS NULL)
		WHERE a.company_id = $1 AND a.archived_at IS NULL`, sourceID.UUID, targetID.UUID); err != nil {
		return false, fmt.Errorf("relink domains: %w", err)
	}
	if err := relinkCompanyEdges(ctx, tx, sourceID, targetID); err != nil {
		return false, fmt.Errorf("relink relationships: %w", err)
	}
	if _, err := relinkLinkRows(ctx, tx, "company", sourceID.UUID, targetID.UUID); err != nil {
		return false, fmt.Errorf("relink activity/list/tag rows: %w", err)
	}
	// The survivor ends up with the UNION of both companies' relationship
	// types. These are child ROWS, so survivorship on the company row
	// cannot move them — they have to be re-homed here, beside the domains.
	if err := moveCompanyRelationshipTypes(ctx, tx, sourceID, targetID); err != nil {
		return false, err
	}
	return absorbCompanyReferences(ctx, tx, sourceID, targetID)
}

// fillCompanySurvivorship folds the merged-away company's fields into the survivor
// where the survivor is blank, and — when the survivor gained the 1:1 partner
// extension — asserts the 'partner' relationship type that is the other half
// of the invariant (ADR-0079 amending ADR-0032 §Decision 2). It returns the
// applied after-image for the merge audit.
//
// A merged-away partner would otherwise leave the survivor holding the
// extension row with nothing saying it is a partner, which is exactly the
// half-state the enforced invariant exists to make impossible.
func fillCompanySurvivorship(ctx context.Context, tx pgx.Tx, src, tgt crmcontracts.Company, targetIsPartner bool, tgtLock storekit.RowLock) (map[string]any, error) {
	p := storekit.NewPatch()
	fillString(p, fieldLegalName, tgt.LegalName, src.LegalName)
	fillString(p, "description", tgt.Description, src.Description)
	fillString(p, "industry", tgt.Industry, src.Industry)
	if targetIsPartner {
		if err := ensureCompanyRelationshipType(ctx, tx, ids.CompanyID{UUID: ids.UUID(tgt.Id)},
			relationshipTypePartner, "system", "system:merge"); err != nil {
			return nil, err
		}
	}
	if !p.Empty() {
		if err := p.ApplyLocked(ctx, tx, tgtLock); err != nil {
			return nil, fmt.Errorf("apply survivorship fill: %w", err)
		}
	}
	// An inherited description arrives with its author still attached. Without
	// this the survivor holds a person's sentence that field_provenance says
	// nobody wrote, and the next site read of the survivor replaces it — the
	// merge would quietly strip a human's claim on words it did not change.
	// The RETIRED record's author is carried across, not the person running the
	// merge: a merge moves a value, it does not author one.
	if _, inherited := p.After()["description"]; inherited {
		if err := carryDescriptionAuthor(ctx, tx,
			ids.CompanyID{UUID: ids.UUID(src.Id)},
			ids.CompanyID{UUID: ids.UUID(tgt.Id)}); err != nil {
			return nil, err
		}
	}
	return p.After(), nil
}

// finalizeCompanyMerge retires the merged-away company and records the merge on the
// write shape — audit row plus company.merged event in the one
// transaction — then returns the reloaded survivor.
func finalizeCompanyMerge(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.CompanyID, filled map[string]any, active []fieldcatalog.Column) (crmcontracts.Company, error) {
	if err := archiveMergedAway(ctx, tx, "company", sourceID.UUID, targetID.UUID); err != nil {
		return crmcontracts.Company{}, fmt.Errorf("retire merged-away company: %w", err)
	}
	auditID, err := storekit.Audit(ctx, tx, "merge", "company", sourceID.UUID,
		map[string]any{auditKeyMergedInto: nil},
		map[string]any{auditKeyMergedInto: targetID, auditKeyFilled: filled})
	if err != nil {
		return crmcontracts.Company{}, fmt.Errorf("audit company merge: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, sourceID.UUID, crmcontracts.PublicEventCompanyMerged{
		MergedFromId: openapi_types.UUID(sourceID.UUID),
		MergedIntoId: openapi_types.UUID(targetID.UUID),
	}); err != nil {
		return crmcontracts.Company{}, fmt.Errorf("emit company.merged: %w", err)
	}
	out, err := readCompany(ctx, tx, targetID, storekit.LiveOnly, active)
	if err != nil {
		return crmcontracts.Company{}, fmt.Errorf("read surviving company: %w", err)
	}
	return out, nil
}

// absorbCompanyReferences re-homes everything beyond the relationship and
// link tables that points at the source company — deal attributions, the
// company hierarchy, the 1:1 partner extension, and the merge redirect
// chain — and reports whether the survivor ends up holding a partner
// row (the A41 classification invariant needs to know).
func absorbCompanyReferences(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.CompanyID) (bool, error) {
	for _, stmt := range []string{
		// The project moves FIRST, and the order is load-bearing: the
		// deal_project_same_company trigger is INITIALLY IMMEDIATE, so moving a
		// deal while its project still points at the dissolved company fires it
		// mid-merge. A project's anchor is NOT NULL ... ON DELETE RESTRICT
		// and so cannot stay behind either (PROJ-LIFE-4) — leaving it is
		// what turns a healthy deal un-editable over a mismatch nobody made.
		// The legacy anchor column, kept in step so a downgrade reads something
		// sane; the live answer is the edge below.
		`UPDATE project SET company_id = $2 WHERE company_id = $1`,
		`UPDATE deal SET company_id = $2 WHERE company_id = $1`,
		`UPDATE deal SET partner_company_id = $2 WHERE partner_company_id = $1`,
		// The document library's account pointer. It is a denormalized READ
		// path, so nothing else moves it — and a file left pointing at the
		// dissolved company is filed under a record that no longer exists,
		// which reads to a user as the contract having vanished.
		`UPDATE attachment SET company_id = $2 WHERE company_id = $1`,
	} {
		if _, err := tx.Exec(ctx, stmt, sourceID, targetID); err != nil {
			return false, fmt.Errorf("repoint project and deal attributions: %w", err)
		}
	}

	// Hierarchy: if the survivor sits under the source, lift it to the
	// source's parent first — otherwise absorbing the source's
	// children would make B its own ancestor.
	if _, err := tx.Exec(ctx, `
		UPDATE company SET parent_company_id =
		  (SELECT parent_company_id FROM company WHERE id = $1)
		WHERE id = $2 AND parent_company_id = $1`, sourceID, targetID); err != nil {
		return false, fmt.Errorf("lift survivor out of source hierarchy: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE company SET parent_company_id = $2 WHERE parent_company_id = $1`,
		sourceID, targetID); err != nil {
		return false, fmt.Errorf("re-parent child companies: %w", err)
	}

	// The 1:1 partner extension moves only into a vacancy; when both
	// records carry program state the survivor's stands and the
	// source's rides its archived company untouched (recoverable, never
	// silently blended).
	var targetIsPartner bool
	if err := tx.QueryRow(ctx, `
		WITH moved AS (
		  UPDATE partner SET company_id = $2
		  WHERE company_id = $1
		    AND NOT EXISTS (SELECT 1 FROM partner WHERE company_id = $2)
		  RETURNING 1)
		SELECT EXISTS (SELECT 1 FROM moved)
		    OR EXISTS (SELECT 1 FROM partner WHERE company_id = $2)`,
		sourceID, targetID).Scan(&targetIsPartner); err != nil {
		return false, fmt.Errorf("move partner extension: %w", err)
	}
	// Earlier merged-away rows repoint too: the redirect chain stays
	// one hop deep, so following merged_into_id always lands live.
	if _, err := tx.Exec(ctx,
		`UPDATE company SET merged_into_id = $2 WHERE merged_into_id = $1`,
		sourceID, targetID); err != nil {
		return false, fmt.Errorf("repoint earlier merges: %w", err)
	}
	return targetIsPartner, nil
}

// readCompanyMergeState loads one end of a company merge: a live row
// returns itself; an archived one returns its redirect pointer (nil when
// it was plain-archived, not merged).
func readCompanyMergeState(ctx context.Context, tx pgx.Tx, id ids.CompanyID) (crmcontracts.Company, *ids.UUID, error) {
	// A merge-state read feeds the resolution decision, never the wire —
	// core columns suffice.
	o, err := readCompany(ctx, tx, id, storekit.IncludeArchived, nil)
	if err != nil {
		return crmcontracts.Company{}, nil, err
	}
	if o.ArchivedAt == nil {
		return o, nil, nil
	}
	return crmcontracts.Company{}, (*ids.UUID)(o.MergedIntoId), apperrors.ErrNotFound
}

// relinkCompanyEdges moves A's relationship edges to B across both company
// columns. Order matters: edges that would degenerate (A↔B partner
// edges, duplicates of what B already has) archive first, then the
// survivors relink.
func relinkCompanyEdges(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.CompanyID) error {
	now := time.Now().UTC()
	// An edge between the two merging companies would become a self-edge.
	if _, err := tx.Exec(ctx, `
		UPDATE relationship SET archived_at = $3
		WHERE archived_at IS NULL
		  AND ((company_id = $1 AND counterparty_company_id = $2)
		    OR (company_id = $2 AND counterparty_company_id = $1))`,
		sourceID, targetID, now); err != nil {
		return err
	}
	// Duplicates of edges the survivor already has, on either column.
	if _, err := tx.Exec(ctx, `
		UPDATE relationship a SET archived_at = $3
		WHERE a.archived_at IS NULL
		  AND (a.company_id = $1 OR a.counterparty_company_id = $1)
		  AND EXISTS (
		    SELECT 1 FROM relationship b
		    WHERE b.kind = a.kind AND b.archived_at IS NULL AND b.id <> a.id
		      AND b.person_id IS NOT DISTINCT FROM a.person_id
		      AND b.deal_id IS NOT DISTINCT FROM a.deal_id
		      -- The project too, or a project_company edge on project A is
		      -- read as a duplicate of one on project B merely because both
		      -- name the survivor — and the company silently leaves project A.
		      AND b.project_id IS NOT DISTINCT FROM a.project_id
		      AND b.company_id IS NOT DISTINCT FROM
		            (CASE WHEN a.company_id = $1 THEN $2::uuid ELSE a.company_id END)
		      AND b.counterparty_company_id IS NOT DISTINCT FROM
		            (CASE WHEN a.counterparty_company_id = $1 THEN $2::uuid ELSE a.counterparty_company_id END))`,
		sourceID, targetID, now); err != nil {
		return err
	}
	// Relinked employment edges keep ≤1 current-primary per person.
	if _, err := tx.Exec(ctx, `
		UPDATE relationship a SET company_id = $2,
		  is_current_primary = a.is_current_primary AND NOT EXISTS (
		    SELECT 1 FROM relationship b
		    WHERE b.person_id = a.person_id AND b.id <> a.id
		      AND `+employment.CurrentPrimarySlotSQL("b")+`)
		WHERE a.company_id = $1 AND a.archived_at IS NULL`, sourceID, targetID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx,
		`UPDATE relationship SET counterparty_company_id = $2
		 WHERE counterparty_company_id = $1 AND archived_at IS NULL`, sourceID, targetID)
	return err
}

// liveProjectEdge is the ONE spelling of "this company carries live work".
//
// Read through the EDGES, not project.company_id: that column stopped
// being written when a project became work several companies do together, so a
// guard reading it would let a merge through that joins two companies both
// genuinely carrying projects.
//
// Shared by the refusal below and by the read the duplicates lane makes before
// offering a Merge button. Two spellings would drift, and the way they would
// show it is a button that is offered and then refuses.
//
// Held by: TestTheCardAndTheMergeAgreeOnWhoCarriesProjects
// (backend/internal/compose/integration/projectmerge_integration_test.go)
const liveProjectEdge = `FROM relationship c
		  JOIN project p ON p.id = c.project_id AND p.archived_at IS NULL
		 WHERE c.kind = 'project_company' AND c.archived_at IS NULL`

// refuseWhenBothCarryProjects enforces PROJ-LIFE-4's ask. Two companies
// that each hold live bodies of work may, once merged, be running the same
// one twice or two genuinely different ones — and nothing in the data says
// which. Combining them silently would leave a human to find the duplicates
// later, so the merge stops and names them instead. The refusal is
// actionable and reversible: archive or re-anchor one side, then merge.
//
// Only live projects count. An archived project is a grouping already ended,
// and it relinks with everything else.
// The DECISION is unscoped and the NAMING is not, and the split is the whole
// point: work the caller cannot see must still block the merge, or a rep
// would quietly combine two companies whose projects another team owns. But
// naming a project is a read of it, so the refusal lists only the ones this
// caller may already see and counts the rest.
func refuseWhenBothCarryProjects(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.CompanyID) error {
	// Naming a project is a read of it, and the merge entry point checks only
	// company.update — nothing on this path has asked for project.read.
	// Row scope no longer narrows a project (no own/team arm in platform/auth
	// tableclass.go, and migration 1787320003 narrowed project.visibility to
	// 'workspace'), but the OBJECT grant is a separate gate and a seat can
	// hold company.update with no sight of a project at all. So the
	// naming asks for the grant it actually needs, and a caller without it is
	// still refused the merge — on counts, which say the work exists without
	// saying whose it is or what it is called.
	mayName := auth.Require(ctx, "project", principal.ActionRead) == nil

	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	sourcePos, targetPos := arg(sourceID), arg(targetID)
	// Read through the EDGES, not project.company_id: that column stopped
	// being written when a project became work several companies do together,
	// so a guard reading it would let a merge through that joins two companies
	// both genuinely carrying projects.
	rows, err := tx.Query(ctx, storekit.SQLf(`
		SELECT c.company_id, p.name
		  `+liveProjectEdge+`
		   AND c.company_id IN ($%d, $%d)
		 ORDER BY c.company_id, p.name`, sourcePos, targetPos), args...)
	if err != nil {
		return fmt.Errorf("read projects on both merge endpoints: %w", err)
	}
	defer rows.Close()

	var refusal BothCompaniesCarryProjectsError
	for rows.Next() {
		var company ids.UUID
		var name string
		if err := rows.Scan(&company, &name); err != nil {
			return err
		}
		side, names := &refusal.TargetCount, &refusal.Target
		if company == sourceID.UUID {
			side, names = &refusal.SourceCount, &refusal.Source
		}
		*side++
		if mayName {
			*names = append(*names, name)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if refusal.SourceCount > 0 && refusal.TargetCount > 0 {
		return &refusal
	}
	return nil
}

// BothCompaniesCarryProjectsError maps to 409: the merge needs a human to
// say whether the two sets of work are the same body of work or two.
// Source and Target name only the projects the caller may see; the counts
// cover every live project on each side, visible or not. A caller with no
// sight of either side still learns the merge is blocked and why, without
// learning what another team is working on.
type BothCompaniesCarryProjectsError struct {
	Source      []string
	Target      []string
	SourceCount int
	TargetCount int
}

func (e *BothCompaniesCarryProjectsError) Error() string {
	return "both companies have live projects (" +
		mergeSideSummary(e.Source, e.SourceCount) + " and " + mergeSideSummary(e.Target, e.TargetCount) +
		"); archive or re-anchor one side before merging, so the merge does not guess whether they are the same work"
}

// mergeSideSummary names what the caller may see and acknowledges the rest
// WITHOUT counting it. "Other work is live here" is as actionable as a number
// — either way the answer is to archive or re-anchor a side — but a number is
// a census: repeat the merge against every company you can reach and the
// refusals tell you how much hidden work each one carries, and how that
// changes week to week. The caller's own side is theirs to count; the other
// side's total is not something this refusal needs to settle.
func mergeSideSummary(names []string, total int) string {
	switch {
	case len(names) == 0:
		return "work you cannot see"
	case len(names) == total:
		return strings.Join(names, ", ")
	default:
		return strings.Join(names, ", ") + ", and other work you cannot see"
	}
}
