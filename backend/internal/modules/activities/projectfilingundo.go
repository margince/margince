// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Undoing a filing under a project: the one door out of the retention class
// StampCorrespondenceForProject wrote.
//
// The class is what shields commercial correspondence from erasure and the
// retention sweep, and over-retention is an argument to have with a supervisory
// authority where destruction is not. So the undo is a named member's decision
// with a written reason, it clears the class only when the project filing is the
// sole thing qualifying the activity, and the triggers underneath refuse the
// same clear whenever the evidence or a statutory hold says otherwise.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/statedreason"
)

// UndoDeclarationSetting is the transaction-local setting the two retention
// triggers read to admit an undo; migration 1791118500 spells the same name.
// It carries the activity id, so one declaration cannot be spent on another row.
const UndoDeclarationSetting = "margince.project_filing_undo"

// UndoProjectFiling takes an activity back out of the project it was filed
// under and withdraws the retention class that filing gave it, in one
// transaction with the audit entry and the event.
//
// Unfiling and clearing are one act on purpose: the relink verbs only replace a
// project, so nothing else removes the link, and the class may not clear while a
// project still holds the activity.
func (s *Store) UndoProjectFiling(ctx context.Context, id ids.ActivityID, reason string) (crmcontracts.ProjectFiling, error) {
	stated, err := parseUndoReason(reason)
	if err != nil {
		return crmcontracts.ProjectFiling{}, err
	}
	actor, err := admitUndoDecider(ctx)
	if err != nil {
		return crmcontracts.ProjectFiling{}, err
	}
	var out crmcontracts.ProjectFiling
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// Deals and offers are locked before the activity, the order a deal's win
		// takes (its own row, then the activity it stamps), so the two cannot
		// wait on each other. The set is read once more under the activity lock:
		// a deal linked in between was not locked, and the undo says so rather
		// than judge it unprotected.
		dealsBefore, err := shareLockQualifyingRecords(ctx, tx, id)
		if err != nil {
			return err
		}
		held, err := lockActivityForWrite(ctx, tx, id.UUID)
		if err != nil {
			return err
		}
		if err := auth.EnsureActivityWritableIn(ctx, tx, id.UUID, !held); err != nil {
			return err
		}
		dealsAfter, err := linkedDealSignature(ctx, tx, id)
		if err != nil {
			return err
		}
		if dealsBefore != dealsAfter {
			return fmt.Errorf("the activity's deals changed while the undo was being judged; retry: %w", apperrors.ErrConflict)
		}
		facts, err := readProjectFilingFacts(ctx, tx, id)
		if err != nil {
			return err
		}
		if refusal := facts.refusal(); refusal != nil {
			return refusal
		}
		name, err := deciderName(ctx, tx, actor)
		if err != nil {
			return err
		}
		if err := withdrawProjectFiling(ctx, tx, id); err != nil {
			return err
		}
		if err := recordProjectFilingUndone(ctx, tx, id, facts, name, stated); err != nil {
			return err
		}
		out, err = projectFilingState(ctx, tx, id)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return crmcontracts.ProjectFiling{}, apperrors.ErrNotFound
	}
	return out, err
}

// parseUndoReason refuses an unstated reason before any transaction opens:
// whitespace passes a required-field check and says nothing, which is the
// silent override the written reason exists to prevent.
func parseUndoReason(reason string) (string, error) {
	stated, ok := statedreason.Trim(reason)
	if !ok {
		return "", httperr.Validation("reason", "required", fmt.Sprintf(
			"undoing a project filing withdraws a retention class, so it must state why in 1–%d characters",
			statedreason.Max,
		))
	}
	return stated, nil
}

// admitUndoDecider is the gate on WHO decides: a named member. An agent never
// withdraws a retention class, even holding an administrator's passport and even
// when it filed the activity itself. The same grant that files under a project
// (activity.UPDATE) is what takes the filing back; the row-level write check
// follows under the lock.
func admitUndoDecider(ctx context.Context) (principal.Principal, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return principal.Principal{}, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID == ids.Nil {
		return principal.Principal{}, fmt.Errorf("only a named member undoes a project filing: %w", apperrors.ErrPermissionDenied)
	}
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return principal.Principal{}, err
	}
	return actor, nil
}

// deciderName is the deciding member's display name, frozen into the audit
// evidence so a deactivated account does not turn an attributed decision into an
// anonymous one.
func deciderName(ctx context.Context, tx pgx.Tx, actor principal.Principal) (string, error) {
	var name string
	err := tx.QueryRow(ctx, `SELECT display_name FROM app_user WHERE id = $1`, actor.UserID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && strings.TrimSpace(name) == "") {
		return "", &DeciderUnnamedError{}
	}
	return name, err
}

// DeciderUnnamedError refuses a decision by an account with no display name: the
// audit entry has to say who decided, and the member can fix that themselves.
type DeciderUnnamedError struct{}

func (e *DeciderUnnamedError) Error() string {
	return "the deciding account has no display name, and an unattributed decision cannot be recorded"
}

// MessageFault asks the member for the one thing that unblocks them.
func (e *DeciderUnnamedError) MessageFault() (code, message string) {
	return "decider_unnamed", e.Error() + ". Set your display name in your profile, then undo the filing again."
}

func (e *DeciderUnnamedError) Unwrap() error { return apperrors.ErrConflict }

// withdrawProjectFiling unlinks the project, deletes the filing's evidence and
// clears the class, under the declaration the triggers admit. The declaration is
// cleared again before returning so nothing later in the transaction rides it.
func withdrawProjectFiling(ctx context.Context, tx pgx.Tx, id ids.ActivityID) error {
	if _, err := tx.Exec(ctx, `SELECT set_config($1, $2, true)`, UndoDeclarationSetting, id.String()); err != nil {
		return err
	}
	if _, err := deleteVisibleLinksOfType(ctx, tx, id, linkEntityProject, linkColumn(linkEntityProject)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM activity_retention_evidence WHERE activity_id = $1 AND basis = $2`,
		id, BasisProjectLinked); err != nil {
		return err
	}
	cleared, err := tx.Exec(ctx,
		`UPDATE activity SET retention_class = NULL, retention_class_at = NULL
		  WHERE id = $1 AND retention_class IS NOT NULL AND archived_at IS NULL`, id)
	if err != nil {
		return err
	}
	// The caller holds the row lock, so a miss means the class was already
	// gone, which the judgement just read as filed; fail rather than audit a
	// withdrawal that did not happen.
	if cleared.RowsAffected() != 1 {
		return fmt.Errorf("the activity carries no retention class to withdraw: %w", apperrors.ErrConflict)
	}
	_, err = tx.Exec(ctx, `SELECT set_config($1, '', true)`, UndoDeclarationSetting)
	return err
}

// recordProjectFilingUndone writes the audit entry and the event beside the
// change. The entry carries no field images: the class is not a field a
// reversal may put back, and a before-image of it would invite one.
func recordProjectFilingUndone(ctx context.Context, tx pgx.Tx, id ids.ActivityID, facts projectFilingFacts, name, reason string) error {
	projects := make([]string, 0, len(facts.filings))
	projectIDs := make([]ids.UUID, 0, len(facts.filings))
	for _, filing := range facts.filings {
		projects = append(projects, filing.name)
		if filing.projectID != nil {
			projectIDs = append(projectIDs, *filing.projectID)
		}
	}
	auditID, err := storekit.AuditEventWithEvidence(ctx, tx, "update", "activity", id.UUID, nil, map[string]any{
		"cause": causeProjectFilingUndone, "decided_by_name": name, "reason": reason,
		"projects": projects, "project_ids": projectIDs, "retention_class": retentionClassCorrespondence,
	})
	if err != nil {
		return err
	}
	undone := true
	return storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: crmcontracts.PublicEventActivityChangedFields{ProjectFilingUndone: &undone},
	})
}
