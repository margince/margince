// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// Why a snapshot was taken. Each is a real cause with different rules: only
// `daily` is arbitrated to one per local day, because the others are
// deliberately additional — a manager taking three calls in a day should get
// three frozen states.
const (
	TriggerDaily       = "daily"
	TriggerCall        = "call"
	TriggerPeriodClose = "period_close"
	TriggerRecheck     = "recheck"
)

// DefinitionVersion is which rules produced a set of readings.
//
// Stored on every snapshot because a definition change is a real cause of
// movement with its own bucket: a number that moved because the rules changed
// must not be reported as the business moving. Bump it when a reading's
// membership or arithmetic changes.
const DefinitionVersion = "forecast_v1"

// Column and payload keys shared by the insert and the audit payload. Named
// rather than repeated, because a key written in two places can come to be
// written two ways.
const (
	colScopeKind   = "scope_kind"
	colAmountMinor = "amount_minor"
	colCurrency    = "currency"
)

// NewSnapshot is one frozen set of readings and the rows behind it.
type NewSnapshot struct {
	Period       Period
	Scope        Scope
	Trigger      string
	BaseCurrency string
	Readings     Readings
	// TakenAt is the instant the readings describe. Passed rather than read
	// from a clock here so a snapshot and the readings inside it cannot be
	// stamped microseconds apart, which is what makes a daily arbiter fire on
	// the wrong local day at a midnight boundary.
	TakenAt time.Time
	// CallID ties a call-triggered snapshot to the call that caused it.
	CallID *ids.UUID
}

// TakeSnapshot freezes one set of readings, with the rows they were summed
// from, in ONE transaction.
//
// The contributions are not a convenience: they are what makes a headline
// answerable. "The forecast moved and here is which deals moved it" is a
// question about two recorded states, and a snapshot without its rows can only
// say that a number changed.
func (s *Store) TakeSnapshot(ctx context.Context, tx pgx.Tx, in NewSnapshot) (ids.UUID, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionCreate); err != nil {
		return ids.Nil, err
	}
	if err := checkSnapshot(in); err != nil {
		return ids.Nil, err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return ids.Nil, err
	}

	var id ids.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO forecast_snapshot
		    (period_start, period_end, scope_kind, scope_id, taken_at, local_day,
		     trigger, definition_version, base_currency,
		     won_minor, evidence_minor, best_case_minor, open_minor, weighted_minor,
		     eligible_count, priced_count, confirmed_date_count, fx_missing_count,
		     call_id, captured_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
		        $15, $16, $17, $18, $19, $20)
		RETURNING id`,
		in.Period.StartDate, in.Period.EndDate, in.Scope.Kind, in.Scope.ID,
		in.TakenAt,
		// The local day the snapshot was taken on, in the installation's own
		// zone — the same conversion every reading goes through. Derived from
		// TakenAt rather than read from a clock, so the row the daily arbiter
		// sees is the row this snapshot actually describes.
		in.Period.LocalDay(in.TakenAt),
		in.Trigger, DefinitionVersion, in.BaseCurrency,
		in.Readings.WonMinor, in.Readings.EvidenceMinor, in.Readings.BestCaseMinor,
		in.Readings.OpenMinor, in.Readings.WeightedMinor,
		in.Readings.EligibleCount, in.Readings.PricedCount,
		in.Readings.ConfirmedDateCount, in.Readings.FxMissingCount,
		in.CallID, capturedBy).Scan(&id); err != nil {
		return ids.Nil, fmt.Errorf("forecasting: writing the snapshot: %w", err)
	}

	if err := s.writeContributions(ctx, tx, id, in.Readings.Contributions, capturedBy); err != nil {
		return ids.Nil, err
	}

	auditID, err := storekit.AuditEvent(ctx, tx, "create", "forecast_snapshot", id,
		map[string]any{
			"period_start": in.Period.StartDate.Format(time.DateOnly),
			"period_end":   in.Period.EndDate.Format(time.DateOnly),
			colScopeKind:   in.Scope.Kind,
			"trigger":      in.Trigger,
			// The counts, not the money. An audit row is read by people
			// auditing what the system did, and how many deals a run
			// considered is the fact that says whether it ran completely.
			"eligible_count": in.Readings.EligibleCount,
			"priced_count":   in.Readings.PricedCount,
		})
	if err != nil {
		return ids.Nil, err
	}
	// The event a movement consumer waits on: the difference between two
	// snapshots is not answerable until the second exists. Counts ride along
	// and money does not — how many deals a run considered is what says whether
	// it ran completely, and totals on a bus are a second place to correct when
	// a definition changes.
	if err := storekit.EmitEvent(ctx, tx, auditID, actorForEvent(ctx),
		crmcontracts.PublicEventForecastSnapshotCreated{
			SnapshotId:    openapi_types.UUID(id),
			PeriodStart:   openapi_types.Date{Time: in.Period.StartDate},
			PeriodEnd:     openapi_types.Date{Time: in.Period.EndDate},
			Trigger:       crmcontracts.PublicEventForecastSnapshotCreatedTrigger(in.Trigger),
			EligibleCount: in.Readings.EligibleCount,
		}); err != nil {
		return ids.Nil, err
	}
	return id, nil
}

// actorForEvent is who the snapshot is attributed to.
//
// A nightly run has no human behind it, and the entity a stream is keyed by
// cannot be absent — so the system's own runs carry the nil id rather than
// borrowing a person who did not ask for them.
func actorForEvent(ctx context.Context) ids.UUID {
	if actor, ok := principal.Actor(ctx); ok {
		return actor.UserID
	}
	return ids.Nil
}

// writeContributions inserts the per-deal rows behind one snapshot.
//
// One statement rather than a row at a time: a workspace's open pipeline is
// thousands of deals and a round trip each would make the nightly run's cost
// the network's rather than the database's.
func (s *Store) writeContributions(
	ctx context.Context, tx pgx.Tx, snapshotID ids.UUID, rows []Contribution, capturedBy string,
) error {
	if len(rows) == 0 {
		// A period with no deals is a real answer. The snapshot stands with no
		// contributions, and its headlines are honest zeros.
		return nil
	}
	_, err := tx.CopyFrom(ctx,
		pgx.Identifier{"forecast_contribution"},
		[]string{
			"snapshot_id", "deal_id", "owner_id", colAmountMinor, colCurrency,
			"base_minor", "fx_rate", "fx_date", "effective_close_date",
			"close_provisional", "category", "stage_probability", "weighted_minor",
			"in_won", "in_evidence", "in_best_case", "in_open", "exclusion_reason",
			"audit_id", "approval_id", "captured_by",
		},
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			row := rows[i]
			dealID, err := ids.Parse(row.DealID)
			if err != nil {
				return nil, fmt.Errorf("forecasting: contribution %d names a deal id it cannot parse: %w", i, err)
			}
			// A weighted amount exists exactly when there is a base amount to
			// weight it from — the pairing the table's own CHECK holds.
			var weighted *int64
			if row.BaseMinor != nil {
				w := row.WeightedMinor
				weighted = &w
			}
			return []any{
				snapshotID, dealID, nullableID(row.Owner), row.AmountMinor,
				nullIfEmpty(row.Currency), row.BaseMinor, nil, nil,
				row.EffectiveClose, row.CloseProvisional, nullIfEmpty(row.Category),
				nullableInt(row.StageProbability), weighted,
				row.InWon, row.InEvidence, row.InBestCase, row.InOpen,
				nullIfEmpty(row.ExclusionReason),
				row.AuditID, row.ApprovalID, capturedBy,
			}, nil
		}))
	if err != nil {
		return fmt.Errorf("forecasting: writing the snapshot's contributions: %w", err)
	}
	return nil
}

// checkSnapshot holds what the table's CHECKs hold, so a caller reads a named
// field back rather than a constraint violation.
func checkSnapshot(in NewSnapshot) error {
	if !in.Period.consistent() {
		return &values.ParseError{
			Field: "period", Code: codeInvalid,
			Message: "the period's day bounds and instant bounds name different windows",
		}
	}
	if err := checkScope(in.Scope); err != nil {
		return err
	}
	switch in.Trigger {
	case TriggerDaily, TriggerCall, TriggerPeriodClose, TriggerRecheck:
	default:
		return &values.ParseError{
			Field: "trigger", Code: codeInvalid,
			Message: "a snapshot is taken daily, on a call, at period close, or on a recheck",
		}
	}
	if !values.ValidCurrency(in.BaseCurrency) {
		return &values.ParseError{
			Field: "base_currency", Code: codeInvalid,
			Message: "a snapshot names the currency its money is counted in",
		}
	}
	return nil
}

// nullableID keeps an absent owner out of the column as NULL. A deal with no
// owner is a real state, and the empty string is not a user id.
func nullableID(raw string) *ids.UUID {
	if raw == "" {
		return nil
	}
	parsed, err := ids.Parse(raw)
	if err != nil {
		return nil
	}
	return &parsed
}

// nullableInt keeps a zero probability distinguishable from an absent stage.
// Zero is a real probability — a stage a deal never wins from — so only a
// negative marks "no stage", which contribute() never produces.
func nullableInt(v int) *int {
	if v < 0 {
		return nil
	}
	return &v
}

// SnapshotSide loads one snapshot's definition version and its per-deal rows.
//
// Gated on read: a snapshot is a record of what the workspace expected, and
// reading one is reading the forecast.
func (s *Store) SnapshotSide(ctx context.Context, tx pgx.Tx, id ids.UUID) (snapshotSide, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionRead); err != nil {
		return snapshotSide{}, err
	}
	var out snapshotSide
	err := tx.QueryRow(ctx,
		`SELECT definition_version FROM forecast_snapshot WHERE id = $1`, id).
		Scan(&out.DefinitionVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return snapshotSide{}, apperrors.ErrNotFound
	}
	if err != nil {
		return snapshotSide{}, fmt.Errorf("forecasting: reading the snapshot: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT deal_id, owner_id, amount_minor, currency, base_minor,
		       effective_close_date, close_provisional, category, stage_probability,
		       weighted_minor, in_won, in_evidence, in_best_case, in_open,
		       exclusion_reason, audit_id, approval_id
		FROM forecast_contribution
		WHERE snapshot_id = $1
		ORDER BY deal_id`, id)
	if err != nil {
		return snapshotSide{}, fmt.Errorf("forecasting: reading the snapshot's contributions: %w", err)
	}
	defer rows.Close()

	out.Contributions, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Contribution, error) {
		var c Contribution
		var dealID ids.UUID
		var owner *ids.UUID
		var currency, category, exclusion *string
		var probability *int
		var weighted *int64
		err := row.Scan(&dealID, &owner, &c.AmountMinor, &currency, &c.BaseMinor,
			&c.EffectiveClose, &c.CloseProvisional, &category, &probability,
			&weighted, &c.InWon, &c.InEvidence, &c.InBestCase, &c.InOpen,
			&exclusion, &c.AuditID, &c.ApprovalID)
		c.DealID = dealID.String()
		if owner != nil {
			c.Owner = owner.String()
		}
		if currency != nil {
			c.Currency = *currency
		}
		if category != nil {
			c.Category = *category
		}
		if exclusion != nil {
			c.ExclusionReason = *exclusion
		}
		if probability != nil {
			c.StageProbability = *probability
		}
		if weighted != nil {
			c.WeightedMinor = *weighted
		}
		return c, err
	})
	if err != nil {
		return snapshotSide{}, fmt.Errorf("forecasting: collecting the snapshot's contributions: %w", err)
	}
	return out, nil
}

// Movement classifies the difference between two snapshots.
//
// Both sides are read in ONE transaction. Read separately, a snapshot written
// between the two reads would be compared against a state that no longer
// matches the one the opening total came from.
func (s *Store) Movement(ctx context.Context, reading Reading, from, to ids.UUID) (Movement, error) {
	var out Movement
	err := s.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		opening, err := s.SnapshotSide(ctx, tx, from)
		if err != nil {
			return err
		}
		closing, err := s.SnapshotSide(ctx, tx, to)
		if err != nil {
			return err
		}
		out = Classify(reading, opening, closing)
		return nil
	})
	if err != nil {
		return Movement{}, err
	}
	return out, nil
}
