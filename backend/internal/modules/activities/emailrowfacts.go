// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WithEmailRowFacts fills in what a page's email rows say beyond the message
// itself: how many files came with it, what happened when it was sent, and
// whether everyone here can read it.
//
// ONE entry point rather than a pass per fact, because they are one
// obligation. Each is read once per page and refused on a withheld row. Who
// wrote each row and whom it was with ride the same read of its parties.
func WithEmailRowFacts(ctx context.Context, tx pgx.Tx, page []crmcontracts.Activity, asOf time.Time) error {
	parties, err := pageParties(ctx, tx, page)
	if err != nil {
		return err
	}
	if err := withMessageVoices(ctx, tx, page, parties); err != nil {
		return err
	}
	applyCounterparties(page, parties)
	if err := withMessageMoves(ctx, tx, page, asOf); err != nil {
		return err
	}
	emailIDs := emailIDsOf(page)
	if len(emailIDs) == 0 {
		return nil
	}
	counts, err := AttachmentCountsFor(ctx, tx, emailIDs)
	if err != nil {
		return err
	}
	applyAttachmentCounts(page, counts)

	states, err := EmailStatesFor(ctx, tx, emailIDs, asOf)
	if err != nil {
		return err
	}
	applyEmailStates(page, states)

	// The badge starts at the narrow word and is widened only here, where the
	// question can actually be asked. A projection with no transaction cannot
	// know whether a workspace audience is narrowed by the records it is filed
	// against, and a privacy label that guesses must guess DOWN: overstating
	// the audience makes a reader more careful than they need to be, and
	// understating it makes them less.
	reach, err := auth.ActivitiesReachingEverySeat(ctx, tx, emailIDs)
	if err != nil {
		return err
	}
	applyEverySeatReach(page, reach)
	if err := withCalendarInvitations(ctx, tx, page, asOf); err != nil {
		return err
	}
	return withCalendarChanges(ctx, tx, page)
}

// applyCounterparties names the far side on each readable email row. It uses
// the message viewer's own rule, so a list and the viewer name one person. A
// withheld row names nobody: who it was with is content.
func applyCounterparties(page []crmcontracts.Activity, parties map[ids.UUID]emailParties) {
	for i := range page {
		summary := page[i].EmailSummary
		if summary == nil || !contentReadable(page[i]) {
			continue
		}
		summary.Counterparty, summary.CounterpartyContactId =
			rowCounterparty(page[i].Direction, parties[ids.UUID(page[i].Id)])
	}
}

// applyEverySeatReach widens `team` to `workspace` on the rows a stranger can
// reach. Only that word is touched: participants, selected and withheld are
// narrower answers that a workspace-wide filing does not change.
func applyEverySeatReach(page []crmcontracts.Activity, reach map[ids.UUID]bool) {
	for i := range page {
		summary := page[i].EmailSummary
		if summary == nil || summary.DisplayStatus != crmcontracts.EmailAccessStatusTeam {
			continue
		}
		if reach[ids.UUID(summary.ActivityId)] {
			summary.DisplayStatus = crmcontracts.EmailAccessStatusWorkspace
		}
	}
}

// messageIDsOf is the chat messages on a page the reader may read. A withheld
// one draws no move, as a withheld mail draws no badge.
func messageIDsOf(page []crmcontracts.Activity) []ids.UUID {
	var out []ids.UUID
	for i := range page {
		withheld := page[i].ContentState != nil && *page[i].ContentState == crmcontracts.ActivityContentStateWithheld
		if page[i].Kind != crmcontracts.ActivityKindMessage || withheld {
			continue
		}
		out = append(out, ids.UUID(page[i].Id))
	}
	return out
}

// withMessageMoves sets each readable chat message's move, by the rules the
// mail badge follows.
func withMessageMoves(ctx context.Context, tx pgx.Tx, page []crmcontracts.Activity, asOf time.Time) error {
	messages := messageIDsOf(page)
	if len(messages) == 0 {
		return nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	wanted := arg(messages)
	in, err := moveInputs(ctx, arg, asOf)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT a.id, `+moveSQL(in, conversationKinds)+`
 FROM activity a WHERE a.id = ANY(`+fmt.Sprintf("$%d", wanted)+`) AND a.restricted_at IS NULL`, args...)
	if err != nil {
		return fmt.Errorf("activities: reading whose move each chat message is: %w", err)
	}
	moves, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (messageMoveRow, error) {
		var m messageMoveRow
		err := row.Scan(&m.id, &m.move)
		return m, err
	})
	if err != nil {
		return fmt.Errorf("activities: reading whose move each chat message is: %w", err)
	}
	byID := make(map[ids.UUID]crmcontracts.ActivityMessageMove, len(moves))
	for _, m := range moves {
		byID[m.id] = m.move
	}
	for i := range page {
		if move, ok := byID[ids.UUID(page[i].Id)]; ok {
			page[i].MessageMove = &move
		}
	}
	return nil
}

// messageMoveRow is one chat message's move as the page pass reads it.
type messageMoveRow struct {
	id   ids.UUID
	move crmcontracts.ActivityMessageMove
}
