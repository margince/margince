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
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/capture/partslim"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// calendarMethodBackfillPerTick bounds one batch, as the rsvp backfill's does.
const calendarMethodBackfillPerTick = 200

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

// backfillCalendarMethodBatch re-reads up to limit stored originals and records
// the calendar method each declared, returning how many rows it settled. files
// reads a slimmed original's moved parts back; it needs an object store.
func backfillCalendarMethodBatch(ctx context.Context, pool *pgxpool.Pool, files *activities.Store, limit int, log *slog.Logger) (int, error) {
	return drainStoredOriginals(ctx, pool, limit, log, storedOriginalPass[calendarMethodCandidate]{
		name:  "calendar method backfill",
		unit:  "messages",
		offer: selectCalendarMethodCandidates,
		settle: func(ctx context.Context, tx pgx.Tx, c calendarMethodCandidate, payload []byte) (string, error) {
			return backfillOneCalendarMethod(ctx, tx, files, c, payload)
		},
		mark:         activities.MarkCalendarMethodCheckedTx,
		settleUnread: settleCalendarMethodUnread,
	})
}

func selectCalendarMethodCandidates(ctx context.Context, tx pgx.Tx, limit int) ([]calendarMethodCandidate, error) {
	found, err := activities.CalendarMethodCandidatesTx(ctx, tx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]calendarMethodCandidate, len(found))
	for i, c := range found {
		out[i] = calendarMethodCandidate{c}
	}
	return out, nil
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
