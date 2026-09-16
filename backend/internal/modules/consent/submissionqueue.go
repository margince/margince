// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// THE QUEUE of what subjects proposed, read a page at a time.
//
// Its own file because it is its own act. The review beside it takes one
// submission and records a decision about it; this answers what is waiting, in
// the order somebody works it, and has the whole of a keyset walk to itself —
// the bound, the order, the cursor and the arm that resumes on it. A keyset
// buried among a review's branches is where a tie-break goes wrong unnoticed.
//
// The row's columns and the comparison SQL stay with the review: they are what
// a submission IS, and both surfaces read them.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ListSubmissionsInput narrows the queue.
type ListSubmissionsInput struct {
	// ContactID lists one contact's proposals. Zero lists every contact's.
	ContactID ids.ContactID
	// Resolved filters on whether somebody has decided. Nil returns both.
	Resolved *bool
	Limit    int
	// Cursor continues a walk. Empty starts one.
	Cursor string
}

// submissionListMax and submissionListDefault bound one page of the queue.
type submissionPageBound int

const (
	submissionListMax     submissionPageBound = 200
	submissionListDefault submissionPageBound = 50
)

// submissionCursor is where a page of the queue stopped: all three parts of its
// order, because the order has three parts.
//
// `Resolved` is the leading key and it is a BOOLEAN, so a walk resumed without
// it re-enters at the first unresolved row and hands a reviewer the queue they
// have already worked. `SubmittedAt` alone is not a position either — two
// subjects can send in the same second, and a nightly batch of confirm links
// makes that ordinary rather than rare — so the id is the tie-break, exactly as
// the ORDER BY has it.
type submissionCursor struct {
	Resolved    bool      `json:"done"`
	SubmittedAt time.Time `json:"sent"`
	ID          ids.UUID  `json:"id"`
}

// ListSubmissions answers what subjects have sent and what became of it.
//
// UNRESOLVED FIRST, oldest first within that. A correction somebody sent three
// weeks ago is the one still waiting, and a queue that sorted newest-first
// would bury it under everything that arrived since.
//
// PAGED, because the bound alone made the tail unreachable. A limit with no
// continuation answered the first fifty and said nothing about the rest: past
// fifty unresolved rows the panel drew a complete-looking queue and the ones
// that fell off the end were the newest, with no error, no count and no way to
// ask for more. The resolved archive was cut the same way, permanently.
func (s *Store) ListSubmissions(
	ctx context.Context, in ListSubmissionsInput,
) ([]Submission, storekit.Page, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, storekit.Page{}, err
	}
	limit := in.Limit
	if limit <= 0 || limit > int(submissionListMax) {
		limit = int(submissionListDefault)
	}
	var out []Submission
	var page storekit.Page
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		where, err := submissionQueueClause(ctx, in, arg)
		if err != nil {
			return err
		}
		// ONE MORE THAN ASKED FOR, so `has_more` is a fact rather than a guess:
		// a page that came back exactly full says nothing about what is behind
		// it, and guessing either way is a lie half the time.
		rows, err := tx.Query(ctx, `
			SELECT `+prefixed(submissionColumns, "s")+`,
			       coalesce(c.full_name, ''), `+currentValueSQL("c")+`
			  FROM contact_confirm_submission s
			  JOIN contact c ON c.id = s.contact_id
			 WHERE `+where+`
			 ORDER BY s.resolution IS NOT NULL, s.submitted_at, s.id
			 LIMIT `+fmt.Sprintf("$%d", arg(limit+1)), args...)
		if err != nil {
			return fmt.Errorf("consent: listing what subjects proposed: %w", err)
		}
		out, err = pgx.CollectRows(rows, scanListedSubmission)
		if err != nil {
			return err
		}
		if len(out) <= limit {
			return nil
		}
		out = out[:limit]
		last := out[limit-1]
		token, err := storekit.EncodeOpaque(submissionCursor{
			Resolved: last.Resolution != nil, SubmittedAt: last.SubmittedAt, ID: last.ID,
		})
		if err != nil {
			return err
		}
		page = storekit.Page{HasMore: true, NextCursor: token}
		return nil
	})
	if err != nil {
		return nil, storekit.Page{}, err
	}
	return out, page, nil
}

// submissionQueueClause narrows the queue: who may see it, which rows were
// asked for, and where the last page stopped.
//
// Split out of ListSubmissions because it is the whole of the WHERE and the
// caller is the whole of the read — and because a keyset arm buried among a
// dozen other branches is where a tie-break goes wrong unnoticed.
func submissionQueueClause(
	ctx context.Context, in ListSubmissionsInput, arg func(any) int,
) (string, error) {
	// THE ROW SCOPE of the contact each submission is about, so a reviewer sees
	// proposals only for contacts they could open. A submission holds the
	// subject's own words about themselves, which is no less protected for
	// having been typed by them.
	scope, err := auth.ScopeClauseFor(ctx, "contact", "c", arg)
	if err != nil {
		return "", err
	}
	where := "TRUE"
	if scope != "" {
		where = scope
	}
	if !in.ContactID.IsZero() {
		where += fmt.Sprintf(" AND s.contact_id = $%d", arg(in.ContactID.UUID))
	}
	if in.Resolved != nil {
		if *in.Resolved {
			where += " AND s.resolution IS NOT NULL"
		} else {
			where += " AND s.resolution IS NULL"
		}
	}
	if in.Cursor == "" {
		return where, nil
	}
	// THE KEYSET ARM, spelled as a row comparison rather than as an OR of
	// three: it is the order the index reads, and one fewer place to get a
	// tie-break wrong.
	after, err := storekit.DecodeOpaque[submissionCursor](in.Cursor)
	// EVERY part, not the id alone. The envelope proves the token is one of
	// ours, not that it names a position in THIS queue: a cursor from any other
	// paged route spells `id` the same way and decodes cleanly here, leaving
	// the instant at its zero value — which pages from the year zero and hands
	// the reviewer the whole queue again as though it were their next page. A
	// malformed token is the client's mistake and says so; answering an empty
	// page would tell a reviewer their queue was clear, which is the one answer
	// this route must never guess at.
	if err != nil || after.SubmittedAt.IsZero() || after.ID.IsZero() {
		return "", &storekit.MalformedCursorError{}
	}
	return where + storekit.SQLf(
		" AND (s.resolution IS NOT NULL, s.submitted_at, s.id) > ($%d, $%d, $%d)",
		arg(after.Resolved), arg(after.SubmittedAt), arg(after.ID)), nil
}
