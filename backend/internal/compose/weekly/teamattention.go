// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// What needs the manager's attention now, over the team the week froze.
//
// Computed when the review is read, never stored. A finding cleared on Tuesday
// must not read as open in last week's review. Nor can the week-end rewind say
// which deals were stalled: it restores status and close dates, not activity.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// teamExceptionsCap bounds the two lists together: a manager reading before a
// meeting has minutes, and the findings come first.
const teamExceptionsCap = 25

// StaleDeal is an open deal of the team with no activity for the stalled
// period, as it stands now.
type StaleDeal struct {
	DealID         ids.UUID
	DealName       string
	OwnerID        ids.UUID
	LastActivityAt time.Time
	DaysQuiet      int
}

// TeamAttentionItems are the live lists the team read carries. Read says the
// seam was bound and asked, so an empty list is "nothing" and not "not asked".
type TeamAttentionItems struct {
	Read       bool
	Exceptions []crmcontracts.InputCheck
	StaleDeals []StaleDeal
}

// TeamAttention answers the live lists over the deals a set of members owns
// now. Implementations apply the caller's deal permission and row scope in SQL.
type TeamAttention interface {
	TeamExceptions(ctx context.Context, tx pgx.Tx, members []ids.UUID, limit int) ([]crmcontracts.InputCheck, error)
	StalledDealsOf(ctx context.Context, tx pgx.Tx, members []ids.UUID, limit int) ([]StaleDeal, error)
}

// readTeamAttention fills the live lists for a review already read.
func (e *Engine) readTeamAttention(ctx context.Context, tx pgx.Tx, review *TeamReview) error {
	if e.attention == nil {
		return nil
	}
	// The frozen roster, narrowed to who is on the team now. A member who has
	// since moved owns deals another lead answers for. Live data is never wider
	// than the team the reader may open today.
	members, err := identity.LiveTeamMembersAmongTx(ctx, tx, review.TeamID, rosterIDs(*review))
	if err != nil {
		return fmt.Errorf("weekly: reading the team's live members: %w", err)
	}
	if len(members) == 0 {
		review.Attention = TeamAttentionItems{Read: true}
		return nil
	}
	exceptions, err := e.attention.TeamExceptions(ctx, tx, members, teamExceptionsCap)
	if err != nil {
		return err
	}
	review.Attention = TeamAttentionItems{Read: true, Exceptions: exceptions}
	if room := teamExceptionsCap - len(exceptions); room > 0 {
		stale, err := e.attention.StalledDealsOf(ctx, tx, members, room)
		if err != nil {
			return err
		}
		review.Attention.StaleDeals = stale
	}
	return nil
}

// rosterIDs is the frozen roster, or the measured reps on a snapshot written
// before the roster was kept.
func rosterIDs(review TeamReview) []ids.UUID {
	out := make([]ids.UUID, 0, len(review.Members))
	for _, m := range review.Members {
		out = append(out, m.UserID)
	}
	if len(out) > 0 {
		return out
	}
	for _, rep := range review.Reps {
		out = append(out, rep.UserID)
	}
	return out
}
