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
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// projectFilingRefusals are the four ways an undo is refused, one sentence each.
var projectFilingRefusals = map[crmcontracts.ProjectFilingRefusalCode]string{
	crmcontracts.ProjectFilingRefusalCodeNotFiled: "Filing under a project is not what keeps this " +
		"activity, so there is nothing to undo.",
	crmcontracts.ProjectFilingRefusalCodeRestricted: "A statutory retention hold has already started " +
		"on this activity, and a hold that has started never shortens.",
	crmcontracts.ProjectFilingRefusalCodeOtherBasisRemains: "Something else still qualifies this " +
		"activity as commercial correspondence — a won deal, a sent offer, a controller's pin — " +
		"so it keeps its retention class.",
	crmcontracts.ProjectFilingRefusalCodeQualifyingDeal: "This activity is filed under a deal that " +
		"qualifies it as commercial correspondence, so it keeps its retention class.",
}

func refusalFor(code crmcontracts.ProjectFilingRefusalCode) *ProjectFilingRefusedError {
	return &ProjectFilingRefusedError{Code: code, Message: projectFilingRefusals[code]}
}

// projectFilingFacts is what the judgement reads, gathered once.
type projectFilingFacts struct {
	restricted    bool
	filings       []crmcontracts.ProjectFilingEntry
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
	case f.otherBasis:
		return refusalFor(crmcontracts.ProjectFilingRefusalCodeOtherBasisRemains)
	case f.qualifiedDeal:
		return refusalFor(crmcontracts.ProjectFilingRefusalCodeQualifyingDeal)
	}
	return nil
}

// readProjectFilingFacts gathers the evidence, the link and the hold under the
// caller's transaction. The undo calls it under the row lock, so what it read is
// what the write then changes.
func readProjectFilingFacts(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (projectFilingFacts, error) {
	var facts projectFilingFacts
	if err := tx.QueryRow(ctx,
		`SELECT restricted_at IS NOT NULL FROM activity WHERE id = $1`, id).Scan(&facts.restricted); err != nil {
		return facts, err
	}
	rows, err := tx.Query(ctx, `
		SELECT basis, coalesce(project_name, ''), qualified_at
		  FROM activity_retention_evidence
		 WHERE activity_id = $1
		 ORDER BY qualified_at, id`, id)
	if err != nil {
		return facts, err
	}
	defer rows.Close()
	for rows.Next() {
		var basis, name string
		var qualifiedAt time.Time
		if err := rows.Scan(&basis, &name, &qualifiedAt); err != nil {
			return facts, err
		}
		if basis != BasisProjectLinked {
			facts.otherBasis = true
			continue
		}
		facts.filings = append(facts.filings, crmcontracts.ProjectFilingEntry{
			Name: name, QualifiedAt: qualifiedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return facts, err
	}
	facts.qualifiedDeal, err = linkedDealQualifies(ctx, tx, id)
	return facts, err
}

// linkedDealQualifies asks whether a deal the activity is linked to qualifies it
// as commercial correspondence. It is judged by the rule the erasure's fallback
// applies to a row with no stamp (privacy/retention_floor.go, handelsbriefArm):
// won, or carrying an offer past draft. That module is a sibling, so the
// predicate is restated here; both say what a Handelsbrief is, and a deal linked
// after the stamp was written has no evidence row to be read instead.
func linkedDealQualifies(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (bool, error) {
	var qualifies bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM activity_link l JOIN deal d ON d.id = l.deal_id
		   WHERE l.activity_id = $1 AND l.entity_type = 'deal'
		     AND (d.status = 'won'
		          OR EXISTS (SELECT 1 FROM offer o WHERE o.deal_id = d.id AND o.status <> 'draft')))`,
		id).Scan(&qualifies)
	return qualifies, err
}

// GetProjectFiling reads one activity's project filing and the decisions already
// taken on it. Human-only like the undo it offers: the reasons are members'
// own words.
func (s *Store) GetProjectFiling(ctx context.Context, id ids.ActivityID) (crmcontracts.ProjectFiling, error) {
	if err := auth.RequireHuman(ctx); err != nil {
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
		Projects: facts.filings,
		Undone:   undone,
	}
	if state.Projects == nil {
		state.Projects = []crmcontracts.ProjectFilingEntry{}
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
	DecidedByName string   `json:"decided_by_name"`
	Reason        string   `json:"reason"`
	Projects      []string `json:"projects"`
}

// readUndoDecisions lists the undos recorded for one activity, newest first,
// from the audit rows themselves: the entry shown on the activity is the entry
// the write committed, not a copy kept beside it.
func readUndoDecisions(ctx context.Context, tx pgx.Tx, id ids.ActivityID) ([]crmcontracts.ProjectFilingUndoDecision, error) {
	rows, err := tx.Query(ctx, `
		SELECT occurred_at, evidence FROM `+storekit.TableAudit+`
		 WHERE entity_type = 'activity' AND entity_id = $1
		   AND evidence ->> 'cause' = $2
		 ORDER BY occurred_at DESC, id DESC`, id, causeProjectFilingUndone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	decisions := []crmcontracts.ProjectFilingUndoDecision{}
	for rows.Next() {
		var at time.Time
		var raw []byte
		if err := rows.Scan(&at, &raw); err != nil {
			return nil, err
		}
		var evidence undoDecisionEvidence
		if err := json.Unmarshal(raw, &evidence); err != nil {
			return nil, fmt.Errorf("read project filing undo evidence: %w", err)
		}
		decisions = append(decisions, crmcontracts.ProjectFilingUndoDecision{
			At: at, ByName: evidence.DecidedByName, Reason: evidence.Reason, Projects: evidence.Projects,
		})
	}
	return decisions, rows.Err()
}
