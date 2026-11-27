// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The one "Which deal is this about?" question the capture deal rule raises
// when several open deals fit a mail (capture.AskWhichDeal), asked through
// activities.AskWhichDealTx.
//
// It goes only to the colleague the mailbox confirms is responsible: the seat
// that sent it, or the one seat capture bound to its To. Copied seats, several
// seats, a typed address, a mailbox owner or a record owner are no evidence.
// With none, nobody is asked.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/dealfiler"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// whichDealActor is the system identity the question is proposed under.
const whichDealActor = "system:capture-which-deal"

// whichDealAsker proposes the question through the activities module. The
// deals it offers are the suggestions, so "none of them" declines those and
// no others.
type whichDealAsker struct {
	deals offeredDeals
	now   func() time.Time
}

// newWhichDealAsker is the one asker capture and the refile passes share, so
// both raise the same question. now is the clock that decides which mail is
// current.
func newWhichDealAsker(pool *pgxpool.Pool, now func() time.Time) whichDealAsker {
	return whichDealAsker{deals: deals.NewStore(InstallationDB(pool), DealsInstallation()), now: now}
}

// offeredDeals reads the open deals a question offers, from the deals module.
type offeredDeals interface {
	OfferedDeals(ctx context.Context, tx pgx.Tx, projectID, companyID ids.UUID, stakeholders []ids.UUID) ([]ids.UUID, error)
}

// Ask raises the question for current mail only: an old mail captured today
// makes no new work.
func (a whichDealAsker) Ask(ctx context.Context, tx pgx.Tx, q capture.WhichDeal) error {
	seat, err := a.respondent(ctx, tx, q.ActivityID.UUID)
	if err != nil || seat.IsZero() {
		return err
	}
	var current bool
	args := []any{q.ActivityID}
	arg := func(v any) int { args = append(args, v); return len(args) }
	query := `
		SELECT EXISTS (SELECT 1 FROM activity a WHERE a.id = $1 AND a.restricted_at IS NULL AND a.archived_at IS NULL
		   AND ` + storekit.CurrentSourceMailClause("a.id", a.now(), arg) + `)`
	if err := tx.QueryRow(ctx, query, args...).Scan(&current); err != nil || !current {
		return err
	}
	lang, err := seatLanguage(ctx, tx, seat)
	if err != nil {
		return err
	}
	offered, err := a.deals.OfferedDeals(ctx, tx, q.ProjectID, q.CompanyID, q.StakeholderIDs)
	if err != nil {
		return err
	}
	writer := principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: whichDealActor, OnBehalfOf: seat})
	return activities.AskWhichDealTx(writer, tx, activities.WhichDealQuestion{
		Mail: q.ActivityID, Seat: seat, Deals: offered,
		Reason: systemtext.Copy("capture.which_deal_subject", lang),
	})
}

// respondent is the one colleague the mail names as responsible, or zero.
// Mail only a shared mailbox captured goes to its work owner.
//
// Only rows the mailbox confirms count, never an address the sender typed. The
// seat imported the mail. It sent the mail with the provider attesting it, or
// holds a To row capture bound to the seat from an address. A seat row with
// no address only says whose mailbox the copy is in, not To from Cc.
func (a whichDealAsker) respondent(ctx context.Context, tx pgx.Tx, mail ids.UUID) (ids.UUID, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT p.user_id
		  FROM activity a
		  JOIN activity_participant p ON p.activity_id = a.id AND p.user_id IS NOT NULL
		  JOIN capture_import ci ON ci.activity_id = a.id AND ci.user_id = p.user_id
		 WHERE a.id = $1 AND a.restricted_at IS NULL AND a.archived_at IS NULL
		   AND CASE WHEN a.direction = 'outbound' THEN a.counterparty_outbound_attested AND p.role = 'from'
		            ELSE p.role = 'to' AND p.address IS NOT NULL END
		 ORDER BY p.user_id
		 LIMIT 2`, mail)
	if err != nil {
		return ids.Nil, fmt.Errorf("compose: reading who a deal question is for: %w", err)
	}
	seats, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil || len(seats) > 1 {
		return ids.Nil, err
	}
	var seat ids.UUID
	if len(seats) == 1 {
		seat = seats[0]
	} else if seat, err = storekit.SharedMailWorkOwnerTx(ctx, tx, mail, false); err != nil || seat.IsZero() {
		return ids.Nil, err
	}
	nobody := ids.Nil
	may, err := auth.SeatMayHoldPrivateWorkTx(ctx, tx, mail, &nobody, seat)
	if err != nil || !may {
		return ids.Nil, err
	}
	state, err := activities.NewTaskSeatState(ctx, tx, ids.From[ids.UserKind](seat))
	if err != nil || state == activities.SeatCannotHoldWork {
		return ids.Nil, err
	}
	return seat, nil
}

// seatLanguage is the seat's own UI language, else the installation's.
func seatLanguage(ctx context.Context, tx pgx.Tx, seat ids.UUID) (textlang.Lang, error) {
	var locale *string
	if err := tx.QueryRow(ctx, `SELECT locale FROM app_user WHERE id = $1`, seat).Scan(&locale); err != nil {
		return textlang.English, fmt.Errorf("compose: reading the asked colleague's language: %w", err)
	}
	if locale != nil && textlang.Known(*locale) {
		return textlang.Lang(*locale), nil
	}
	base, err := identity.LanguageOf(ctx, tx)
	if err != nil || !textlang.Known(base) {
		return textlang.English, err
	}
	return textlang.Lang(base), nil
}

// ActivityDealFiler is capture.DealFiler over the activities module (dealfiler.Activities).
type ActivityDealFiler = dealfiler.Activities
