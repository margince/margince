// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// ImportedMeetingCopiesDataTask folds a meeting an importer stored beside the
// calendar meeting of the seat it names as the author. Such imports hold no
// meeting identity: the author's calendar held it first, and the import door
// did not bind across seats. The calendar meeting gains the copy's record
// links. The copy's own text stays on the archived copy, so unarchiving it
// undoes the fold.
func ImportedMeetingCopiesDataTask(store *Store) datatask.Task {
	return datatask.Task{
		ID: "core/1795580400_imported_meeting_copies", Version: 1,
		Title: systemtext.Literal("datatask.title.core.1795580400_imported_meeting_copies"),
		Steps: []datatask.Step{{
			Name: "fold_imported_copies", Limit: 100, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateImportedMeetingCopies, Batch: store.foldImportedMeetingCopiesBatch,
		}},
	}
}

// importedMeetingCopiesSQL pairs a live import that holds no meeting identity
// with a live calendar meeting of its named author. Both share the instant and
// the title. The import keeps no calendar identity, so the author's own seat,
// the instant and the title stand in for it. Only a calendar meeting the whole
// workspace already reads is a fold target, so the links it gains widen no
// audience. A slot with two such calendar meetings is left alone, and a booked
// copy is never folded.
var importedMeetingCopiesSQL = `
	FROM activity imp
	JOIN LATERAL (
	       SELECT min(cal.id::text)::uuid AS id, count(*) AS candidates
	         FROM activity cal
	        WHERE cal.kind = '` + KindMeeting + `' AND cal.archived_at IS NULL AND cal.restricted_at IS NULL
	          AND cal.id <> imp.id AND ` + auth.WorkspaceAudienceSQL("cal") + `
	          AND cal.occurred_at = imp.occurred_at AND cal.subject = imp.subject
	          AND ` + auth.ConnectorSeatSQL("cal") + ` = imp.source_author_id::text
	          AND EXISTS (SELECT 1 FROM activity_identity held
	                       WHERE held.activity_id = cal.id AND held.identity_kind = '` + IdentityKindMeeting + `')
	     ) calendar ON calendar.candidates = 1
	WHERE imp.kind = '` + KindMeeting + `' AND imp.source_system LIKE 'mirror:%'
	  AND imp.archived_at IS NULL AND imp.restricted_at IS NULL AND imp.source_author_id IS NOT NULL
	  AND NOT EXISTS (SELECT 1 FROM activity_identity own
	                   WHERE own.activity_id = imp.id AND own.identity_kind = '` + IdentityKindMeeting + `')
	  AND NOT EXISTS (SELECT 1 FROM meeting_invitation booked WHERE booked.activity_id = imp.id)`

func estimateImportedMeetingCopies(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 `+importedMeetingCopiesSQL+` LIMIT $1) capped`,
		maximum+1).Scan(&count); err != nil {
		return 0, false, fmt.Errorf("activities: estimating imported copies of calendar meetings: %w", err)
	}
	return min(count, maximum), count > maximum, nil
}

func (s *Store) foldImportedMeetingCopiesBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT imp.id, calendar.id `+importedMeetingCopiesSQL+`
		  AND imp.id > $1 ORDER BY imp.id LIMIT $2`, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("activities: listing imported copies of calendar meetings: %w", err)
	}
	type pair struct{ imported, calendar ids.UUID }
	page, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (pair, error) {
		var p pair
		return p, row.Scan(&p.imported, &p.calendar)
	})
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("activities: reading imported copies of calendar meetings: %w", err)
	}
	scanned := make([]ids.UUID, 0, len(page))
	folded := 0
	for _, p := range page {
		scanned = append(scanned, p.imported)
		done, err := s.foldImportedMeetingCopyTx(ctx, tx, ids.From[ids.ActivityKind](p.imported), ids.From[ids.ActivityKind](p.calendar))
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if done {
			folded++
		}
	}
	return datatask.AdvanceUUID(in, scanned, folded)
}

// foldImportedMeetingCopyTx files the calendar meeting under every record the
// imported copy was filed under, then retires the copy onto it.
func (s *Store) foldImportedMeetingCopyTx(ctx context.Context, tx pgx.Tx, imported, calendar ids.ActivityID) (bool, error) {
	if err := storekit.LockMeetingAttendeeWrites(ctx, tx, imported.UUID, calendar.UUID); err != nil {
		return false, err
	}
	var live int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM activity
		 WHERE id IN ($1, $2) AND archived_at IS NULL AND restricted_at IS NULL ORDER BY id FOR UPDATE) locked`,
		imported, calendar).Scan(&live); err != nil {
		return false, fmt.Errorf("activities: locking an imported meeting copy: %w", err)
	}
	if live != 2 {
		return false, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT l.entity_type, coalesce(l.contact_id, l.company_id, l.deal_id, l.lead_id, l.project_id)
		  FROM activity_link l
		 WHERE l.activity_id = $1
		 ORDER BY l.entity_type = '`+linkEntityProject+`' DESC, l.id`, imported)
	if err != nil {
		return false, fmt.Errorf("activities: reading where an imported meeting copy is filed: %w", err)
	}
	links, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (RelinkActivityInput, error) {
		var link RelinkActivityInput
		return link, row.Scan(&link.EntityType, &link.EntityID)
	})
	if err != nil {
		return false, fmt.Errorf("activities: reading where an imported meeting copy is filed: %w", err)
	}
	for _, link := range links {
		// A record erased since the import is not one to file the meeting under.
		if _, err := RelinkActivityInTx(ctx, tx, calendar, link); err != nil && !errors.Is(err, apperrors.ErrNotFound) {
			return false, err
		}
	}
	if err := s.RetireMeetingCopyTx(ctx, tx, imported, calendar); err != nil {
		return false, err
	}
	return true, RecomputeAudiencesTx(ctx, tx, []ids.ActivityID{calendar})
}
