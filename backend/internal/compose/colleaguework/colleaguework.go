// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package colleaguework reads the work a seat does for its colleagues. The
// reporting figures credit two kinds: introductions it made, and meetings it
// booked for a colleague to host. Each reading selects one row per piece of
// work, in the column order of the compose package's outcome facts.
package colleaguework

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// Withheld is the provenance of a row the reader may not open. The row still
// tells the caller that something was left out, and the caller drops it.
const Withheld = "withheld"

// Reading is one figure's SQL fragments, each a placeholder or a predicate
// the caller bound.
type Reading struct {
	// Credited narrows the credited seat, Figure.Seat, to the scope read.
	Credited string
	// Fields holds on a row whose fields the reader may see; TRUE when the
	// figure reads none a mask can hide.
	Fields string
	// From and Until bound when the work was done, Until excluded.
	From, Until string
	Limit       string
	// Label names a row that has no name of its own.
	Label string
}

// Figure is one count of colleague work: the column of the seat it credits,
// and the statement that reads it for the reader in ctx.
type Figure struct {
	Seat string
	sql  func(context.Context, Reading, func(any) int) (string, error)
}

// SQL renders the figure's statement for one reading, binding the reader's
// row gate through arg.
func (f Figure) SQL(ctx context.Context, r Reading, arg func(any) int) (string, error) {
	return f.sql(ctx, r, arg)
}

var (
	// IntroductionsMade credits the colleague who introduced.
	IntroductionsMade = Figure{Seat: "i.introducer_user_id", sql: introductionsMadeSQL}
	// MeetingsBookedForColleagues credits the seat that booked a meeting
	// another colleague hosts.
	MeetingsBookedForColleagues = Figure{Seat: "booked.seat", sql: meetingsBookedForColleaguesSQL}
)

// introductionsMadeSQL counts at the handshake. A name-drop is permission to
// mention someone and never counts. An introduction recorded afterwards holds
// only its date, so it counts from the start of that date in the
// installation's zone.
func introductionsMadeSQL(ctx context.Context, r Reading, arg func(any) int) (string, error) {
	readable, err := introductions.ReadableSQL(ctx, "i", arg)
	if err != nil {
		return "", err
	}
	readable = "(" + readable + " AND " + r.Fields + ")"
	precision := `CASE WHEN i.origin='recorded' THEN 'day' END`
	placed := occurredday.StartAtSQL("i.introduced_at", precision)
	return `SELECT i.id::text,i.id,CASE WHEN ` + readable + ` THEN ` + r.Label + ` ELSE '' END,i.introducer_user_id,COALESCE(u.display_name,''),
 i.introduced_at,` + precision + `,` + placed + `,CASE WHEN ` + readable + ` THEN 'introduced' ELSE '` + Withheld + `' END
 FROM intro_request i
 LEFT JOIN app_user u ON u.id=i.introducer_user_id
 WHERE i.archived_at IS NULL AND ` + placed + `>=` + r.From + ` AND ` + placed + `<` + r.Until + ` AND ` + r.Credited + `
 ORDER BY i.id LIMIT ` + r.Limit, nil
}

// meetingsBookedForColleaguesSQL credits the seat whose session, or whose
// agent, created a meeting another colleague hosts, at the moment it was
// created. A calendar sync or a public booking page acts for no seat. A meeting
// first recorded as held, missed or cancelled was logged after the fact rather
// than booked. A booking cancelled later still counts.
func meetingsBookedForColleaguesSQL(ctx context.Context, r Reading, arg func(any) int) (string, error) {
	readable, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return "", err
	}
	readable = "(" + readable + " AND " + r.Fields + ")"
	return `SELECT a.id::text,a.id,CASE WHEN ` + readable + ` THEN COALESCE(NULLIF(a.subject,''),` + r.Label + `) ELSE '' END,booked.seat,COALESCE(u.display_name,''),
 a.created_at,NULL::text,NULL::timestamptz,CASE WHEN ` + readable + ` THEN 'booked_for_colleague' ELSE '` + Withheld + `' END
 FROM activity a
 JOIN LATERAL (SELECT made.actor_type,made.passport_id,
   COALESCE(made.on_behalf_of,CASE WHEN made.actor_type='human' THEN NULLIF(split_part(made.actor_id,':',2),'')::uuid END) AS seat
   FROM audit_log made WHERE made.entity_type='activity' AND made.entity_id=a.id AND made.action='create'
   ORDER BY made.occurred_at,made.id LIMIT 1) booked ON ` + storekit.AuditForAUser("booked") + `
 LEFT JOIN app_user u ON u.id=booked.seat
 WHERE a.kind='meeting' AND a.archived_at IS NULL AND ` + storekit.KnownMeetingHostSQL("a") + `<>booked.seat
 AND COALESCE((SELECT h.status FROM activity_meeting_history h WHERE h.activity_id=a.id ORDER BY h.effective_at,h.id LIMIT 1),'booked')='booked'
 AND a.created_at>=` + r.From + ` AND a.created_at<` + r.Until + ` AND ` + r.Credited + `
 ORDER BY a.id LIMIT ` + r.Limit, nil
}
