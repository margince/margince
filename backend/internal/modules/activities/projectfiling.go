// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What filing an activity under a project did to its retention, and whether it
// can be taken back. One judgement serves the read the screen draws from and
// the write that acts on it, so the control offered and the refusal given
// cannot disagree.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/retentionscope"
)

// causeProjectFilingUndone marks the audit row an undo writes; the read below
// finds the decisions on one activity by it.
const causeProjectFilingUndone = "project_filing_undone"

// ProjectFilingRefusedError is a refusal the caller can act on: the code says
// which rule stands in the way, and it answers 409 because the request is fine
// and the record is in a state that forbids it.
type ProjectFilingRefusedError struct {
	Code    crmcontracts.ProjectFilingRefusalCode
	Message string
}

func (e *ProjectFilingRefusedError) Error() string { return e.Message }

// MessageFault carries the code so a client can branch on the reason.
func (e *ProjectFilingRefusedError) MessageFault() (code, message string) {
	return string(e.Code), e.Message
}

func (e *ProjectFilingRefusedError) Unwrap() error { return apperrors.ErrConflict }

// projectFilingRefusals are the ways an undo is refused, one sentence each.
var projectFilingRefusals = map[crmcontracts.ProjectFilingRefusalCode]string{
	crmcontracts.ProjectFilingRefusalCodeNotFiled: "Filing under a project is not what keeps this " +
		"activity, so there is nothing to undo.",
	crmcontracts.ProjectFilingRefusalCodeRestricted: "A statutory retention hold has already started " +
		"on this activity, and a hold that has started never shortens.",
	crmcontracts.ProjectFilingRefusalCodeLegalHold: "A legal hold sits on a record this activity is " +
		"linked to, so its retention cannot be shortened until the hold is lifted.",
	crmcontracts.ProjectFilingRefusalCodeHiddenProject: "A project you cannot see still holds this " +
		"activity. Ask someone who can see it to undo the filing.",
	crmcontracts.ProjectFilingRefusalCodeOtherBasisRemains: "Something else still qualifies this " +
		"activity as commercial correspondence — a won deal, a sent offer, a controller's pin — " +
		"so it keeps its retention class.",
	crmcontracts.ProjectFilingRefusalCodeQualifyingDeal: "This activity is filed under a deal that " +
		"qualifies it as commercial correspondence, so it keeps its retention class.",
}

func refusalFor(code crmcontracts.ProjectFilingRefusalCode) *ProjectFilingRefusedError {
	return &ProjectFilingRefusedError{Code: code, Message: projectFilingRefusals[code]}
}

// filingEntry is one project filing on record. The id is absent once the
// project was deleted; the name is the one frozen when the filing qualified.
type filingEntry struct {
	projectID   *ids.UUID
	name        string
	qualifiedAt time.Time
}

// projectFilingFacts is what the judgement reads, gathered once.
type projectFilingFacts struct {
	filings       []filingEntry
	hidden        map[ids.UUID]bool // projects that exist and this caller cannot see
	restricted    bool
	legalHold     bool
	hiddenLink    bool
	otherBasis    bool
	qualifiedDeal bool
}

// refusal is the verdict, in the order the reasons outrank one another: a hold
// that has started is named before anything that merely keeps the class.
func (f projectFilingFacts) refusal() *ProjectFilingRefusedError {
	switch {
	case len(f.filings) == 0:
		return refusalFor(crmcontracts.ProjectFilingRefusalCodeNotFiled)
	case f.restricted:
		return refusalFor(crmcontracts.ProjectFilingRefusalCodeRestricted)
	case f.legalHold:
		return refusalFor(crmcontracts.ProjectFilingRefusalCodeLegalHold)
	case f.hiddenLink:
		return refusalFor(crmcontracts.ProjectFilingRefusalCodeHiddenProject)
	case f.otherBasis:
		return refusalFor(crmcontracts.ProjectFilingRefusalCodeOtherBasisRemains)
	case f.qualifiedDeal:
		return refusalFor(crmcontracts.ProjectFilingRefusalCodeQualifyingDeal)
	}
	return nil
}

// readProjectFilingFacts gathers the evidence, the holds and the links under the
// caller's transaction. The undo calls it under the row lock, so what it read is
// what the write then changes.
//
// The hold and deal questions are retentionscope's, the same text the erasure
// engine asks of the same rows, and the project-hold arm also covers the project
// an evidence row names after the link itself is gone.
func readProjectFilingFacts(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (projectFilingFacts, error) {
	var facts projectFilingFacts
	if err := tx.QueryRow(ctx, `
		SELECT a.restricted_at IS NOT NULL,
		       `+retentionscope.HeldThroughAnyLink("a.id")+`
		       OR EXISTS (SELECT 1 FROM activity_retention_evidence e JOIN project fp ON fp.id = e.project_id
		                   WHERE e.activity_id = a.id AND fp.legal_hold),
		       `+retentionscope.QualifyingDealLink("a.id")+`
		  FROM activity a WHERE a.id = $1`, id).Scan(&facts.restricted, &facts.legalHold, &facts.qualifiedDeal); err != nil {
		return facts, err
	}
	rows, err := tx.Query(ctx, `
		SELECT basis, project_id, coalesce(project_name, ''), qualified_at
		  FROM activity_retention_evidence
		 WHERE activity_id = $1
		 ORDER BY qualified_at, id`, id)
	if err != nil {
		return facts, err
	}
	defer rows.Close()
	for rows.Next() {
		var basis string
		var entry filingEntry
		if err := rows.Scan(&basis, &entry.projectID, &entry.name, &entry.qualifiedAt); err != nil {
			return facts, err
		}
		if basis != BasisProjectLinked {
			facts.otherBasis = true
			continue
		}
		facts.filings = append(facts.filings, entry)
	}
	if err := rows.Err(); err != nil {
		return facts, err
	}
	rows.Close()

	named := make([]ids.UUID, 0, len(facts.filings))
	for _, filing := range facts.filings {
		if filing.projectID != nil {
			named = append(named, *filing.projectID)
		}
	}
	if facts.hidden, err = hiddenProjects(ctx, tx, named); err != nil {
		return facts, err
	}
	facts.hiddenLink, err = holdsAHiddenProjectLink(ctx, tx, id)
	return facts, err
}

// hiddenProjects answers which of the named projects exist and are out of the
// caller's sight. A deleted project is in no result, so it reads as visible:
// there is nothing left to hide. A caller with no project read grant sees none.
func hiddenProjects(ctx context.Context, tx pgx.Tx, projects []ids.UUID) (map[ids.UUID]bool, error) {
	hidden := map[ids.UUID]bool{}
	if len(projects) == 0 {
		return hidden, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	listPos := arg(projects)
	visible := "true"
	if !auth.ReadGranted(ctx, linkEntityProject) {
		visible = "false"
	} else if clause, err := auth.ScopeClauseFor(ctx, linkEntityProject, "t", arg); err != nil {
		return nil, err
	} else if clause != "" {
		visible = clause
	}
	rows, err := tx.Query(ctx, storekit.SQLf(
		`SELECT t.id, (%s) FROM project t WHERE t.id = ANY($%d)`, visible, listPos,
	), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var projectID ids.UUID
		var seen bool
		if err := rows.Scan(&projectID, &seen); err != nil {
			return nil, err
		}
		hidden[projectID] = !seen
	}
	return hidden, rows.Err()
}

// holdsAHiddenProjectLink is true when the activity is linked to a project the
// caller cannot see. The undo can only remove a link it can see, so a hidden one
// would survive it and the database would then refuse the clear; this says so
// first, and the same way on the read.
func holdsAHiddenProjectLink(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (bool, error) {
	if !auth.ReadGranted(ctx, linkEntityProject) {
		var linked bool
		err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activity_link
			WHERE activity_id = $1 AND entity_type = 'project')`, id).Scan(&linked)
		return linked, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos := arg(id)
	clause, err := auth.LinkTargetVisibleClause(ctx, "l", arg)
	if err != nil || clause == "" {
		return false, err
	}
	var hidden bool
	err = tx.QueryRow(ctx, storekit.SQLf(`SELECT EXISTS (SELECT 1 FROM activity_link l
		WHERE l.activity_id = $%d AND l.entity_type = 'project' AND NOT %s)`, idPos, clause), args...).Scan(&hidden)
	return hidden, err
}

// linkedDealSignature identifies the set of deal links the activity carries, so a
// link added between the lock and the judgement shows as a different set.
func linkedDealSignature(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (string, error) {
	var signature string
	err := tx.QueryRow(ctx, `SELECT coalesce(md5(string_agg(l.id::text, ',' ORDER BY l.id)), '')
		  FROM activity_link l WHERE l.activity_id = $1 AND l.entity_type = 'deal'`, id).Scan(&signature)
	return signature, err
}

// shareLockQualifyingRecords takes FOR SHARE on the deals the activity is linked
// to and on their offers, and answers the signature of the set it locked. A
// deal's win and an offer's send each stamp the correspondence in their own
// transaction, and a stamp that read the class as set skips it while its
// evidence row still lands; against an undo that cleared the class in between,
// that leaves evidence on an unclassed row. Sharing the rows makes the two
// serialize: the undo either waits for the stamp and then reads its evidence, or
// the stamp waits, finds the class gone and stamps afresh.
func shareLockQualifyingRecords(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (string, error) {
	var lockedDeals, lockedOffers int
	err := tx.QueryRow(ctx, `
		WITH locked_deals AS (
		  SELECT d.id FROM deal d
		   WHERE EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = $1 AND l.deal_id = d.id)
		   ORDER BY d.id FOR SHARE),
		locked_offers AS (
		  SELECT o.id FROM offer o
		   WHERE EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = $1 AND l.deal_id = o.deal_id)
		   ORDER BY o.id FOR SHARE)
		SELECT (SELECT count(*) FROM locked_deals), (SELECT count(*) FROM locked_offers)`,
		id).Scan(&lockedDeals, &lockedOffers)
	if err != nil {
		return "", err
	}
	return linkedDealSignature(ctx, tx, id)
}

// GetProjectFiling reads one activity's project filing and the decisions already
// taken on it. Human-only like the undo it offers, and bounded by what the caller
// may see: a project out of their sight is unnamed, and a decision that touched
// one is a bare timestamp.
func (s *Store) GetProjectFiling(ctx context.Context, id ids.ActivityID) (crmcontracts.ProjectFiling, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.ProjectFiling{}, err
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return crmcontracts.ProjectFiling{}, err
	}
	var out crmcontracts.ProjectFiling
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureActivityContentVisible(ctx, tx, id.UUID); err != nil {
			return err
		}
		var err error
		out, err = projectFilingState(ctx, tx, id)
		return err
	})
	return out, err
}

// projectFilingState is the read model: the facts, the verdict on them and the
// undo decisions on record.
func projectFilingState(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (crmcontracts.ProjectFiling, error) {
	facts, err := readProjectFilingFacts(ctx, tx, id)
	if err != nil {
		return crmcontracts.ProjectFiling{}, err
	}
	undone, err := readUndoDecisions(ctx, tx, id)
	if err != nil {
		return crmcontracts.ProjectFiling{}, err
	}
	state := crmcontracts.ProjectFiling{
		Filed:    len(facts.filings) > 0,
		Projects: make([]crmcontracts.ProjectFilingEntry, 0, len(facts.filings)),
		Undone:   undone,
	}
	for _, filing := range facts.filings {
		entry := crmcontracts.ProjectFilingEntry{Name: filing.name, QualifiedAt: filing.qualifiedAt}
		if filing.projectID != nil && facts.hidden[*filing.projectID] {
			hiddenFlag := true
			entry.Name, entry.Hidden = "", &hiddenFlag
		}
		state.Projects = append(state.Projects, entry)
	}
	refusal := facts.refusal()
	state.Undoable = refusal == nil
	// A record that was never filed owes no explanation of why it cannot be undone.
	if refusal != nil && state.Filed {
		state.Refusal = &crmcontracts.ProjectFilingRefusal{Code: refusal.Code, Message: refusal.Message}
	}
	return state, nil
}

// undoDecisionEvidence is the part of the undo's audit evidence the screen shows.
type undoDecisionEvidence struct {
	DecidedByName string     `json:"decided_by_name"`
	Reason        string     `json:"reason"`
	Projects      []string   `json:"projects"`
	ProjectIDs    []ids.UUID `json:"project_ids"`
}

// readUndoDecisions lists the undos recorded for one activity, newest first,
// from the audit rows themselves: the entry shown on the activity is the entry
// the write committed, not a copy kept beside it. A decision that touched a
// project the caller cannot see keeps its moment and drops the rest, so a
// member's words about a project are not shown to somebody who cannot see it.
func readUndoDecisions(ctx context.Context, tx pgx.Tx, id ids.ActivityID) ([]crmcontracts.ProjectFilingUndoDecision, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, occurred_at, evidence FROM `+storekit.TableAudit+`
		 WHERE entity_type = 'activity' AND entity_id = $1
		   AND evidence ->> 'cause' = $2
		 ORDER BY occurred_at DESC, id DESC`, id, causeProjectFilingUndone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type recorded struct {
		id       ids.UUID
		at       time.Time
		evidence undoDecisionEvidence
	}
	var all []recorded
	var touched []ids.UUID
	for rows.Next() {
		var entry recorded
		var raw []byte
		if err := rows.Scan(&entry.id, &entry.at, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &entry.evidence); err != nil {
			return nil, fmt.Errorf("read project filing undo evidence: %w", err)
		}
		touched = append(touched, entry.evidence.ProjectIDs...)
		all = append(all, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	seen := map[ids.UUID]bool{}
	var distinct []ids.UUID
	for _, project := range touched {
		if !seen[project] {
			seen[project] = true
			distinct = append(distinct, project)
		}
	}
	hidden, err := hiddenProjects(ctx, tx, distinct)
	if err != nil {
		return nil, err
	}
	decisions := make([]crmcontracts.ProjectFilingUndoDecision, 0, len(all))
	for _, entry := range all {
		decision := crmcontracts.ProjectFilingUndoDecision{
			Id: openapi_types.UUID(entry.id), At: entry.at, ByName: entry.evidence.DecidedByName,
			Reason: entry.evidence.Reason, Projects: entry.evidence.Projects,
		}
		if slices.ContainsFunc(entry.evidence.ProjectIDs, func(project ids.UUID) bool { return hidden[project] }) {
			redacted := true
			decision = crmcontracts.ProjectFilingUndoDecision{Id: openapi_types.UUID(entry.id), At: entry.at, Redacted: &redacted, Projects: []string{}}
		}
		decisions = append(decisions, decision)
	}
	return decisions, nil
}
