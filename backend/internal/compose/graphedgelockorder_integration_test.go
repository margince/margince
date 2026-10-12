// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Concurrent writers of the two edge projections must not deadlock. Replicas of
// the cg:graph-edge group fold activity and contact events at the same time,
// and the daily rebuild runs on every worker start, so the three writers meet
// on the same rows. A crowd rather than a pair: the interleaving that deadlocks
// is rare per attempt, so one pair of transactions passes by luck.

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEdgeWritersDoNotDeadlockWithEachOtherOrTheRebuild(t *testing.T) {
	v := edgeEnv{integration.Setup(t)}
	now := time.Now().UTC()
	gen := search.NewGraphEdgeGen(search.NewStore(v.e.DB()))
	ctx := context.Background()

	var activityIDs []ids.UUID
	var contacts []ids.ContactID
	for i := range 12 {
		c := v.contact(t, fmt.Sprintf("Crowd %d", i))
		contacts = append(contacts, c)
		a := v.interaction(t, v.e.Rep1, c, now.AddDate(0, 0, -i), "inbound", "from")
		if i > 0 {
			if err := database.WithWorkspaceTx(v.e.Admin(), v.e.Pool, func(tx pgx.Tx) error {
				if _, err := tx.Exec(ctx, `
					INSERT INTO activity_participant (activity_id, user_id, role) VALUES ($1, $2, 'to')`,
					a, v.e.Rep2); err != nil {
					return err
				}
				_, err := tx.Exec(ctx, `
					INSERT INTO activity_participant (activity_id, contact_id, role) VALUES ($1, $2, 'cc')`,
					a, contacts[i/2])
				return err
			}); err != nil {
				t.Fatalf("seeding a shared activity: %v", err)
			}
		}
		activityIDs = append(activityIDs, a)
	}
	v.recompute(t, activityIDs...)

	activityEvents := func() []kevents.Envelope {
		var envs []kevents.Envelope
		for _, id := range activityIDs {
			envs = append(envs, envelopeFor(v.e.WS, "activity.updated", "activity", id))
		}
		return envs
	}
	writers := []func(i int) error{
		func(int) error { return gen.HandleBatch(ctx, activityEvents()) },
		func(i int) error {
			return gen.HandleEvent(ctx, envelopeFor(v.e.WS, "contact.updated", "contact", contacts[i%len(contacts)].UUID))
		},
		func(int) error {
			return gen.HandleBatch(ctx, append(activityEvents(), envelopeFor(v.e.WS, "contact.updated", "contact", contacts[0].UUID)))
		},
		func(int) error {
			return database.WithWorkspaceTx(v.e.Admin(), v.e.Pool, func(tx pgx.Tx) error {
				return search.RebuildEdges(v.e.Admin(), tx)
			})
		},
	}

	const rounds = 15
	errs := make(chan error, len(writers)*3*rounds)
	var wg sync.WaitGroup
	for w := 0; w < len(writers)*3; w++ {
		wg.Add(1)
		go func(write func(int) error) {
			defer wg.Done()
			for i := range rounds {
				if err := write(i); err != nil {
					errs <- err
				}
			}
		}(writers[w%len(writers)])
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("a projection writer failed under concurrency: %v", err)
	}
}
