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
	"log/slog"
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

// requireArchiveReviewer admits only a signed-in user; each caller then asks
// the retention-policy grant. The archive restricts access, so an agent does
// not read it.
func requireArchiveReviewer(ctx context.Context) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return fmt.Errorf("the archive review is for a signed-in user: %w", apperrors.ErrPermissionDenied)
	}
	return nil
}

// archiveContactNameSQL is the name of the contact aliased c. It is
// empty where the reader holds no contact read or the row scope does not reach it.
func archiveContactNameSQL(ctx context.Context, arg func(any) int) (string, error) {
	if !auth.ReadGranted(ctx, "contact") {
		return "''", nil
	}
	scope, err := auth.ScopeClauseFor(ctx, "contact", "c", arg)
	if err != nil || scope == "" {
		return "coalesce(c.full_name, '')", err
	}
	return "CASE WHEN " + scope + " THEN coalesce(c.full_name, '') ELSE '' END", nil
}

// archiveCompanyNameSQL is the name of the company aliased o. It is
// empty where the reader holds no company read or the row scope does not reach it.
func archiveCompanyNameSQL(ctx context.Context, arg func(any) int) (string, error) {
	if !auth.ReadGranted(ctx, "company") {
		return "''", nil
	}
	scope, err := auth.ScopeClauseFor(ctx, "company", "o", arg)
	if err != nil || scope == "" {
		return "o.display_name", err
	}
	return "CASE WHEN " + scope + " THEN o.display_name ELSE '' END", nil
}

// ListArchivedActivities reads the archive, oldest first, with what keeps
// each record. A subject shows only where the reader's audience admits the
// record's content, and a linked party's name only where the reader may read it.
func (e *Eraser) ListArchivedActivities(ctx context.Context, f ArchiveFilter) (ArchivePage, error) {
	if err := requireArchiveReviewer(ctx); err != nil {
		return ArchivePage{}, err
	}
	if err := auth.Require(ctx, retentionPolicyObject, principal.ActionRead); err != nil {
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
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		kept := activityKeptReasonSQL(arg(floorCurrent), arg(floorCarried))
		readable, err := auth.ActivityContentClause(ctx, "a", arg)
		if err != nil {
			return err
		}
		reach, err := archiveReachSQL(ctx, arg)
		if err != nil {
			return err
		}
		contactName, err := archiveContactNameSQL(ctx, arg)
		if err != nil {
			return err
		}
		companyName, err := archiveCompanyNameSQL(ctx, arg)
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
			       coalesce((SELECT jsonb_agg(jsonb_build_object('id', c.id, 'name', `+contactName+`) ORDER BY `+contactName+`, c.id)
			                   FROM contact c WHERE c.id IN (SELECT l.contact_id FROM activity_link l WHERE l.activity_id = a.id)), '[]'::jsonb),
			       coalesce((SELECT jsonb_agg(jsonb_build_object('id', o.id, 'name', `+companyName+`) ORDER BY `+companyName+`, o.id)
			                   FROM company o WHERE o.id IN (SELECT l.company_id FROM activity_link l WHERE l.activity_id = a.id)), '[]'::jsonb),
			       `+kept+`
			  FROM activity a
			 WHERE `+archivedActivitySQL+reach+where+`
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

// archiveReachSQL leaves out the archived activity the reader may not
// discover, so its id, kind, dates and links are not listed. It applies to an
// all-scope reader too. A colleague's unpromoted captured contact is private
// even to an admin, and so is mail linked only to it.
func archiveReachSQL(ctx context.Context, arg func(any) int) (string, error) {
	discover, err := auth.ActivityDiscoverClause(ctx, "a", arg)
	if err != nil {
		return "", err
	}
	return " AND " + discover, nil
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

// ArchiveDeletion is what one request deleted, what it kept and why, and
// which records failed and are left as they were.
type ArchiveDeletion struct {
	Deleted []ids.UUID
	Kept    []KeptActivity
	Failed  []ids.UUID
}

// KeptActivity is one record a deletion left as it was.
type KeptActivity struct {
	ActivityID ids.UUID
	Reason     string
}

// DeleteArchivedActivities erases the content of each named archived record
// that nothing keeps, each in its own audited transaction. A record that a
// legal hold, an opt-out receipt or the statutory floor keeps is reported
// with the reason and left as it was. A record whose transaction fails is
// reported as failed, and the rest still run, so the answer names what each
// committed transaction did.
func (e *Eraser) DeleteArchivedActivities(ctx context.Context, activityIDs []ids.UUID) (ArchiveDeletion, error) {
	if err := requireArchiveReviewer(ctx); err != nil {
		return ArchiveDeletion{}, err
	}
	if err := auth.Require(ctx, retentionPolicyObject, principal.ActionDelete); err != nil {
		return ArchiveDeletion{}, err
	}
	if len(activityIDs) == 0 || len(activityIDs) > ArchiveDeletionLimit {
		return ArchiveDeletion{}, httperr.Validation("activity_ids", "size",
			fmt.Sprintf("name between 1 and %d archived activities to delete", ArchiveDeletionLimit))
	}
	return deleteEach(ctx, activityIDs, e.deleteArchivedActivity), nil
}

// deleteEach runs deleteOne once for each distinct id and sorts the answers.
// A failed id is logged and reported, and the ids after it still run.
func deleteEach(ctx context.Context, activityIDs []ids.UUID,
	deleteOne func(context.Context, ids.UUID) (string, error),
) ArchiveDeletion {
	out := ArchiveDeletion{Deleted: []ids.UUID{}, Kept: []KeptActivity{}, Failed: []ids.UUID{}}
	seen := map[ids.UUID]bool{}
	for _, id := range activityIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		kept, err := deleteOne(ctx, id)
		switch {
		case err != nil:
			slog.ErrorContext(ctx, "privacy: an archived activity could not be deleted; it is left as it was",
				"activity_id", id, "error", err)
			out.Failed = append(out.Failed, id)
		case kept != "":
			out.Kept = append(out.Kept, KeptActivity{ActivityID: id, Reason: kept})
		default:
			out.Deleted = append(out.Deleted, id)
		}
	}
	return out
}

// deleteArchivedActivity erases one record's content under its row lock, or
// answers what keeps it. A record the reader may not discover reads as not
// found. The linked records are share-locked before the keepers are read. A
// hold placed meanwhile either commits first and keeps the record, or waits
// until the deletion commits.
func (e *Eraser) deleteArchivedActivity(ctx context.Context, id ids.UUID) (string, error) {
	var kept string
	err := e.db.Tx(ctx, func(tx pgx.Tx) error {
		hidden, err := archiveTarget(ctx, tx, id)
		if err != nil || hidden != "" {
			kept = hidden
			return err
		}
		floor, err := ResolveFloorPolicy(ctx, tx, e.country)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, retentionscope.LockHeldLinks("$1"), id); err != nil {
			return err
		}
		floorCurrent, floorCarried := floor.sqlArgs()
		args := []any{}
		arg := func(v any) int { args = append(args, v); return len(args) }
		target := arg(id)
		reason := activityKeptReasonSQL(arg(floorCurrent), arg(floorCarried))
		var archived bool
		err = tx.QueryRow(ctx, `
			SELECT `+reason+`, a.archived_at IS NOT NULL AND NOT `+storekit.ActivityErasedSQL("a")+`
			  FROM activity a WHERE a.id = $`+fmt.Sprint(target)+` FOR UPDATE`, args...).Scan(&kept, &archived)
		if errors.Is(err, pgx.ErrNoRows) {
			kept = keptNotFound
			return nil
		}
		if err != nil || kept != "" {
			return err
		}
		if !archived {
			kept = keptNotArchived
			return nil
		}
		if err := e.eraseActivityContent(ctx, tx, id, aControllerReviewedArchive); err != nil {
			return err
		}
		return auditArchiveDeletion(ctx, tx, id)
	})
	return kept, err
}

// archiveTarget answers keptNotFound for an activity the reader may not
// discover, the gate archiveReachSQL lists by. The gate hides a restricted
// record from everyone, so a restricted one answers kept by law instead, as
// the controller's pin answers it.
func archiveTarget(ctx context.Context, tx pgx.Tx, id ids.UUID) (string, error) {
	err := auth.EnsureActivityVisible(ctx, tx, id)
	if !errors.Is(err, apperrors.ErrNotFound) {
		return "", err
	}
	var restricted bool
	err = tx.QueryRow(ctx, `SELECT restricted_at IS NOT NULL FROM activity WHERE id = $1`, id).Scan(&restricted)
	switch {
	case errors.Is(err, pgx.ErrNoRows) || (err == nil && !restricted):
		return keptNotFound, nil
	case err != nil:
		return "", err
	}
	return keptByLaw, nil
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
