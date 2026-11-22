// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingrosters

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/storedmail"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/originalstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// BirthRostersDataTask names the calendar roster on guest rows written before
// rows recorded one. Such a row is never retired, so a colleague the organizer
// removed keeps reading the meeting. Only a row the stored event proves is named.
func BirthRostersDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795253139_calendar_guests_name_their_roster", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795253139_calendar_guests_name_their_roster"),
		Requires: []string{"core/1794604200"},
		Steps: []datatask.Step{{
			Name: "name_birth_rosters", Limit: 100, MinLimit: 1,
			Audit: datatask.AuditPolicy{Reason: "capture.stampBirthParties names the same roster on the rows it writes, without a separate audit"},
			Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
				return storedmail.Count(ctx, q, unnamedBirthRosters, maximum)
			},
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				return storedmail.AttendeePage(ctx, tx, in, "a.id, a.source_system, "+originalstore.SQL("rc"), unnamedBirthRosters,
					func(row *calendarMeeting) (ids.UUID, *[]byte) { return row.ID, &row.Payload },
					nameBirthRoster)
			},
		}},
	}
}

// unnamedBirthRosters selects live meetings a calendar made that keep their
// stored event and still hold a guest row from their birth naming no roster.
var unnamedBirthRosters = `FROM activity a JOIN raw_capture rc ON ` + storekit.StoredOriginalSQL("a", "rc") + `
 WHERE a.kind = 'meeting' AND a.source_system IN ('` + GoogleCalendar + `', '` + GraphCalendar + `')
   AND a.archived_at IS NULL AND a.restricted_at IS NULL
   AND split_part(a.captured_by, ':', 1) = 'connector' AND split_part(a.captured_by, ':', 2) = a.source_system
   AND EXISTS (SELECT 1 FROM activity_participant ap
                WHERE ap.activity_id = a.id AND ` + activities.BirthRosterRowSQL("ap", "a") + `)`

type calendarMeeting struct {
	ID       ids.UUID
	Provider string
	Payload  []byte
}

// nameBirthRoster names the stamping seat on the meeting's birth rows, when the
// stored event is that seat's own calendar copy.
func nameBirthRoster(ctx context.Context, tx pgx.Tx, row calendarMeeting) (bool, error) {
	raw, err := capture.DecodeStoredOriginal(row.Payload)
	if err != nil {
		return false, nil //nolint:nilerr // an original no parser reads proves no roster
	}
	calendar, err := storedCalendar(ctx, tx, row.Provider, raw)
	if err != nil || calendar == "" {
		return false, err
	}
	meeting := ids.From[ids.ActivityKind](row.ID)
	seat, stamps, err := StampingSeat(ctx, tx, meeting, row.Provider, calendar)
	if err != nil || !stamps {
		return false, err
	}
	rec, err := recordOf(row.Provider, raw, calendar)
	if err != nil {
		return false, nil //nolint:nilerr // an event live capture would drop lists nobody
	}
	listed := make([]string, 0, len(rec.Participants)+len(rec.WithheldParties))
	for _, party := range slices.Concat(rec.Participants, rec.WithheldParties) {
		listed = append(listed, party.Email)
	}
	named, err := activities.NameBirthRosterTx(ctx, tx, meeting, seat, listed)
	return named > 0, err
}
