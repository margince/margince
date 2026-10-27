// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// StampActivityAffordances sets what the reader may do with each row of a page.
// Writable says they may write it; CalendarManaged that an invitation holds it;
// AwaitingOutcome, judged at asOf, that its meeting still asks how it went.
func StampActivityAffordances(ctx context.Context, tx pgx.Tx, page []crmcontracts.Activity, asOf time.Time) error {
	if err := stampActivitiesWritable(ctx, tx, page); err != nil {
		return err
	}
	for i := range page {
		page[i].CalendarManaged = new(CalendarManaged(page[i]))
	}
	return stampAwaitingOutcome(ctx, tx, page, asOf)
}

// stampActivitiesWritable answers the question the update path asks: the
// object grant, then the row authority auth.ActivityWritableSubset answers
// for the whole page at once.
func stampActivitiesWritable(ctx context.Context, tx pgx.Tx, page []crmcontracts.Activity) error {
	may := make(map[ids.UUID]bool)
	if mayMutate(ctx) && auth.Allows(ctx, "activity", principal.ActionUpdate) && len(page) > 0 {
		named := make([]ids.UUID, len(page))
		for i := range page {
			named[i] = ids.UUID(page[i].Id)
		}
		var err error
		if may, err = auth.ActivityWritableSubset(ctx, tx, named); err != nil {
			return err
		}
	}
	for i := range page {
		w := may[ids.UUID(page[i].Id)]
		page[i].Writable = &w
	}
	return nil
}

// stampAwaitingOutcome asks awaitingOutcomeClause of the page's readable
// meetings. Readability is the row's own content_state, which the outcome lane
// gets from ReadableOnly: a meeting the reader cannot read is not theirs to answer.
func stampAwaitingOutcome(ctx context.Context, tx pgx.Tx, page []crmcontracts.Activity, asOf time.Time) error {
	at := make(map[ids.UUID]int)
	for i := range page {
		page[i].AwaitingOutcome = new(false)
		readable := page[i].ContentState == nil || *page[i].ContentState == crmcontracts.ActivityContentStateAvailable
		if page[i].Kind == crmcontracts.ActivityKindMeeting && readable {
			at[ids.UUID(page[i].Id)] = i
		}
	}
	if len(at) == 0 {
		return nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	owed, err := awaitingOutcomeClause(ctx, asOf, arg)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, sprintf(`SELECT a.id FROM activity a WHERE a.id = ANY($%d::uuid[]) AND %s`,
		arg(slices.Collect(maps.Keys(at))), owed), args...)
	if err != nil {
		return fmt.Errorf("activities: reading which meetings await an outcome: %w", err)
	}
	owedIDs, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("activities: reading which meetings await an outcome: %w", err)
	}
	for _, id := range owedIDs {
		page[at[id]].AwaitingOutcome = new(true)
	}
	return nil
}

// mayMutate is the seat ceiling the mutation admission applies before any
// grant: a read seat changes nothing, whatever its role says. The system
// principal holds no seat and is trusted by construction, as in auth.Require.
func mayMutate(ctx context.Context) bool {
	p, ok := principal.Actor(ctx)
	return ok && (p.Type == principal.PrincipalSystem || p.SeatType.CanMutate())
}
