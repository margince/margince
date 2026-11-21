// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Reading the calendar method of mail captured before capture recorded it.
//
// A calendar copy, inbound or outbound, waits on nobody's written reply. Rows
// from before the column read NULL and stay owed or waiting. This pass re-reads
// each stored original through mailmap.Parse, the reader live capture uses.
// No subject-line guess: "Declined:" is worded per provider language.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/capture/partslim"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/originalstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// calendarMethodsDataTask re-reads the mail captured before calendar_method
// existed. The engine records that it finished, so a drained installation
// reads the table no more.
func calendarMethodsDataTask(pool *pgxpool.Pool) datatask.Task {
	return datatask.Task{
		ID: "core/1795240327_legacy_calendar_methods", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795240327_legacy_calendar_methods"),
		Requires: []string{"core/" + activities.CalendarMethodMigration},
		Steps: []datatask.Step{{
			Name: "read_calendar_methods", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: activities.EstimateCalendarMethodCandidates,
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				return readCalendarMethodsBatch(ctx, tx, activities.NewStore(InstallationDB(pool)), in)
			},
		}},
	}
}

// readCalendarMethodsBatch checks each offered mail and marks it, so a replay
// offers it no more. A slimmed original's parts come from the bound object
// store; with none bound such an original reads as unreadable.
func readCalendarMethodsBatch(ctx context.Context, tx pgx.Tx, store *activities.Store, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	found, err := activities.CalendarMethodCandidatesTx(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	if blob, err := originalstore.Objects(ctx); err == nil {
		store = store.WithBlobstore(blob)
	}
	pass := storedOriginalPass[calendarMethodCandidate]{
		settle: func(ctx context.Context, tx pgx.Tx, c calendarMethodCandidate, payload []byte) (string, error) {
			return backfillOneCalendarMethod(ctx, tx, store, c, payload)
		},
		mark:         activities.MarkCalendarMethodCheckedTx,
		settleUnread: settleCalendarMethodUnread,
	}
	scanned := make([]ids.UUID, 0, len(found))
	changed := 0
	for _, c := range found {
		scanned = append(scanned, c.ActivityID.UUID)
		settled, err := settleStoredOriginal(ctx, tx, pass, calendarMethodCandidate{c})
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if settled {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, scanned, changed)
}

// calendarMethodCandidate adapts activities' candidate to the drain.
type calendarMethodCandidate struct {
	activities.CalendarMethodCandidate
}

func (c calendarMethodCandidate) activity() ids.ActivityID { return c.ActivityID }
func (c calendarMethodCandidate) original() ids.UUID {
	if c.RawCaptureID == nil {
		return ids.UUID{}
	}
	return *c.RawCaptureID
}

// settleCalendarMethodUnread settles mail whose original is gone without a read.
func settleCalendarMethodUnread(c calendarMethodCandidate) (string, bool) {
	if c.RawCaptureID == nil {
		return activities.CalendarMethodBackfillUnreadable, true
	}
	return "", false
}

// backfillOneCalendarMethod reads one stored original's calendar method and
// stores it. A parse failure is an outcome rather than an error, so one bad
// original cannot stop the pass reaching the rest.
//
// A slimmed original has its part bytes in the object store, and the method
// may be only there. Its parts are put back first, so the header and the body
// are checked against each other as live capture checks them.
func backfillOneCalendarMethod(ctx context.Context, tx pgx.Tx, files *activities.Store, c calendarMethodCandidate, payload []byte) (string, error) {
	raw, err := decodeStoredOriginal(payload)
	if err != nil {
		return activities.CalendarMethodBackfillUnreadable, nil //nolint:nilerr // unreadable is the recorded outcome, not a fault
	}
	if partslim.IsSlimmed(raw) {
		if raw, err = restoreSlimmedOriginal(ctx, tx, files, c.ActivityID, raw); err != nil {
			return activities.CalendarMethodBackfillUnreadable, nil //nolint:nilerr // unreadable is the recorded outcome, not a fault
		}
	}
	msg, err := mailmap.Parse(raw, "")
	if err != nil {
		return activities.CalendarMethodBackfillUnreadable, nil //nolint:nilerr // unreadable is the recorded outcome, not a fault
	}
	method := msg.CalendarMethod()
	if method == "" {
		return activities.CalendarMethodBackfillNone, nil
	}
	if _, err := activities.SetCalendarMethodTx(ctx, tx, c.ActivityID, method); err != nil {
		return "", err
	}
	return activities.CalendarMethodBackfillSet, nil
}

// restoreSlimmedOriginal puts a slimmed original's moved part bytes back from
// this message's own stored files.
//
// The stanza naming each object sits in the payload, which a sender can write
// into. So only keys of files filed on this message are read, as the private
// thread strip reads them; any other key fails the restore.
func restoreSlimmedOriginal(ctx context.Context, tx pgx.Tx, files *activities.Store, id ids.ActivityID, raw []byte) ([]byte, error) {
	stored, err := files.StoredFilesOfMessageTx(ctx, tx, id.UUID)
	if err != nil {
		return nil, err
	}
	byKey := make(map[string][]byte, len(stored))
	for _, f := range stored {
		byKey[f.Key] = f.Body
	}
	return partslim.RestoreStoredParts(raw, func(ref partslim.PartRef) ([]byte, error) {
		if body, ok := byKey[ref.StorageKey]; ok && body != nil {
			return body, nil
		}
		return nil, fmt.Errorf("compose: the original names a part this message does not store: part:%d", ref.Ordinal)
	})
}
