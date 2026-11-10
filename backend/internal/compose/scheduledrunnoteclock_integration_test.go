// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A note a scheduled agent run writes moves no last-activity clock, so the
// overnight sweep's risk note leaves the quiet deal it warns about quiet. A
// note a user writes through their own assistant, with a passport and no
// run, moves every clock like one typed in the app. Both go through the
// activities provider, the seam every tool's create reaches.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// noteThroughTheTools creates a note on the contact and the deal through the
// activities provider, as log_activity does, and answers its origin. Filing
// under a project is refused unattended, so the project clock is not read.
func (f taskClockFixture) noteThroughTheTools(ctx context.Context, t *testing.T, at time.Time) string {
	t.Helper()
	links := []map[string]any{
		{"entity_type": "contact", "entity_id": f.contact.String()},
		{"entity_type": "deal", "entity_id": f.deal.String()},
	}
	created, err := activities.NewProvider(f.e.DB()).Create(ctx, datasource.CreateInput{
		EntityType: datasource.EntityActivity, Source: "manual",
		Fields: map[string]any{
			"kind": "note", "subject": "Risk assessment", "body": "No reply in weeks.",
			"occurred_at": at.Format(time.RFC3339), "links": links,
		},
	})
	if err != nil {
		t.Fatalf("creating the note through the provider: %v", err)
	}
	return f.e.WsScalar(t, `SELECT origin FROM activity WHERE id = $1`, created.ID)
}

func TestANoteAScheduledRunWritesLeavesEveryClockQuiet(t *testing.T) {
	f := newTaskClockFixture(t)
	assistant := f.e.AgentFor(t, f.e.AdminUser, nil, integration.AdminPerms)
	run := principal.WithAgentRunID(assistant, ids.NewV7())

	if origin := f.noteThroughTheTools(run, t, f.now.Add(-time.Minute)); origin != activities.OriginAgent {
		t.Fatalf("a scheduled run's note was stored with origin %q, want %q", origin, activities.OriginAgent)
	}
	f.assertClocks(t, f.mailed, "a scheduled run's note is the product's comment, not contact")
}

func TestANoteAUserWritesThroughTheirAssistantMovesEveryClock(t *testing.T) {
	f := newTaskClockFixture(t)
	assistant := f.e.AgentFor(t, f.e.AdminUser, nil, integration.AdminPerms)
	at := f.now.Add(-time.Minute)

	if origin := f.noteThroughTheTools(assistant, t, at); origin != activities.OriginHuman {
		t.Fatalf("a note from a chat assistant was stored with origin %q, want %q", origin, activities.OriginHuman)
	}
	f.assertClocks(t, at, "a user's note counts, whichever door it came through")
}
