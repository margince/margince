// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

// The two directions of a contact's silence, answered for a SET of contacts in
// one statement. The page's last-touch section is this over a set of one; the
// ranked queue asks it for every contact a page names. One reader, so the row
// a rep acts on and the record it opens cannot disagree about who wrote last.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// LastTouch is when they last wrote to us and when we last wrote to them. Nil
// on either side means it never happened, not that the answer was withheld: a
// withheld answer is a refusal from LastTouchFor, never a zero in its map.
type LastTouch struct {
	InboundAt  *time.Time
	OutboundAt *time.Time
}

// LastTouchFor answers the two moments for every contact in the set the caller
// may read, in ONE statement: a queue page naming thirty contacts costs one
// read rather than thirty.
//
// A contact the caller may not read, or one that is archived, is absent from
// the answer — the same refusal the record read makes, one row at a time. The
// caller who cannot read activity at all is refused outright, which is what
// the page reports as a withheld section and a queue row as no moments.
//
// THE TWO DIRECTIONS ASK DIFFERENT QUESTIONS, and only one of them is about
// reachability. "You wrote to them" is satisfied by the message reaching them,
// which is what an outbound message linked to this contact means. "THEY wrote"
// is a claim about authorship, and a thread is linked to everybody it concerns
// — so reading it off reachability told a reader "they wrote last" about a
// message somebody else sent into a conversation this contact is on.
//
// An aggregate, not a per-row read: it has to agree for every colleague, so it
// asks whether the WORKSPACE may see the row (auth.AudienceWorkspaceOnly)
// rather than the caller-scoped arm the timeline uses for content — a private
// message narrowed to its participants must not move the date a colleague
// outside them reads here, the same rule relationship strength holds
// (contacts/strength.go).
func LastTouchFor(ctx context.Context, tx pgx.Tx, contactIDs []ids.ContactID, opts AssembleOptions) (map[ids.ContactID]LastTouch, error) {
	if err := requireRead(ctx, "activity"); err != nil {
		return nil, err
	}
	if err := requireRead(ctx, "contact"); err != nil {
		return nil, err
	}
	out := make(map[ids.ContactID]LastTouch, len(contactIDs))
	if len(contactIDs) == 0 {
		return out, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	wantedPos := arg(contactIDs)
	visible, err := auth.ScopeClauseFor(ctx, "contact", "c", arg)
	if err != nil {
		return nil, err
	}
	if visible == "" {
		visible = scopeAll
	}
	scope, err := activityDiscoverScope(ctx, arg)
	if err != nil {
		return nil, err
	}
	// The activities this contact is on, shared by both directions; the
	// inbound arm adds authorship on top.
	reached := fmt.Sprintf(`FROM activity a
		WHERE a.archived_at IS NULL AND %s AND (%s)%s%s`,
		fmt.Sprintf(contactReachesActivity, "c.id"), scope, projectScope(opts, arg),
		auth.AudienceWorkspaceOnly("a"))
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT c.id,
		       (SELECT max(a.occurred_at) %[1]s AND a.direction = 'inbound' AND %[2]s),
		       (SELECT max(a.occurred_at) %[1]s AND a.direction = 'outbound')
		FROM contact c
		WHERE c.id = ANY($%[3]d) AND c.archived_at IS NULL AND (%[4]s)`,
		reached, contacts.SenderPredicate("c.id", "a"), wantedPos, visible), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var contactID ids.ContactID
		var touch LastTouch
		if err := rows.Scan(&contactID, &touch.InboundAt, &touch.OutboundAt); err != nil {
			return nil, err
		}
		out[contactID] = touch
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
