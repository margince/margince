// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The trigger fixture two workflow suites share: the event a seeded starter
// reacts to, and the enrolment that makes it react.
//
// Here rather than in either suite because the two now sit behind DIFFERENT
// build tags. The dispatch benchmark is `integration && bench` — it measures a
// budget and has no place in the merge gate — while the replay suite is
// `integration` and runs on every push. A helper living in the benchmark is
// invisible to the plain integration build, which is a compile failure in the
// lane that matters rather than a missing test.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/platform/database"
)

// leadCreatedEventType names the trigger both suites fire — the one seeded
// starter (route_lead) that reacts to it unconditionally, so a sampled lead
// produces exactly one dispatch to measure and a replayed one exactly one run
// to reconstruct.
const leadCreatedEventType = "lead.created"

// seedAllStarterAutomations enrols the seeded starter templates the way a fresh
// workspace's bootstrap does (SeedStarterAutomationsTx) — the seeded dataset,
// not a hand-picked single instance.
func seedAllStarterAutomations(t *testing.T, e *Env) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return automation.SeedStarterAutomationsTx(context.Background(), tx)
	})
	if err != nil {
		t.Fatalf("seeding starter automations: %v", err)
	}
}
