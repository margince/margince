// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The controller's review of the archive. A retention rule moves captured
// correspondence into the archive and never deletes it for age. The archive
// keeps it for a stated purpose (the privacy page names it), and the
// controller deletes what no longer serves that purpose. Nothing here runs on
// a schedule.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/retentionscope"
)

// ArchiveDeletionLimit bounds one deletion request, so each request is a
// bounded run of short transactions.
const ArchiveDeletionLimit = 100

// The reasons a record stays, in the contract's spelling.
const (
	keptLegalHold    = string(crmcontracts.ArchiveKeptReasonLegalHold)
	keptOptOutRecord = string(crmcontracts.ArchiveKeptReasonOptOutRecord)
	keptByLaw        = string(crmcontracts.ArchiveKeptReasonKeptByLaw)
	keptNotArchived  = string(crmcontracts.ArchiveKeptReasonNotArchived)
	keptNotFound     = string(crmcontracts.ArchiveKeptReasonNotFound)
)

// archivedActivitySQL is the archive: the activity aliased a is archived and
// still holds its content. A statutory restriction has its own review, so a
// restricted record is left out.
var archivedActivitySQL = `a.archived_at IS NOT NULL AND a.restricted_at IS NULL AND NOT ` +
	storekit.ActivityErasedSQL("a")

// archiveKeeperSQL names what keeps the archived activity aliased a, or ”
// when nothing does. These are the checks the nightly retention selector runs
// before it acts on an activity, so the controller cannot delete what the
// clock may not. currentArg and carriedArg are the floor's classes.
func archiveKeeperSQL(currentArg, carriedArg int) string {
	return `CASE WHEN ` + retentionscope.HeldThroughAnyLink("a.id") + ` THEN '` + keptLegalHold + `'
	  WHEN ` + receiptKept("a.id") + ` THEN '` + keptOptOutRecord + `'
	  WHEN ` + handelsbriefShielded(currentArg, carriedArg) + ` THEN '` + keptByLaw + `'
	  ELSE '' END`
}

// ArchiveParty is a contact or company an archived activity is linked to.
type ArchiveParty struct {
	ID   ids.UUID `json:"id"`
	Name string   `json:"name"`
}

// ArchivedActivity is one archived record as the controller reviews it. Kept
// is empty when a deletion would erase it.
type ArchivedActivity struct {
	ActivityID ids.UUID
	Kind       string
	Subject    *string
	OccurredAt time.Time
	Precision  string
	ArchivedAt time.Time
	Contacts   []ArchiveParty
	Companies  []ArchiveParty
	Kept       string
}

// ArchiveFilter narrows the review: activity that occurred before a date, and
// activity linked to one contact or company.
type ArchiveFilter struct {
	Before    *time.Time
	ContactID *ids.UUID
	CompanyID *ids.UUID
	Cursor    *string
	Limit     *int
}

// ArchivePage is one oldest-first keyset page of the archive.
type ArchivePage struct {
	Records    []ArchivedActivity
	NextCursor string
	HasMore    bool
}

// requireArchiveReviewer admits a signed-in user holding the retention-policy
// authority at verb. The archive restricts access, so an agent does not read it.
func requireArchiveReviewer(ctx context.Context, verb principal.Action) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return fmt.Errorf("the archive review is for a signed-in user: %w", apperrors.ErrPermissionDenied)
	}
	return auth.Require(ctx, retentionPolicyObject, verb)
}

// ListArchivedActivities reads the archive, oldest first, with what keeps
// each record. A subject shows only where the reader's audience admits the
// record's content.
func (e *Eraser) ListArchivedActivities(ctx context.Context, f ArchiveFilter) (ArchivePage, error) {
	if err := requireArchiveReviewer(ctx, principal.ActionRead); err != nil {
		return ArchivePage{}, err
	}
	size := storekit.ClampLimit(f.Limit)
	var page ArchivePage
	err := e.db.Tx(ctx, func(tx pgx.Tx) error {
		floor, err := ResolveFloorPolicy(ctx, tx, e.country)
		if err != nil {
			return err
		}
		floorCurrent, floorCarried := floor.sqlArgs()
		args := []any{floorCurrent, floorCarried}
		arg := func(v any) int { args = append(args, v); return len(args) }
		readable, err := auth.ActivityContentClause(ctx, "a", arg)
		if err != nil {
			return err
		}
		where, err := archiveListWhere(f, arg)
		if err != nil {
			return err
		}
		limit := arg(size + 1)
		rows, err := tx.Query(ctx, `
			SELECT a.id, a.kind, CASE WHEN `+readable+` THEN a.subject END, a.occurred_at, a.occurred_precision, a.archived_at,
			       coalesce((SELECT jsonb_agg(jsonb_build_object('id', c.id, 'name', coalesce(c.full_name, '')) ORDER BY c.full_name, c.id)
			                   FROM contact c WHERE c.id IN (SELECT l.contact_id FROM activity_link l WHERE l.activity_id = a.id)), '[]'::jsonb),
			       coalesce((SELECT jsonb_agg(jsonb_build_object('id', o.id, 'name', o.display_name) ORDER BY o.display_name, o.id)
			                   FROM company o WHERE o.id IN (SELECT l.company_id FROM activity_link l WHERE l.activity_id = a.id)), '[]'::jsonb),
			       `+archiveKeeperSQL(1, 2)+`
			  FROM activity a
			 WHERE `+archivedActivitySQL+where+`
			 ORDER BY a.occurred_at ASC, a.id ASC
			 LIMIT $`+fmt.Sprint(limit), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r ArchivedActivity
			if err := rows.Scan(&r.ActivityID, &r.Kind, &r.Subject, &r.OccurredAt, &r.Precision, &r.ArchivedAt,
				&r.Contacts, &r.Companies, &r.Kept); err != nil {
				return err
			}
			page.Records = append(page.Records, r)
		}
		return rows.Err()
	})
	if err != nil {
		return ArchivePage{}, err
	}
	if len(page.Records) > size {
		page.Records = page.Records[:size]
		last := page.Records[len(page.Records)-1]
		next, err := storekit.EncodeCursor(last.OccurredAt, last.ActivityID)
		if err != nil {
			return ArchivePage{}, err
		}
		page.NextCursor, page.HasMore = next, true
	}
	return page, nil
}

// archiveListWhere renders the filter, numbering each argument through arg.
// The cursor's created_at slot carries occurred_at, which the page is ordered
// by.
func archiveListWhere(f ArchiveFilter, arg func(any) int) (string, error) {
	where := ""
	if f.Before != nil {
		where += " AND " + occurredday.BeforeSQL("a", "$"+fmt.Sprint(arg(*f.Before)))
	}
	if f.ContactID != nil {
		where += " AND EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = a.id AND l.contact_id = $" +
			fmt.Sprint(arg(*f.ContactID)) + ")"
	}
	if f.CompanyID != nil {
		where += " AND EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = a.id AND l.company_id = $" +
			fmt.Sprint(arg(*f.CompanyID)) + ")"
	}
	if f.Cursor != nil && *f.Cursor != "" {
		c, err := storekit.DecodeCursor(*f.Cursor)
		if err != nil {
			return "", err
		}
		at, id := arg(c.CreatedAt), arg(c.ID)
		where += fmt.Sprintf(" AND (a.occurred_at, a.id) > ($%d, $%d) -- occurred_at raw: a keyset bound\n", at, id)
	}
	return where, nil
}

// ArchiveDeletion is what one request deleted, and what it kept and why.
type ArchiveDeletion struct {
	Deleted []ids.UUID
	Kept    []KeptActivity
}

// KeptActivity is one record a deletion left as it was.
type KeptActivity struct {
	ActivityID ids.UUID
	Reason     string
}

// DeleteArchivedActivities erases the content of each named archived record
// that nothing keeps, each in its own audited transaction. A record that a
// legal hold, an opt-out receipt or the statutory floor keeps is reported
// with the reason and left as it was.
func (e *Eraser) DeleteArchivedActivities(ctx context.Context, activityIDs []ids.UUID) (ArchiveDeletion, error) {
	if err := requireArchiveReviewer(ctx, principal.ActionDelete); err != nil {
		return ArchiveDeletion{}, err
	}
	if len(activityIDs) == 0 || len(activityIDs) > ArchiveDeletionLimit {
		return ArchiveDeletion{}, httperr.Validation("activity_ids", "size",
			fmt.Sprintf("name between 1 and %d archived activities to delete", ArchiveDeletionLimit))
	}
	out := ArchiveDeletion{Deleted: []ids.UUID{}, Kept: []KeptActivity{}}
	seen := map[ids.UUID]bool{}
	for _, id := range activityIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		kept, err := e.deleteArchivedActivity(ctx, id)
		if err != nil {
			return out, err
		}
		if kept != "" {
			out.Kept = append(out.Kept, KeptActivity{ActivityID: id, Reason: kept})
			continue
		}
		out.Deleted = append(out.Deleted, id)
	}
	return out, nil
}

// deleteArchivedActivity erases one record's content under its row lock, or
// answers what keeps it. The keepers are read under the lock, so a hold placed
// after the list was read still keeps the record.
func (e *Eraser) deleteArchivedActivity(ctx context.Context, id ids.UUID) (string, error) {
	var kept string
	err := e.db.Tx(ctx, func(tx pgx.Tx) error {
		floor, err := ResolveFloorPolicy(ctx, tx, e.country)
		if err != nil {
			return err
		}
		floorCurrent, floorCarried := floor.sqlArgs()
		err = tx.QueryRow(ctx, `
			SELECT CASE WHEN a.restricted_at IS NOT NULL THEN '`+keptByLaw+`'
			            WHEN a.archived_at IS NULL OR `+storekit.ActivityErasedSQL("a")+` THEN '`+keptNotArchived+`'
			            ELSE `+archiveKeeperSQL(2, 3)+` END
			  FROM activity a WHERE a.id = $1 FOR UPDATE`, id, floorCurrent, floorCarried).Scan(&kept)
		if errors.Is(err, pgx.ErrNoRows) {
			kept = keptNotFound
			return nil
		}
		if err != nil || kept != "" {
			return err
		}
		if err := e.eraseActivityContent(ctx, tx, id, aControllerReviewedArchive); err != nil {
			return err
		}
		return auditArchiveDeletion(ctx, tx, id)
	})
	return kept, err
}

// auditArchiveDeletion records the controller's deletion as an erase
// certificate, strips the record's earlier images, and publishes it.
func auditArchiveDeletion(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	auditID, err := storekit.AuditWithEvidence(ctx, tx, actionErase, "activity", id, nil, nil, map[string]any{
		evidenceKeyCause: causeArchiveReview, evidenceKeyRetentionAction: actionErase,
	})
	if err != nil {
		return err
	}
	if err := storekit.RedactAuditHistory(ctx, tx, auditID); err != nil {
		return err
	}
	reason := aControllerReviewedArchive.collateralReason
	return storekit.EmitEventForEntity(ctx, tx, auditID, "activity", id,
		retentionAppliedPayload(crmcontracts.RetentionAppliedErase, nil, &reason))
}
