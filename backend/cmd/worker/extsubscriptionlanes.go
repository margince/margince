// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/events"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
)

// startExtensionSubscriptionLanes starts one consumer per composed unit
// subscription — the tier's half of the bus, and this role's alone: every role
// composes the same units, and the worker is the role that consumes.
//
// It is started from run(), AFTER the job runner, rather than with the lanes
// above. A delivery reaches the installation through the per-call Runtime, and
// this role's unconditional BindExtensionRuntime is the job runner's; started
// with its siblings, a retained entry redelivered in the window before that
// bind would fail on the wiring and then wait out the subscriber's whole
// reclaim interval before anything tried again. It still runs on the LANES'
// context, so it ends when they do.
//
// A listener whose group cannot be built is LOGGED and skipped rather than
// failing the boot, because the boot already refused the only way that can
// happen: RegisterExtensions preflights every declared type against the
// catalog. Reaching this branch means those two disagree, which is a defect in
// this binary rather than in the deployment — and taking down the worker's
// other lanes over one unit's listener would turn a unit-sized fault into an
// installation-sized one.
func startExtensionSubscriptionLanes(ctx context.Context, pool *pgxpool.Pool, rdb *redis.Client, background *sync.WaitGroup, logger *slog.Logger, stdout io.Writer) {
	for _, lane := range extensionSubscriptionLanes(pool, logger, stdout) {
		background.Go(func() { runGroupSubscriber(ctx, rdb, lane.group, lane.handler, logger, 0) })
	}
}

// subscriptionLane is one composed listener resolved to what running it takes:
// the group to consume, and the handler to consume it with.
type subscriptionLane struct {
	group   kevents.Group
	handler events.Handler
}

// extensionSubscriptionLanes resolves every composed listener, announcing the
// ones it can run and skipping the ones it cannot.
//
// It is split from the starter above so this decision — which lanes exist, and
// what happens to a listener that cannot be resolved — can be exercised without
// a bus to consume from. The skip is the part worth pinning: one unresolvable
// listener must not cost the others their lane.
func extensionSubscriptionLanes(pool *pgxpool.Pool, logger *slog.Logger, stdout io.Writer) []subscriptionLane {
	var lanes []subscriptionLane
	for _, sub := range compose.ComposedSubscriptions() {
		group, err := sub.Group()
		if err != nil {
			logger.Error("worker: an extension subscription has no consumer group, so it would receive nothing",
				"unit", string(sub.Unit), "subscription", sub.Sub.Name, "error", err)
			continue
		}
		_, _ = fmt.Fprintf(stdout, "worker delivering %s to %s/%s (%s)\n",
			strings.Join(sub.Sub.Events, ", "), sub.Unit, sub.Sub.Name, group.Name)
		lanes = append(lanes, subscriptionLane{group: group, handler: sub.Handler(pool, logger)})
	}
	return lanes
}
