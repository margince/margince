// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Whether the model lanes are answering, per rung.
//
// The failure this exists for: a classifier that stopped answering and one that
// is working and merely cautious look identical from every other surface. Under
// the capture posture a thread stays held either way, so nobody notices an
// outage until somebody asks why a thread never opened — and by then the answer
// is a week of held mail.
//
// Read from ai_call, which already records every attempt with its tier, its
// latency and its sentinel. Nothing new is written for this: a health surface
// that needed its own bookkeeping would be a second account of what happened,
// free to disagree with the first.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// HealthWindow is how far back a rung's health is read.
//
// An hour, because the question is "is it answering NOW" and a day-long window
// would report a lane that died forty minutes ago as healthy on the strength of
// this morning. Long enough that a lane nobody happened to call in the last ten
// minutes does not read as broken.
const HealthWindow = time.Hour

// RungHealth is one model tier and what it has been doing.
type RungHealth struct {
	// Tier is the rung's name — local_small, cloud_large and the rest.
	Tier string
	// Calls and Failures count the window.
	Calls    int
	Failures int
	// LastOutcomeFailed is what actually decides health: whether the MOST
	// RECENT terminal attempt on this rung carried an error.
	//
	// A count cannot answer it. A lane that answered once at the top of the
	// hour and has failed every call since has more successes than zero, and a
	// rule reading "any success in the window" calls that healthy — which is
	// precisely the lane this endpoint exists to catch, reported as fine.
	LastOutcomeFailed bool
	// LastSentinel is the most recent error this rung reported, empty when it
	// reported none. It is the operator's first clue: a budget refusal and an
	// unreachable model are both "not answering" and want different fixes.
	LastSentinel string
	// LastCallAt is when this rung last answered anything at all.
	LastCallAt *time.Time
	// MedianLatencyMs is the window's middle latency, which distinguishes a
	// lane that is slow from one that is down.
	MedianLatencyMs int
}

// Healthy reports that this rung's most recent terminal attempt answered.
//
// The LATEST outcome, not a ratio over the window. A lane that answered at the
// top of the hour and has failed everything since is down now, and that is the
// only tense this question has — an operator reads this page to decide whether
// to act, and "it worked 59 minutes ago" is not a reason not to.
//
// A rung with no calls at all is not healthy and not unhealthy; it is silent,
// and Calls == 0 is what says so. Reporting silence as either would make an
// unused installation look broken or a dead one look fine.
//
// Decided here rather than left to each reader: two clients deciding what an
// all-failed rung means is how one surface calls an outage while the other does
// not.
func (r RungHealth) Healthy() bool { return r.Calls > 0 && !r.LastOutcomeFailed }

// RungHealthReport reads what every model tier has been doing for the last
// hour.
//
// Admin-gated through the automation-config write grant, the same door
// UsageReport uses and for the same reason: the closed RBAC object set carries
// no AI-runtime entry, and this is operational configuration rather than
// anybody's own data.
func (m *Meter) RungHealthReport(ctx context.Context) ([]RungHealth, error) {
	if err := auth.Require(ctx, "automation", principal.ActionUpdate); err != nil {
		return nil, err
	}
	since := m.now().Add(-HealthWindow)
	var out []RungHealth
	err := m.db.Tx(ctx, func(tx pgx.Tx) error {
		// TERMINAL attempts only, and cache hits excluded. A retried call
		// writes a row per attempt, and counting the failed ones a retry then
		// rescued would report a lane as failing while every caller of it got
		// an answer. A cache hit never reached the provider at all, so it says
		// nothing about whether the provider is answering — counting one as a
		// success is how a dead lane reports healthy.
		//
		// `metering_failed` is a SUCCESS here. It marks a call the model
		// answered where only the usage-meter write failed (callstore.go), and
		// callstats.go already treats it as served for exactly that reason.
		// Counting it as a failure would report a working lane as down.
		rows, err := tx.Query(ctx, `
			WITH terminal AS (
			  SELECT tier, occurred_at, latency_ms,
			         (error_sentinel IS NOT NULL
			          AND error_sentinel <> ''
			          AND error_sentinel <> 'metering_failed') AS failed,
			         coalesce(error_sentinel, '') AS sentinel
			    FROM ai_call
			   WHERE occurred_at >= $1
			     AND is_terminal
			     AND NOT cache_hit
			     AND tier <> ''
			)
			SELECT tier,
			       count(*)                                   AS calls,
			       count(*) FILTER (WHERE failed)             AS failures,
			       coalesce((array_agg(sentinel ORDER BY occurred_at DESC)
			                 FILTER (WHERE failed))[1], '')   AS last_sentinel,
			       max(occurred_at)                           AS last_call_at,
			       coalesce(percentile_disc(0.5) WITHIN GROUP (ORDER BY latency_ms), 0) AS median_latency,
			       -- The LATEST attempt's own outcome, which is what decides
			       -- health. array_agg over the same ordering the sentinel
			       -- uses, so both describe the same most-recent row.
			       coalesce((array_agg(failed ORDER BY occurred_at DESC))[1], false) AS last_failed
			  FROM terminal
			 GROUP BY tier
			 ORDER BY tier`, since)
		if err != nil {
			return fmt.Errorf("ai: reading rung health: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var h RungHealth
			var median int64
			if err := rows.Scan(&h.Tier, &h.Calls, &h.Failures,
				&h.LastSentinel, &h.LastCallAt, &median, &h.LastOutcomeFailed); err != nil {
				return fmt.Errorf("ai: reading rung health: %w", err)
			}
			h.MedianLatencyMs = int(median)
			out = append(out, h)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
