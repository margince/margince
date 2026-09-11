// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Reopening a disqualified lead — the reverse of DisqualifyLead, which is the
// one path enforcing "disqualified ⇒ archived".
//
// The lead page could say "Disqualified: <reason>" and could not offer the way
// back, because there was none: demote reverses a PROMOTION and answers about a
// lead that became a contact. A judgement that somebody is not worth pursuing is
// exactly the kind that changes, and a record with no way back makes the
// operator re-key the lead — which loses its history and its score along with
// its reason.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// NotDisqualifiedError maps to 409: the lead is on the open ladder already, or
// was never taken off it, so there is nothing to reopen. Succeeding silently
// would tell a caller their undo did something.
type NotDisqualifiedError struct{}

func (e *NotDisqualifiedError) Error() string {
	return "lead is not disqualified; there is nothing to reopen"
}

// reopenFallbackStatus is where a lead lands when its disqualify audit row no
// longer names the status it held.
//
// `engaged`, because that is what a lead somebody is choosing to pursue again
// IS. `new` would claim nobody had ever touched it, which is false of every
// lead that reached a disqualification. Refusing was the other option and it is
// worse: a lead that cannot be reopened because of a record it does not own is
// a dead row, and the audit trail is retained on its own schedule.
const reopenFallbackStatus = string(crmcontracts.LeadStatusEngaged)

// ReopenLead puts a disqualified lead back on the open ladder at the status it
// held when it was closed.
//
// It takes both grants the disqualify took — update AND delete — because it is
// that act's reverse, and a caller who may not close a lead has no business
// deciding a closure was wrong.
//
// The PROLOGUE it shares with DisqualifyLead is leadWrite: the object gates,
// the catalog read above the transaction, and the writability probe inside it.
// Everything after it diverges and deliberately stays here — the liveness the
// lock and the reads take (this verb acts only on archived rows, its sibling
// only on live ones), the guard, the columns, the audit action and the event. A
// helper covering those would take them as parameters and be a switch between
// two verbs wearing one name, which is harder to read than the two and hides
// that they are opposites.
func (s *Store) ReopenLead(ctx context.Context, id ids.LeadID) (crmcontracts.Lead, error) {
	if err := auth.Require(ctx, "lead", principal.ActionUpdate); err != nil {
		return crmcontracts.Lead{}, err
	}
	if err := auth.Require(ctx, "lead", principal.ActionDelete); err != nil {
		return crmcontracts.Lead{}, err
	}
	active, err := s.activeColumns(ctx, "lead")
	if err != nil {
		return crmcontracts.Lead{}, err
	}
	var out crmcontracts.Lead
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureWritable(ctx, tx, "lead", id.UUID); err != nil {
			return err
		}
		// IncludeArchived on both the lock and the read: a disqualified lead IS
		// archived, so the live-only forms this verb's siblings take would
		// answer not-found for every row it exists to act on.
		if _, err := storekit.LockRow(ctx, tx, "lead", id.UUID, storekit.IncludeArchived); err != nil {
			return err
		}
		current, err := readLead(ctx, tx, id, storekit.IncludeArchived, active)
		if err != nil {
			return err
		}
		// Re-read under the lock, so a reopen racing a second reopen refuses
		// rather than both succeeding and the later one restoring a status the
		// earlier one had already moved on from.
		if current.Status != crmcontracts.LeadStatusDisqualified {
			return &NotDisqualifiedError{}
		}
		restored, err := statusBeforeDisqualification(ctx, tx, id)
		if err != nil {
			return err
		}
		setBy, err := statusSetByFor(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE lead
			   SET status = $2, status_set_by = $3, archived_at = NULL,
			       disqualify_reason_id = NULL, disqualify_note = NULL
			 WHERE id = $1`, id, restored, setBy); err != nil {
			return fmt.Errorf("reopen the lead: %w", err)
		}
		auditID, err := storekit.Audit(ctx, tx, "restore", "lead", id.UUID,
			map[string]any{leadStatusColumn: string(crmcontracts.LeadStatusDisqualified)},
			map[string]any{leadStatusColumn: restored})
		if err != nil {
			return err
		}
		// lead.updated rather than a verb of its own: the closed catalog carries
		// no lead.reopened, and what a subscriber acts on is that the lead is
		// back on the ladder at a status — which the changed fields say.
		//
		// The reason and the note are named as cleared rather than left out. A
		// subscriber holding the closure has to learn that it is gone, and an
		// absent key reads as "unchanged" in this payload's own shape.
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventLeadUpdated{
			ChangedFields: map[string]any{
				leadStatusColumn:       restored,
				"disqualify_reason_id": nil,
				"disqualify_note":      nil,
			},
		}); err != nil {
			return err
		}
		out, err = readLead(ctx, tx, id, storekit.LiveOnly, active)
		return err
	})
	return out, err
}

// statusBeforeDisqualification reads the status this lead held when it was
// closed, from the disqualify's own audit row.
//
// READ, never re-derived. The status a lead had reached is a fact the trail
// already holds, and recomputing it from today's activity would answer about
// the lead as it is now rather than as it was when somebody closed it — a lead
// disqualified at `contacted` and since emailed twice would come back
// `engaged`, which is a claim nobody made.
//
// A row whose trail no longer names one falls back rather than refusing: see
// reopenFallbackStatus. A status the ladder no longer has falls back the same
// way, because restoring a word the enum dropped would put the lead in a state
// no filter can name.
func statusBeforeDisqualification(ctx context.Context, tx pgx.Tx, id ids.LeadID) (string, error) {
	var recorded *string
	err := tx.QueryRow(ctx, `
		SELECT before->>$2 FROM audit_log
		 WHERE entity_type = 'lead' AND entity_id = $1 AND action = 'archive'
		 ORDER BY occurred_at DESC LIMIT 1`, id, leadStatusColumn).Scan(&recorded)
	if errors.Is(err, pgx.ErrNoRows) {
		return reopenFallbackStatus, nil
	}
	if err != nil {
		return "", fmt.Errorf("read the status this lead was disqualified from: %w", err)
	}
	if recorded == nil || !openLadderStatus(*recorded) {
		return reopenFallbackStatus, nil
	}
	return *recorded, nil
}

// openLadderStatus answers whether a status is one a reopened lead may hold.
//
// `disqualified` and `promoted` are excluded deliberately, and not merely
// because they are terminal: restoring either would undo the archive while
// leaving the lead reading as closed, which is a row the page cannot explain
// and no filter counts correctly.
func openLadderStatus(status string) bool {
	switch crmcontracts.LeadStatus(status) {
	case crmcontracts.LeadStatusNew, crmcontracts.LeadStatusContacted, crmcontracts.LeadStatusEngaged:
		return true
	}
	return false
}
