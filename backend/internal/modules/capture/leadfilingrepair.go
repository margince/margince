// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// LeadFiler is the activities module's automatic lead filing, which owns
// activity_link, its audit row and the relink event. Compose injects it.
type LeadFiler interface {
	// FileOnLead writes one lead link under the activity's lock, and answers
	// whether it landed.
	FileOnLead(ctx context.Context, tx pgx.Tx, id ids.ActivityID, lead ids.UUID, evidence map[string]any) (bool, error)
}

// leadFilingReason is the evidence every repair write records.
const leadFilingReason = "captured_activity_filed_under_lead"

// leadFilingCandidatesSQL selects live connector captures that a live lead's
// address is on and that are not yet filed under it. It applies
// participantRecordsSQL's match and address-history rule. Callers append the cursor.
//
// A message the ladder suppressed is filed under nobody. Its no_record hold
// says so, and its ladder trace says so once a later audience replaced the hold.
var leadFilingCandidatesSQL = `
	SELECT a.id FROM activity a
	 WHERE a.captured_by LIKE 'connector:%'
	   AND a.archived_at IS NULL AND a.restricted_at IS NULL
	   AND a.audience_reason IS DISTINCT FROM 'no_record'
	   AND NOT EXISTS (SELECT 1 FROM capture_trace t
	                    WHERE t.source_system = a.source_system AND t.source_id = a.source_id
	                      AND t.stage = 'tier_ladder' AND t.outcome = 'suppressed')
	   AND EXISTS (SELECT 1 FROM activity_participant ap
	                 JOIN lead ld ON ` + storekit.LeadEmailMatchClause("ld", "ap.address") + `
	                WHERE ap.activity_id = a.id AND ap.address IS NOT NULL
	                  AND ` + storekit.LiveLeadClause("ld") + `
	                  AND ` + storekit.LeadEmailHeldAtClause("ld", "ap.address", "a") + `
	                  AND NOT EXISTS (SELECT 1 FROM activity_link l
	                                   WHERE l.activity_id = a.id AND l.lead_id = ld.id))`

// LeadFilingRepair files activities captured before capture knew leads under
// the live leads their participants' addresses name. It runs the capture rule
// (participantfiling.go) as each mailbox that imported the activity. It writes
// as its own actor, with its reason.
type LeadFilingRepair struct {
	Authority authz.Resolver
	Filer     LeadFiler
}

// BatchTx files one page of activities. A second run changes nothing, because
// every link it could write is then there.
func (r LeadFilingRepair) BatchTx(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, leadFilingCandidatesSQL+` AND a.id > $1 ORDER BY a.id LIMIT $2`, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("capture: listing activities for the lead filing: %w", err)
	}
	scanned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("capture: listing activities for the lead filing: %w", err)
	}
	changed := 0
	for _, id := range scanned {
		written, err := r.file(ctx, tx, ids.From[ids.ActivityKind](id))
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if written > 0 {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, scanned, changed)
}

// file runs the rule for one activity as each importing mailbox, under the
// activity's row lock, with what is left of the link ceiling.
func (r LeadFilingRepair) file(ctx context.Context, tx pgx.Tx, activity ids.ActivityID) (int, error) {
	if _, err := storekit.LockRow(ctx, tx, "activity", activity.UUID, storekit.LiveOnly); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("capture: locking an activity to file under a lead: %w", err)
	}
	var capturedBy string
	var filed int
	var onProject bool
	if err := tx.QueryRow(ctx, `SELECT a.captured_by, (SELECT count(*) FROM activity_link l WHERE l.activity_id = a.id),
		EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = a.id AND l.entity_type = 'project')
		FROM activity a WHERE a.id = $1`, activity).Scan(&capturedBy, &filed, &onProject); err != nil {
		return 0, fmt.Errorf("capture: reading an activity to file under a lead: %w", err)
	}
	name, ok := strings.CutPrefix(capturedBy, "connector:")
	if !ok || name == "" {
		return 0, nil
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT user_id FROM capture_import WHERE activity_id = $1 ORDER BY user_id`, activity)
	if err != nil {
		return 0, fmt.Errorf("capture: reading the mailboxes an activity came from: %w", err)
	}
	seats, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return 0, fmt.Errorf("capture: reading the mailboxes an activity came from: %w", err)
	}
	// The project ladder retries on every replay, so an activity not yet on a
	// project keeps the seat a capture keeps for it.
	if !onProject {
		filed += projectLinkReservation
	}
	evidence := map[string]any{"reason": leadFilingReason}
	written := 0
	for _, seat := range seats {
		acting, err := connectorActing(ctx, r.Authority, name, ids.From[ids.UserKind](seat))
		if errors.Is(err, apperrors.ErrNotFound) {
			continue
		}
		if err != nil {
			return 0, err
		}
		evidence["decided_as"] = seat
		private, err := storedMessageIsPrivateTx(ctx, tx, activity, seat)
		if err != nil {
			return 0, err
		}
		n, err := fileUnderParticipants(tx, activity, participantFiling{
			decider: acting,
			budget:  maxDerivedMeetingLinks - filed - written,
			private: private,
			wants:   func(record participantRecord) bool { return record.entity == datasource.EntityLead },
			write: func(record participantRecord) (bool, error) {
				return r.Filer.FileOnLead(ctx, tx, activity, record.id, evidence)
			},
		})
		if err != nil {
			return 0, err
		}
		written += n
	}
	return written, nil
}

// EstimateLeadFiling bounds the work: how many activities the repair will
// inspect, at most maximum.
func EstimateLeadFiling(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (`+leadFilingCandidatesSQL+` LIMIT $1) AS candidates`, maximum+1).Scan(&count)
	return min(count, maximum), count > maximum, err
}
