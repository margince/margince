// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The one-time pass that files old mail of a deal's stakeholders. Mail captured
// before the stakeholder rung existed, or before a seat was added, never
// reached the deal. The silent close then counted the deal as unanswered.
// The pass decides each mail with the capture rule itself (fileOnDeal). Only
// the mail it looks at and the deals it may file on differ from the refile.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/dealseat"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// StakeholderBackfillVersion is the version of the pass's rule. A later change
// to the rule raises it, and the pass then runs once more.
const StakeholderBackfillVersion = 1

// StakeholderBackfillState is the pass's bookkeeping in installation_state. A
// workspace reset clears it, and the pass then runs again, which is safe
// because it files only mail that still has no deal link.
type StakeholderBackfillState struct {
	Version int `json:"version"`
	// Window is deals.silent_close_days, frozen when the pass starts.
	Window  int       `json:"window_days"`
	AsOf    time.Time `json:"as_of"`
	Ceiling ids.UUID  `json:"ceiling"`
	// The closes come first, so mail from before a close goes to the deal
	// that was closed before an open deal can claim it. ClosesAfter is the
	// last close finished, and ClosesDone ends that phase. MailAfter and
	// FiledOnClose are the position inside the close in hand.
	ClosesAfter  ids.UUID `json:"closes_after"`
	ClosesDone   bool     `json:"closes_done"`
	MailAfter    ids.UUID `json:"mail_after"`
	FiledOnClose int      `json:"filed_on_close"`
	// OpenAfter is the last mail the open phase inspected.
	OpenAfter  ids.UUID   `json:"open_after"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	// The counts are what the pass did, for the one log row it writes at the end.
	Inspected  int `json:"inspected"`
	Filed      int `json:"filed"`
	ModelCalls int `json:"model_calls"`
	Offered    int `json:"offered"`
}

// Finished reports whether the pass is complete at the current version.
func (s StakeholderBackfillState) Finished() bool {
	return s.Version == StakeholderBackfillVersion && s.FinishedAt != nil
}

// StakeholderBackfillDoneTx answers whether the pass has finished. The nightly
// silent close and the sweep that schedules the pass ask it through this one
// function.
func StakeholderBackfillDoneTx(ctx context.Context, tx pgx.Tx) (bool, error) {
	state, found, err := storekit.InstallationStateTx[StakeholderBackfillState](ctx, tx, storekit.StateStakeholderMailBackfill)
	return found && state.Finished(), err
}

// Count adds one mail's filing to the pass's counts.
func (s *StakeholderBackfillState) Count(r BackfillResult) {
	s.Inspected++
	if r.Filed {
		s.Filed++
	}
	if r.Asked {
		s.ModelCalls++
	}
}

// BackfillResult is what one mail's filing did.
type BackfillResult struct {
	// Asked is whether it asked the model.
	Asked bool
	// Filed is whether the mail now sits on a deal.
	Filed bool
	// Deferred is whether the model was blocked or out of budget, so the mail
	// was not decided and a later pass must try it again.
	Deferred bool
}

// stakeholderMailSQL selects live captured mail that has no deal link and
// that names a contact holding a current seat. deal narrows the seat to one
// deal when it is not empty. It does not read the deal's status: the decision
// stays in the capture rule.
func stakeholderMailSQL(dealPredicate string) string {
	return `SELECT a.id FROM activity a
 WHERE a.kind IN ('email','meeting') AND ` + capturedConnectorActivitySQL + `
 AND NOT EXISTS (SELECT 1 FROM activity_link filed WHERE filed.activity_id=a.id AND filed.entity_type='deal')
 AND EXISTS (SELECT 1 FROM relationship seat WHERE ` + dealseat.CurrentSQL("seat") + dealPredicate + `
 AND seat.contact_id IN (
 SELECT p.contact_id FROM activity_participant p WHERE p.activity_id=a.id AND p.contact_id IS NOT NULL
 UNION SELECT l.contact_id FROM activity_link l WHERE l.activity_id=a.id AND l.contact_id IS NOT NULL))`
}

// StakeholderBackfillCandidatesTx lists unfiled mail of a seated contact, in id
// order, after the mail given and up to ceiling, at most limit. The mail
// occurred in the windowDays before asOf.
func (r DealRepair) StakeholderBackfillCandidatesTx(
	ctx context.Context, tx pgx.Tx, asOf time.Time, windowDays int, after, ceiling ids.UUID, limit int,
) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	query := stakeholderMailSQL(``) + ` AND a.occurred_at <= $1::timestamptz AND a.occurred_at >= $1::timestamptz - make_interval(days => $2) -- occurred_at raw: the window is whole days
 AND a.id > $3 AND a.id <= $4 ORDER BY a.id LIMIT $5`
	return collectIDs(ctx, tx, query, asOf, windowDays, after, ceiling, limit)
}

// SilentCloseCandidatesTx lists unfiled mail of a contact seated on deal, in id
// order, after the mail given, at most limit. The mail occurred in the
// windowDays before closedAt and not after it.
func (r DealRepair) SilentCloseCandidatesTx(
	ctx context.Context, tx pgx.Tx, deal ids.UUID, closedAt time.Time, windowDays int, after ids.UUID, limit int,
) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	query := stakeholderMailSQL(` AND seat.deal_id = $1`) + ` AND a.occurred_at <= $2::timestamptz
 AND a.occurred_at >= $2::timestamptz - make_interval(days => $3) -- occurred_at raw: the window is whole days
 AND a.id > $4 ORDER BY a.id LIMIT $5`
	return collectIDs(ctx, tx, query, deal, closedAt, windowDays, after, limit)
}

func collectIDs(ctx context.Context, tx pgx.Tx, query string, args ...any) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("capture: listing stakeholder mail to file: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

// BackfillStakeholderMail decides one mail of the first phase as capture
// would, on the open deals, when it occurred in the windowDays before asOf.
func (r DealRepair) BackfillStakeholderMail(
	ctx context.Context, db TxRunner, activity ids.ActivityID, asOf time.Time, windowDays int,
) (BackfillResult, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return BackfillResult{}, err
	}
	plan := refilePlan{
		eligible: func(arg func(any) int) string {
			at := arg(asOf)
			return fmt.Sprintf(`a.occurred_at <= $%d::timestamptz AND a.occurred_at >= $%d::timestamptz - make_interval(days => $%d) -- occurred_at raw: the window is whole days`+"\n", at, at, arg(windowDays))
		},
		candidates: r.Candidates, ask: r.Ask,
	}
	return r.backfill(ctx, db, activity, plan)
}

// FileOnSilentClose decides one mail of the second phase. The deal Margince
// closed for silence at closedAt competes with the stakeholder's other open
// deals, as it did while open. Nobody is asked, because a question about a
// closed deal cannot offer it.
func (r DealRepair) FileOnSilentClose(
	ctx context.Context, db TxRunner, activity ids.ActivityID, deal ids.UUID, closedAt time.Time, windowDays int,
) (BackfillResult, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return BackfillResult{}, err
	}
	plan := refilePlan{
		eligible: func(arg func(any) int) string {
			at := arg(closedAt)
			return fmt.Sprintf(`a.occurred_at <= $%d::timestamptz AND a.occurred_at >= $%d::timestamptz - make_interval(days => $%d) -- occurred_at raw: the window is whole days`+"\n", at, at, arg(windowDays))
		},
		candidates: r.AdmitClosed(deal), admit: deal,
	}
	return r.backfill(ctx, db, activity, plan)
}

func (r DealRepair) backfill(ctx context.Context, db TxRunner, activity ids.ActivityID, plan refilePlan) (BackfillResult, error) {
	return r.checkedRefile(ctx, db, activity, plan)
}
