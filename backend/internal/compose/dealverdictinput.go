// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What Margince's nightly verdict reads about one open deal. Activities are
// those every member may read, because the reason and its citations reach
// Home and the deal page.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/dealverdict"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// dealVerdictActivities bounds the activities one verdict reads, newest first.
const dealVerdictActivities = 30

// dealVerdictBodyChars bounds each activity's text in the prompt.
const dealVerdictBodyChars = 1500

// dealVerdictChecksumSQL is what a verdict read about one deal. It moves when
// the viability evidence, the deal's own fields, an offer or a stage move
// does. It is stored after the verdict's own writes, so those never ask for a
// second reading.
func dealVerdictChecksumSQL(deal string) string {
	return `md5(concat_ws('|', ` + viabilityFingerprintSQL(deal) + `, ` + deal + `.amount_minor, ` + deal + `.currency, ` +
		deal + `.win_probability_override, ` + deal + `.win_probability_set_by, ` + deal + `.stage_id,
		(SELECT concat_ws('|', count(*), max(vo.updated_at)) FROM offer vo WHERE vo.deal_id = ` + deal + `.id),
		(SELECT concat_ws('|', count(*), max(vh.changed_at)) FROM deal_stage_history vh WHERE vh.deal_id = ` + deal + `.id)))`
}

// dealVerdictRead is one deal's input and the state the rules check.
type dealVerdictRead struct {
	in     dealverdict.Input
	state  dealverdict.State
	undone map[string]time.Time
	// newestEvidenceAt is when the newest activity, offer or stage move
	// arrived. An Undo holds until something arrives after it.
	newestEvidenceAt time.Time
}

// readDealVerdict reads one deal's input in the caller's transaction.
func readDealVerdict(ctx context.Context, tx pgx.Tx, deal ids.UUID, prior json.RawMessage, today time.Time) (dealVerdictRead, error) {
	read := dealVerdictRead{in: dealverdict.Input{Today: today}}
	if err := readVerdictDeal(ctx, tx, deal, &read); err != nil {
		return read, err
	}
	steps := []func(context.Context, pgx.Tx, ids.UUID, *dealVerdictRead) error{
		readVerdictActivities, readVerdictOffers, readVerdictStageMoves, readVerdictEdits,
	}
	for _, step := range steps {
		if err := step(ctx, tx, deal, &read); err != nil {
			return read, err
		}
	}
	if len(prior) > 0 {
		if err := json.Unmarshal(prior, &read.in.Prior); err != nil {
			return read, fmt.Errorf("compose: decoding the deal's earlier verdict: %w", err)
		}
	}
	read.state.Held = dealverdict.Holds(read.undone, read.newestEvidenceAt)
	return read, nil
}

func readVerdictDeal(ctx context.Context, tx pgx.Tx, deal ids.UUID, read *dealVerdictRead) error {
	seen := &read.state.Seen
	err := tx.QueryRow(ctx, `
		SELECT d.name, st.name, d.amount_minor, d.currency::text, d.expected_close_date, d.close_date_provisional,
		       d.win_probability_override, d.win_probability_set_by,
		       `+deals.WinProbabilitySQL("d")+`, `+deals.WinProbabilitySourceSQL("d")+`
		  FROM deal d JOIN stage st ON st.id = d.stage_id
		 WHERE d.id = $1`, deal).
		Scan(&read.in.DealName, &read.in.Stage, &seen.AmountMinor, &seen.Currency, &seen.CloseDate,
			&read.in.Provisional, &seen.Probability, &seen.ProbabilitySource,
			&read.in.Probability, &read.in.ProbabilitySource)
	if err != nil {
		return fmt.Errorf("compose: reading the deal a verdict judges: %w", err)
	}
	read.in.AmountMinor, read.in.Currency, read.in.CloseDate = seen.AmountMinor, seen.Currency, seen.CloseDate
	return nil
}

func readVerdictActivities(ctx context.Context, tx pgx.Tx, deal ids.UUID, read *dealVerdictRead) error {
	rows, err := tx.Query(ctx, `
		SELECT a.id::text, a.kind, coalesce(a.direction, ''), a.occurred_at, coalesce(a.subject, ''),
		       left(coalesce(a.body, ''), $2), greatest(a.updated_at, l.created_at)
		  FROM activity_link l JOIN activity a ON a.id = l.activity_id
		 WHERE l.deal_id = $1 AND a.archived_at IS NULL AND a.kind IN ('email', 'meeting', 'call', 'note')
		   AND `+visibleToAllSQL("a")+` AND `+relstrength.OriginIsEngagementSQL("a")+`
		 ORDER BY a.occurred_at DESC, a.id DESC LIMIT $3`, deal, dealVerdictBodyChars, dealVerdictActivities)
	if err != nil {
		return fmt.Errorf("compose: reading the deal's activities: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var activity dealverdict.Activity
		var arrived time.Time
		if err := rows.Scan(&activity.ID, &activity.Kind, &activity.Direction, &activity.OccurredAt,
			&activity.Subject, &activity.Body, &arrived); err != nil {
			return err
		}
		read.in.Activities = append(read.in.Activities, activity)
		read.newestEvidenceAt = laterOf(read.newestEvidenceAt, arrived)
	}
	slices.Reverse(read.in.Activities)
	return rows.Err()
}

func readVerdictOffers(ctx context.Context, tx pgx.Tx, deal ids.UUID, read *dealVerdictRead) error {
	rows, err := tx.Query(ctx, `
		SELECT gross_minor, currency::text, status, accepted_at IS NOT NULL, updated_at
		  FROM offer WHERE deal_id = $1 AND archived_at IS NULL
		 ORDER BY accepted_at NULLS FIRST, id`, deal)
	if err != nil {
		return fmt.Errorf("compose: reading the deal's offers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var offer dealverdict.Offer
		var updated time.Time
		if err := rows.Scan(&offer.GrossMinor, &offer.Currency, &offer.Status, &offer.Accepted, &updated); err != nil {
			return err
		}
		read.in.Offers = append(read.in.Offers, offer)
		read.newestEvidenceAt = laterOf(read.newestEvidenceAt, updated)
		if offer.Accepted {
			accepted := offer
			read.state.AcceptedOffer = &accepted
		}
	}
	return rows.Err()
}

func readVerdictStageMoves(ctx context.Context, tx pgx.Tx, deal ids.UUID, read *dealVerdictRead) error {
	rows, err := tx.Query(ctx, `
		SELECT coalesce(fs.name, ''), coalesce(ts.name, ''), h.changed_at
		  FROM deal_stage_history h
		  LEFT JOIN stage fs ON fs.id = h.from_stage_id
		  LEFT JOIN stage ts ON ts.id = h.to_stage_id
		 WHERE h.deal_id = $1 ORDER BY h.changed_at, h.id`, deal)
	if err != nil {
		return fmt.Errorf("compose: reading the deal's stage moves: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var move dealverdict.StageMove
		if err := rows.Scan(&move.From, &move.To, &move.At); err != nil {
			return err
		}
		read.in.StageMoves = append(read.in.StageMoves, move)
		read.newestEvidenceAt = laterOf(read.newestEvidenceAt, move.At)
	}
	return rows.Err()
}

// readVerdictEdits reads when a colleague last wrote each field a verdict may
// change, and when the owner last undid each verdict correction.
func readVerdictEdits(ctx context.Context, tx pgx.Tx, deal ids.UUID, read *dealVerdictRead) error {
	read.state.HumanEditedAt = map[string]time.Time{}
	read.undone = map[string]time.Time{}
	rows, err := tx.Query(ctx, `
		SELECT c.name, max(a.occurred_at)
		  FROM (VALUES ($2, ARRAY['expected_close_date']), ($3, ARRAY['amount_minor', 'currency']),
		               ($4, ARRAY['win_probability_override'])) c(name, cols)
		  JOIN audit_log a ON a.entity_type = 'deal' AND a.entity_id = $1 AND a.actor_type = 'human'
		                  AND a.after ?| c.cols
		 GROUP BY c.name
		UNION ALL
		SELECT 'undone:' || correction, max(reversed_at) FROM deal_correction
		 WHERE deal_id = $1 AND reversed_at IS NOT NULL AND correction IN ($2, $3, $4)
		 GROUP BY correction`, deal, deals.VerdictCloseDateCorrection, deals.VerdictAmountCorrection,
		deals.VerdictProbabilityCorrection)
	if err != nil {
		return fmt.Errorf("compose: reading the deal's edits and undos: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var at time.Time
		if err := rows.Scan(&name, &at); err != nil {
			return err
		}
		if correction, ok := strings.CutPrefix(name, "undone:"); ok {
			read.undone[correction] = at
			continue
		}
		read.state.HumanEditedAt[name] = at
	}
	return rows.Err()
}

// laterOf is the later of two instants.
func laterOf(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
