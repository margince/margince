// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// The whole relationship, as metadata, so the plan can pick which parts of it
// matter today.
//
// This is pass ONE of two. It reads dates, subjects and thread keys for up to
// two hundred conversations across a year — enough to see the shape of a
// relationship, cheap enough to run on every open. Pass two (excerpts.go) goes
// back for the bodies of the few threads this pass says are worth reading.
//
// The split is the point. The brief's older reader stopped at the ten newest
// activities, which on an account with a three-month argument in it showed the
// last week of small talk and none of the argument. Lifting that cap alone
// would have bought a bigger prompt and no more understanding: what makes a
// history useful is knowing which parts of it to read closely.
//
// A row this caller may not READ still counts and still keeps its date and
// thread key on the row itself, but contributes neither to the arc it is
// folded into (threads.go) nor a subject or body here. The count of such rows
// becomes an omission, because a thin arc that does not say it is thin reads
// exactly like a quiet account.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// historyCap bounds the scan. Two hundred conversations is more than a year of
// an active account, and the clustering below reduces whatever arrives to at
// most five moments — so a larger number would cost a slower query to reach the
// same five.
const historyCap = 200

// historyMonths is how far back the arc can see. A year holds the whole of a
// normal buying cycle; older than that is company history rather than
// preparation, and `company_context` is where that belongs.
const historyMonths = 12

// HistoryIn is one conversation with anyone in the room, before this meeting.
type HistoryIn struct {
	ID        string
	Kind      string
	Subject   string
	Direction string
	At        time.Time
	// DateOnly says At is a day entry's stored noon.
	DateOnly  bool
	ThreadKey string
	// OnDeal marks a conversation filed under the deal this brief is about.
	OnDeal bool
	// OtherDeal is the deal a conversation is filed under when that deal is
	// not this brief's, and only when the caller may see that deal. Both it and
	// OnDeal are settled by fileUnderDeal once the brief's deal is chosen.
	OtherDeal *FiledDeal
	// Filed is every deal the caller may see that the conversation is filed
	// under, an open one first.
	Filed []FiledDeal
	// Withheld says this caller is outside the activity's audience: the row
	// happened, and what it says is not theirs to read. Subject and body are
	// empty for such a row — not because it was empty, which is the confusion
	// this field exists to prevent.
	Withheld bool
}

// FiledDeal names another deal a conversation is filed under, and whether it
// is open, won or lost. The plan model needs both to keep a lost deal's thread
// out of this meeting's objective.
type FiledDeal struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

const historyQuery = `
	SELECT a.id, a.kind, COALESCE(a.subject, ''), COALESCE(a.direction, ''),
	       a.occurred_at, a.occurred_precision = 'day', COALESCE(a.thread_key, ''),
	       ($%[3]d::uuid IS NOT NULL AND EXISTS (
	          SELECT 1 FROM activity_link dl
	          WHERE dl.activity_id = a.id AND dl.deal_id = $%[3]d)) AS on_deal,
	       (%[2]s) AS readable,
	       -- The deals the row is filed under, under the caller's deal scope.
	       -- An open one first, so a thread on a live and a lost deal is named live.
	       COALESCE((
	         SELECT json_agg(json_build_object('id', fd.id, 'name', fd.name, 'state', fd.status)
	                         ORDER BY (fd.status = 'open') DESC, fd.id)
	         FROM activity_link ol
	         JOIN deal fd ON fd.id = ol.deal_id AND fd.archived_at IS NULL
	         WHERE ol.activity_id = a.id AND ol.deal_id IS NOT NULL AND %[10]s
	       ), '[]'::json) AS filed
	FROM activity a
	WHERE a.archived_at IS NULL
	  AND a.kind <> 'task'
	  AND a.id <> $%[4]d
	  AND %[5]s
	  AND %[9]s
	  AND (a.kind <> 'meeting' OR a.meeting_status IS NULL OR a.meeting_status = 'held')
	  AND (EXISTS (
	         SELECT 1 FROM activity_participant ap
	         WHERE ap.activity_id = a.id AND ap.contact_id = ANY($%[6]d))
	       OR EXISTS (
	         SELECT 1 FROM activity_link pl
	         WHERE pl.activity_id = a.id AND pl.entity_type = 'contact'
	           AND pl.contact_id = ANY($%[6]d)))
	  AND %[1]s
	  AND %[7]s
	-- The id breaks a tie on the timestamp. Two conversations captured in the
	-- same second are common (one sync writes a thread), and without this the
	-- rows that survive the cap, the messages picked for excerpting and the
	-- subject a thread is named after would all vary between reads of an
	-- unchanged database.
	ORDER BY a.occurred_at DESC, a.id DESC
	LIMIT %[8]d`

// readHistory returns the room's conversations, newest first.
//
// Gated the way the contact timeline is (contact360's readActivities): DISCOVER
// decides whether the row is visible at all, and the audience arm decides
// whether its content comes back. Reading only what passes the content clause
// would drop a restricted conversation out of the arc entirely, and an arc
// built from a partial history that does not say it is partial is the failure
// the omission below exists to prevent.
func (s *Service) readHistory(
	ctx context.Context, tx pgx.Tx, room meeting, project *ids.ProjectID, now time.Time,
) ([]HistoryIn, error) {
	if len(room.Room) == 0 {
		return nil, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }

	scope, err := auth.ActivityDiscoverClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = scopeAll
	}
	readable, err := auth.ActivityAudienceArm(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	if readable == "" {
		readable = scopeAll
	}
	var dealArg any
	if room.Deal != nil {
		dealArg = room.Deal.ID
	}
	dealPos := arg(dealArg)
	roomPos := arg(room.ID)
	// Before the room, and happened by now: a brief read weeks early must not fold
	// in what comes before the room. A meeting counts once it took place, as in priormeetings.go.
	opens := fmt.Sprintf("$%d", arg(room.Opens))
	happened := occurredday.HappenedSQL("a", fmt.Sprintf("$%d", arg(now)))
	reach := now
	if room.Opens.Before(reach) {
		reach = room.Opens
	}
	floor := fmt.Sprintf("$%d", arg(reach.AddDate(0, -historyMonths, 0)))
	attendeePos := arg(room.Room)
	within := projectWithinPredicate("a", "hw", project, arg)
	otherDeal, err := scopeFor(ctx, "deal", "fd", arg)
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, fmt.Sprintf(historyQuery,
		scope, readable, dealPos, roomPos, occurredday.WithinSQL("a", floor, opens), attendeePos, within, historyCap, happened,
		otherDeal,
	), args...)
	if err != nil {
		return nil, fmt.Errorf("read the account history: %w", err)
	}
	defer rows.Close()

	return scanHistory(rows)
}

func scanHistory(rows pgx.Rows) ([]HistoryIn, error) {
	var history []HistoryIn
	for rows.Next() {
		var row HistoryIn
		var id ids.UUID
		var readableRow bool
		var filed []byte
		if err := rows.Scan(&id, &row.Kind, &row.Subject, &row.Direction,
			&row.At, &row.DateOnly, &row.ThreadKey, &row.OnDeal, &readableRow, &filed); err != nil {
			return nil, fmt.Errorf("read the account history: %w", err)
		}
		if err := json.Unmarshal(filed, &row.Filed); err != nil {
			return nil, fmt.Errorf("read the deals a conversation is filed under: %w", err)
		}
		row.ID = id.String()
		row.Withheld = !readableRow
		if row.Withheld {
			// What it was called is content. The row still counts; it says
			// nothing and, per threads.go, does not shape the arc either.
			row.Subject = ""
			row.Filed = nil
		}
		history = append(history, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the account history: %w", err)
	}
	return history, nil
}

// fileUnderDeal settles which history rows are on the brief's deal and which
// other deal the rest name. It runs once the deal is chosen: a meeting filed
// under no deal takes the contact's leading open deal, after history is read.
func fileUnderDeal(in *Input) {
	deal := ""
	if in.Deal != nil {
		deal = in.Deal.ID
	}
	for i := range in.History {
		row := &in.History[i]
		row.OnDeal = row.OnDeal || slices.ContainsFunc(row.Filed, func(f FiledDeal) bool { return f.ID == deal })
		row.OtherDeal = otherDealOf(*row, deal)
	}
}

// otherDealOf is the other deal a row may name. A row on this deal is about
// this deal, whatever else it is filed under. A withheld row names nothing,
// as it carries no subject.
func otherDealOf(row HistoryIn, deal string) *FiledDeal {
	if row.Withheld || row.OnDeal {
		return nil
	}
	for _, filed := range row.Filed {
		if filed.ID != deal {
			return &filed
		}
	}
	return nil
}

// withheldCount is how many conversations this caller may know happened but
// not read. It is what the `activity_history` omission reports.
func withheldCount(history []HistoryIn) int {
	count := 0
	for _, row := range history {
		if row.Withheld {
			count++
		}
	}
	return count
}
