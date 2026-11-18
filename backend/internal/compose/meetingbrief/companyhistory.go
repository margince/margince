// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// "Earlier at <company>": what colleagues' contacts at the same company already
// received. A new contact's own records say nothing about it, and the reader can
// walk in and contradict a statement the company has had for months.
//
// The history is the same read as the room's own (readHistoryOf), over the other
// current contacts at the room's company, so row scope, the content audience and
// the exchange predicate are not restated here.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relationshipmark"
)

const (
	// companyColleagueCap bounds the contacts whose history is read.
	companyColleagueCap = 50
	// companyHistoryReadCap bounds the conversations read across them.
	companyHistoryReadCap = 100
	// companyContactCap and companyTopicsPerContact bound what the prep shows:
	// the newest topics of the few most recently active contacts.
	companyContactCap       = 4
	companyTopicsPerContact = 2
)

// colleague is another current contact at the room's company that the reader
// may see.
type colleague struct {
	ID   ids.UUID
	Name string
}

// CompanyTopicIn is one earlier conversation a colleague's contact had with the
// company, named by the contact it was with.
type CompanyTopicIn struct {
	ID        string
	Contact   string
	Kind      string
	Subject   string
	At        Occurred
	Direction string `json:"direction,omitempty"`
}

// readColleagues lists the other current contacts at the companies the room's
// contacts work at. The coworker set is an edge, so it takes the same edge and
// contact row scope as the network's coworker read.
func (s *Service) readColleagues(ctx context.Context, tx pgx.Tx, room meeting) ([]colleague, error) {
	if len(room.Room) == 0 {
		return nil, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	roomPos := arg(room.Room)
	contactScope, err := scopeFor(ctx, "contact", "p", arg)
	if err != nil {
		return nil, err
	}
	edgeScope, err := auth.EdgeReadScope(ctx, "colleague", arg)
	if err != nil {
		return nil, err
	}
	if edgeScope == "" {
		edgeScope = scopeAll
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT id, full_name FROM (
		  SELECT p.id, p.full_name,
		         row_number() OVER (PARTITION BY p.id ORDER BY colleague.id) AS edge_rank
		    FROM relationship theirs
		    JOIN relationship colleague
		      ON colleague.company_id = theirs.company_id AND colleague.kind = 'employment'
		     AND `+employment.IsCurrentSQL("colleague.ended_at")+` AND colleague.archived_at IS NULL
		    JOIN contact p ON p.id = colleague.contact_id AND p.archived_at IS NULL
		     AND `+relationshipmark.NotMarkedSQL("p")+`
		   WHERE theirs.contact_id = ANY($%d) AND theirs.kind = 'employment'
		     AND `+employment.IsCurrentSQL("theirs.ended_at")+` AND theirs.archived_at IS NULL
		     AND NOT (p.id = ANY($%d)) AND (%s) AND (%s)) coworkers
		 WHERE edge_rank = 1
		 ORDER BY full_name, id
		 LIMIT %d`, roomPos, roomPos, contactScope, edgeScope, companyColleagueCap), args...)
	if err != nil {
		return nil, fmt.Errorf("read the contacts at the company: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[colleague])
}

// readCompanyHistory reads what the company's other contacts exchanged before
// this meeting. A conversation the room already has in its own history is left
// out, so it is not told twice.
func (s *Service) readCompanyHistory(
	ctx context.Context, tx pgx.Tx, room meeting, project *ids.ProjectID, now time.Time, own []HistoryIn,
) ([]HistoryIn, []colleague, error) {
	colleagues, err := s.readColleagues(ctx, tx, room)
	if err != nil || len(colleagues) == 0 {
		return nil, nil, err
	}
	people := make([]ids.UUID, 0, len(colleagues))
	for _, c := range colleagues {
		people = append(people, c.ID)
	}
	rows, err := s.readHistoryOf(ctx, tx, room, project, now, people, companyHistoryReadCap)
	if err != nil {
		return nil, nil, err
	}
	seen := make(map[string]bool, len(own))
	for _, row := range own {
		seen[row.ID] = true
	}
	kept := rows[:0]
	for _, row := range rows {
		if !seen[row.ID] {
			kept = append(kept, row)
		}
	}
	return kept, colleagues, nil
}

// companyHistory is the section's whole read: the topics to show and how many
// conversations the reader may know of but not open.
func (s *Service) companyHistory(
	ctx context.Context, tx pgx.Tx, room meeting, project *ids.ProjectID, now time.Time, own []HistoryIn,
) ([]CompanyTopicIn, int, error) {
	rows, colleagues, err := s.readCompanyHistory(ctx, tx, room, project, now, own)
	if err != nil || len(rows) == 0 {
		return nil, 0, err
	}
	people := make([]ids.UUID, 0, len(colleagues))
	for _, c := range colleagues {
		people = append(people, c.ID)
	}
	var readable []ids.UUID
	for _, row := range rows {
		if row.Withheld {
			continue
		}
		id, err := ids.Parse(row.ID)
		if err != nil {
			return nil, 0, fmt.Errorf("company history id %q: %w", row.ID, err)
		}
		readable = append(readable, id)
	}
	whose, err := contactOfActivity(ctx, tx, readable, people)
	if err != nil {
		return nil, 0, err
	}
	return foldCompanyHistory(rows, colleagues, whose), withheldCount(rows), nil
}

// contactOfActivity says which of these contacts each activity was with, by the
// participant or link that put it on their record. The first by name wins, so
// the answer does not vary between reads.
func contactOfActivity(
	ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID, people []ids.UUID,
) (map[string]ids.UUID, error) {
	out := map[string]ids.UUID{}
	if len(activityIDs) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT ON (reach.activity_id) reach.activity_id, reach.contact_id
		  FROM (SELECT activity_id, contact_id FROM activity_participant
		         WHERE activity_id = ANY($1) AND contact_id = ANY($2)
		        UNION ALL
		        SELECT activity_id, contact_id FROM activity_link
		         WHERE activity_id = ANY($1) AND contact_id = ANY($2)) reach
		 ORDER BY reach.activity_id, array_position($2::uuid[], reach.contact_id)`, activityIDs, people)
	if err != nil {
		return nil, fmt.Errorf("read who the company conversations were with: %w", err)
	}
	type pair struct{ Activity, Contact ids.UUID }
	pairs, err := pgx.CollectRows(rows, pgx.RowToStructByPos[pair])
	if err != nil {
		return nil, fmt.Errorf("read who the company conversations were with: %w", err)
	}
	for _, p := range pairs {
		out[p.Activity.String()] = p.Contact
	}
	return out, nil
}

// foldCompanyHistory picks the newest topics per colleague contact, grouped by
// contact with the most recently active first. A row the reader may not read
// is never named; it counts toward the history omission instead.
func foldCompanyHistory(rows []HistoryIn, colleagues []colleague, whose map[string]ids.UUID) []CompanyTopicIn {
	names := make(map[ids.UUID]string, len(colleagues))
	for _, c := range colleagues {
		names[c.ID] = c.Name
	}
	var order []ids.UUID
	topics := map[ids.UUID][]CompanyTopicIn{}
	for _, row := range rows {
		contact, ok := whose[row.ID]
		if row.Withheld || !ok || row.Subject == "" {
			continue
		}
		if _, started := topics[contact]; !started {
			if len(order) == companyContactCap {
				continue
			}
			order = append(order, contact)
		}
		if len(topics[contact]) == companyTopicsPerContact {
			continue
		}
		topics[contact] = append(topics[contact], CompanyTopicIn{
			ID: row.ID, Contact: names[contact], Kind: row.Kind, Subject: row.Subject,
			At: Occurred{At: row.At, DateOnly: row.DateOnly}, Direction: row.Direction,
		})
	}
	var out []CompanyTopicIn
	for _, contact := range order {
		out = append(out, topics[contact]...)
	}
	return out
}
