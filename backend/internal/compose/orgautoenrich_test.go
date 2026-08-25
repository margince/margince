// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/events"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// orgEvent builds the envelope the trigger reads, occurring now.
func orgEvent(eventType string, id ids.UUID) events.Envelope {
	return events.Envelope{
		Type:       eventType,
		OccurredAt: time.Now().UTC(),
		Entity:     events.EntityRef{Type: "organization", ID: id},
	}
}

// The trigger must ignore what it cannot act on WITHOUT touching the pool.
// The nil pool is the assertion: anything that reached the database here
// would panic, so a pass proves the refusal happened at the envelope.
func TestOrgAutoEnrichTriggerIgnoresWhatItCannotActOn(t *testing.T) {
	g := &OrgAutoEnrichTrigger{}
	ctx := context.Background()

	stale := orgEvent("organization.created", ids.NewV7())
	stale.OccurredAt = stale.OccurredAt.Add(-2 * orgAutoEnrichFreshWindow)

	for name, env := range map[string]events.Envelope{
		"an archive can only make organizations less due":                orgEvent("organization.archived", ids.NewV7()),
		"a verb outside the set that can make an organization newly due": orgEvent("organization.geocoded", ids.NewV7()),
		"an unrelated entity type":                                       {Type: "organization.created", OccurredAt: time.Now().UTC(), Entity: events.EntityRef{Type: "person", ID: ids.NewV7()}},
		// A stale event is a replayed backlog (a new consumer group starts at
		// stream position 0) or a delivery held for hours; queueing one job
		// per historical event would flood the queue on the first boot after
		// deployment, and the daily sweep owns everything that old.
		"an event older than the freshness window": stale,
	} {
		if err := g.HandleEvent(ctx, env); err != nil {
			t.Errorf("%s: HandleEvent returned %v, want a silent skip", name, err)
		}
	}
}
