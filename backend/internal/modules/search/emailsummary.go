// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// hitTypeActivity is the `type` an activity hit carries on the wire, and the
// same word the activity branch declares in searchBranches.
const hitTypeActivity = "activity"

// EmailSummaryReader answers the email row of each of these activities, for
// THIS caller, inside the caller's own transaction. It takes the whole page's
// activity ids at once: a per-hit read cost a round trip per hit, which a page
// of 200 turns into 200 statements.
//
// An id missing from the returned map is one this caller gets no email row
// for — it is not an email, or its content is not theirs. Both are ordinary
// answers, not failures.
//
// It is the activities store's own reader, injected by compose because a
// module never imports a sibling. Calling that reader rather than projecting a
// second email row here is what keeps a search hit and a timeline row the same
// row, down to the preview text and the access badge.
type EmailSummaryReader func(
	ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID,
) (map[ids.UUID]crmcontracts.EmailSummary, error)

// attachEmailSummaries fills EmailSummary on the email hits of one page.
//
// It changes nothing about WHICH rows are on the page. The activity branch is
// already gated by auth.ActivityContentClause (branchScope), so a withheld
// email produced no hit to enrich in the first place, and the reader runs that
// same content gate a second time rather than trusting this one — the two
// agree by construction, and the one that would matter if they ever did not is
// the reader's, which is the one standing between a preview and a person who
// may not read it.
//
// A read failure fails the search rather than answering with a silent gap. The
// reader runs the caller's own gate, so an error here is that gate refusing to
// render, and a page that quietly dropped the summaries would show a searcher
// email hits stripped of their subject with no way to tell that from an email
// that has none.
func (s *Store) attachEmailSummaries(ctx context.Context, tx pgx.Tx, hits []Hit) error {
	if s.emailSummaries == nil {
		return nil
	}
	var activityIDs []ids.UUID
	for i := range hits {
		if hits[i].Type == hitTypeActivity {
			activityIDs = append(activityIDs, hits[i].ID)
		}
	}
	if len(activityIDs) == 0 {
		return nil
	}
	summaries, err := s.emailSummaries(ctx, tx, activityIDs)
	if err != nil {
		return fmt.Errorf("search: reading this page's email rows: %w", err)
	}
	for i := range hits {
		if hits[i].Type != hitTypeActivity {
			continue
		}
		// Absent means this activity is not an email — a call, a note, a task
		// and a meeting are activities too, and each keeps the generic hit it
		// has always had.
		summary, ok := summaries[hits[i].ID]
		if !ok {
			continue
		}
		// Copied into a local before its address is taken: `summary` is the
		// loop's own variable and aliasing it would give every hit the last
		// one's row.
		row := summary
		hits[i].EmailSummary = &row
	}
	return nil
}
