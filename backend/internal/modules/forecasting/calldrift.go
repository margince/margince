// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

// Whether a manager's call still describes the pipeline it was drawn from.
//
// A call keeps a snapshot of the reading it was made against, with what that
// reading stood on. Comparing it with today runs the waterfall's per-deal
// comparison. A change is material when it moves a deal's money in a reading,
// moves the deal across the period, or changes its qualification. Age is not
// a change.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Limitation names why a call cannot be compared with today's reading. A
// delta across any of these would report the change of basis as deals that
// moved.
type Limitation string

// The limitations a comparison can meet.
const (
	LimitationPopulation      Limitation = "population_changed"
	LimitationCurrency        Limitation = "currency_changed"
	LimitationDefinition      Limitation = "definition_changed"
	LimitationQualifiedPolicy Limitation = "qualified_policy_changed"
	// LimitationPeriod is a call whose window the financial calendar no
	// longer has, such as after the year was moved to start in another month.
	LimitationPeriod Limitation = "period_changed"
	// LimitationWithheld is a baseline the reader could not see whole, such as
	// one naming a deal that was erased since.
	LimitationWithheld Limitation = "baseline_withheld"
)

// CallDrift is what moved between a call's baseline and today.
type CallDrift struct {
	// Limitation is set when the two cannot be compared. Deals and Net are
	// then empty.
	Limitation Limitation
	// Deals has one row per deal that moved materially, sorted by deal. Its
	// Bucket and AmountMinor are the open reading's; Causes lists every change.
	Deals []DealDelta
	// Net is each reading's change from the baseline to today.
	Net map[Reading]int64
	// QualificationChanges counts deals whose qualification changed.
	QualificationChanges int
}

// Material reports whether anything moved that the call depended on.
func (d CallDrift) Material() bool { return d.Limitation == "" && len(d.Deals) > 0 }

// TodaySide is today's reading in the shape of a snapshot, so a baseline can
// be compared with it as with any other capture.
func TodaySide(today Baseline, at time.Time) snapshotSide {
	return snapshotSide{
		DefinitionVersion:     DefinitionVersion,
		Contributions:         today.Readings.Contributions,
		BaseCurrency:          today.BaseCurrency,
		TakenAt:               at,
		PopulationFingerprint: today.PopulationFingerprint,
		InputFingerprint:      InputFingerprint(today.Readings.Contributions),
		QualifiedPolicy:       today.QualifiedPolicy,
	}
}

// CompareWithBaseline answers what moved since a call's baseline.
func CompareWithBaseline(baseline, today snapshotSide) CallDrift {
	if limitation := limitationOf(baseline, today); limitation != "" {
		return CallDrift{Limitation: limitation}
	}
	out := CallDrift{Net: map[Reading]int64{}}
	for _, reading := range everyReading {
		out.Net[reading] = totalOf(reading, today.Contributions) - totalOf(reading, baseline.Contributions)
	}
	if baseline.InputFingerprint == today.InputFingerprint {
		return out
	}
	before := indexByDeal(baseline.Contributions)
	after := indexByDeal(today.Contributions)
	for dealID, now := range after {
		was, existed := before[dealID]
		if !materiallyChanged(was, existed, now) {
			continue
		}
		delta := contributionOf(ReadingOpen, now) - contributionOf(ReadingOpen, was)
		bucket := classifyOne(ReadingOpen, was, existed, now)
		out.Deals = append(out.Deals, dealDelta(dealID, bucket, delta, ReadingOpen, was, existed, now))
		if existed && was.Qualification != now.Qualification {
			out.QualificationChanges++
		}
	}
	for dealID, was := range before {
		if _, still := after[dealID]; still || !touchesForecast(was) {
			continue
		}
		out.Deals = append(out.Deals, DealDelta{
			DealID: dealID, Bucket: BucketArchived, AmountMinor: -contributionOf(ReadingOpen, was),
			FromMinor: new(contributionOf(ReadingOpen, was)), ToMinor: new(int64(0)),
			Causes: []string{BucketArchived},
		})
	}
	slices.SortFunc(out.Deals, func(a, b DealDelta) int { return strings.Compare(a.DealID, b.DealID) })
	return out
}

// limitationOf answers the first basis the two sides do not share.
func limitationOf(baseline, today snapshotSide) Limitation {
	switch {
	case baseline.Withheld || today.Withheld:
		return LimitationWithheld
	case baseline.PopulationFingerprint != today.PopulationFingerprint:
		return LimitationPopulation
	case baseline.BaseCurrency != today.BaseCurrency:
		return LimitationCurrency
	case baseline.DefinitionVersion != today.DefinitionVersion:
		return LimitationDefinition
	case baseline.QualifiedPolicy != today.QualifiedPolicy:
		return LimitationQualifiedPolicy
	default:
		return ""
	}
}

// materiallyChanged reports a deal whose money moved in any compared reading,
// that crossed the period, or whose qualification changed. A deal that did not
// exist before is compared against contributing nothing.
func materiallyChanged(was Contribution, existed bool, now Contribution) bool {
	if existed && was.Qualification != now.Qualification {
		return true
	}
	for _, reading := range everyReading {
		if contributionOf(reading, was) != contributionOf(reading, now) || inReading(reading, was) != inReading(reading, now) {
			return true
		}
	}
	return was.InWon != now.InWon
}

// touchesForecast reports a contribution that is in some compared reading or
// was won in the period, so its leaving moves the forecast.
func touchesForecast(c Contribution) bool {
	if c.InWon {
		return true
	}
	for _, reading := range everyReading {
		if inReading(reading, c) {
			return true
		}
	}
	return false
}

// CallBaselineTx loads the snapshot a call was made against. found is false
// for a call recorded before calls kept one, whose history is unavailable.
func (s *Store) CallBaselineTx(ctx context.Context, tx pgx.Tx, callID ids.UUID) (snapshotSide, bool, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionRead); err != nil {
		return snapshotSide{}, false, err
	}
	var id ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT id FROM forecast_snapshot
		 WHERE call_id = $1 AND trigger = $2 AND input_fingerprint IS NOT NULL
		 ORDER BY taken_at DESC, id DESC
		 LIMIT 1`, callID, TriggerCall).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return snapshotSide{}, false, nil
	}
	if err != nil {
		return snapshotSide{}, false, fmt.Errorf("forecasting: finding the call's baseline: %w", err)
	}
	side, err := s.SnapshotSide(ctx, tx, id)
	return side, err == nil, err
}
