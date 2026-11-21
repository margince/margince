// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingrosters

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/storedmail"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/originalstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// GraphGuestsDataTask gives a meeting that a Microsoft calendar event resolved
// onto, an import for instance, the guests the stored event lists. inWorkspace
// binds the installation's workspace, which the seat's authority is read in.
func GraphGuestsDataTask(authority authz.Resolver, inWorkspace func(context.Context) (context.Context, error)) datatask.Task {
	return datatask.Task{
		ID: "core/1795253138_matched_graph_meeting_guests", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795253138_matched_graph_meeting_guests"),
		Requires: []string{"core/1792998482"},
		Steps: []datatask.Step{{
			Name: "add_calendar_guests", Limit: 100, MinLimit: 1,
			Audit: datatask.AuditPolicy{PerRecord: true},
			Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
				return storedmail.Count(ctx, q, matchedGraphOriginals, maximum)
			},
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				ctx, err := inWorkspace(ctx)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				return storedmail.Page(ctx, tx, in, "a.id, "+originalstore.SQL("a"), matchedGraphOriginals,
					func(row *storedEvent) (ids.UUID, *[]byte) { return row.ID, &row.Payload },
					func(ctx context.Context, tx pgx.Tx, row storedEvent) (bool, error) {
						return AddGuestsFromOriginal(ctx, tx, authority, GraphCalendar, row.Payload, in.RunID)
					})
			},
		}},
	}
}

// matchedGraphOriginals selects the stored Microsoft calendar originals whose
// own key holds no meeting, because the event was filed under another door's
// row. The original is aliased a, the row the shared pager walks and locks.
const matchedGraphOriginals = `FROM raw_capture a
 WHERE a.source_system = '` + GraphCalendar + `'
   AND NOT EXISTS (SELECT 1 FROM activity own
                    WHERE own.source_system = a.source_system AND own.source_id = a.source_id
                      AND own.restricted_at IS NULL)`

type storedEvent struct {
	ID      ids.UUID
	Payload []byte
}

// AddGuestsFromOriginal files one stored event's guests on the meeting its
// identity names, when the calendar that stored it is the seat that stamps it.
func AddGuestsFromOriginal(
	ctx context.Context, tx pgx.Tx, authority authz.Resolver, provider string, payload []byte, run ids.UUID,
) (bool, error) {
	raw, err := capture.DecodeStoredOriginal(payload)
	if err != nil {
		return false, nil //nolint:nilerr // an original no parser reads is skipped, as live capture skips it
	}
	calendar, err := storedCalendar(ctx, tx, provider, raw)
	if err != nil || calendar == "" {
		return false, err
	}
	rec, err := recordOf(provider, raw, calendar)
	if err != nil || !rec.CrossDoorIdentity.Stated() {
		return false, nil //nolint:nilerr // a drop live capture would make is a drop here too
	}
	key := activities.MeetingIdentityKey(rec.CrossDoorIdentity.Series, capture.OccurrenceText(rec.CrossDoorIdentity))
	meeting, found, err := activities.ResolveIdentity(ctx, tx, activities.IdentityKindMeeting, key)
	if err != nil || !found {
		return false, err
	}
	seat, stamps, err := StampingSeat(ctx, tx, meeting, provider, calendar)
	if err != nil || !stamps {
		return false, err
	}
	added, err := capture.SettleStoredMeetingRosterTx(ctx, tx, authority, provider, meeting, seat, rec,
		map[string]any{runEvidence: run.String()})
	if err != nil || added.Added == 0 {
		return added.Changed(), err
	}
	return true, contacts.FillParticipantNamesTx(ctx, tx, meeting)
}
