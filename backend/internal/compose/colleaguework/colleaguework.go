// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package colleaguework reads the work a seat does for its colleagues. The
// reporting figures credit two kinds: introductions it made, and meetings it
// booked for a colleague to host. Each reading selects one row per piece of
// work, in the column order of the compose package's outcome facts.
package colleaguework

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

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
// row gate through arg. A row the reader may not open is never selected.
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

// introductionsMadeSQL counts an ask when it was completed, and a recorded
// introduction when it was recorded. The date a recording names is the
// colleague's word, so it never moves the work into a past period. A
// name-drop is permission to mention someone and never counts.
func introductionsMadeSQL(ctx context.Context, r Reading, arg func(any) int) (string, error) {
	readable, err := introductions.ReadableSQL(ctx, "i", arg)
	if err != nil {
		return "", err
	}
	made := `(CASE WHEN i.origin='recorded' THEN i.created_at ELSE i.introduced_at END)`
	return `SELECT i.id::text,i.id,` + r.Label + `,i.introducer_user_id,COALESCE(u.display_name,''),` + made + `,NULL::text,NULL::timestamptz,'introduced'
 FROM intro_request i
 LEFT JOIN app_user u ON u.id=i.introducer_user_id
 WHERE ` + readable + ` AND ` + r.Fields + `
 AND ` + made + `>=` + r.From + ` AND ` + made + `<` + r.Until + ` AND ` + r.Credited + `
 ORDER BY i.id LIMIT ` + r.Limit, nil
}

// meetingsBookedForColleaguesSQL credits the seat whose session, or agent,
// created the meeting, at its creation, for the host that create named. A
// source system means an import, transcript or connector booked it. A meeting
// created at or after its start, or first recorded as held, missed or
// cancelled, was logged rather than booked. A later cancellation keeps it.
func meetingsBookedForColleaguesSQL(ctx context.Context, r Reading, arg func(any) int) (string, error) {
	readable, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return "", err
	}
	return `SELECT a.id::text,a.id,COALESCE(NULLIF(a.subject,''),` + r.Label + `),booked.seat,COALESCE(u.display_name,''),
 a.created_at,NULL::text,NULL::timestamptz,'booked_for_colleague'
 FROM activity a
 JOIN LATERAL (SELECT made.actor_type,made.passport_id,
   COALESCE(made.on_behalf_of,CASE WHEN made.actor_type='human' THEN NULLIF(split_part(made.actor_id,':',2),'')::uuid END) AS seat,
   NULLIF(made.evidence->>'` + storekit.ExplicitMeetingHostEvidence + `','')::uuid AS host
   FROM audit_log made WHERE made.entity_type='activity' AND made.entity_id=a.id AND made.action='create'
   ORDER BY made.occurred_at,made.id LIMIT 1) booked ON ` + storekit.AuditForAUser("booked") + `
 LEFT JOIN LATERAL (SELECT h.status FROM activity_meeting_history h
   WHERE h.activity_id=a.id ORDER BY h.effective_at,h.id LIMIT 1) opened ON true
 LEFT JOIN app_user u ON u.id=booked.seat
 WHERE a.kind='meeting' AND a.archived_at IS NULL AND a.source_system IS NULL AND booked.host<>booked.seat
 AND COALESCE(opened.status,'booked')='booked' AND ` + occurredday.AheadSQL("a", "a.created_at") + `
 AND ` + readable + ` AND ` + r.Fields + `
 AND a.created_at>=` + r.From + ` AND a.created_at<` + r.Until + ` AND ` + r.Credited + `
 ORDER BY a.id LIMIT ` + r.Limit, nil
}

// NameSystemWritten names a meeting Margince wrote in the report's own
// language, the language every other word of the report is in.
func NameSystemWritten(ctx context.Context, tx pgx.Tx, facts []reporting.Fact, lang textlang.Lang) error {
	activityIDs := make([]ids.UUID, 0, len(facts))
	for _, fact := range facts {
		activityIDs = append(activityIDs, fact.SourceID)
	}
	written, err := storekit.SystemWrittenActivities(ctx, tx, activityIDs)
	if err != nil {
		return err
	}
	for i := range facts {
		if words, ok := written[facts[i].SourceID]; ok && facts[i].Row.Label == words.Subject {
			facts[i].Row.Label = words.Message.SubjectIn(words.Subject, lang)
		}
	}
	return nil
}
