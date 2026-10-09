// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A lead count may reach no lead the asker's own list withholds. Lead is an
// identity table, so the scope clause renders empty and the list is not
// narrowed either. If the list is ever narrowed alone, the count leaks.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// teamLensManagerCtx is a team lead reading leads. The lens defaults to
// ScopeKindManagedTeams (themselves plus their team), so the query names no
// scope.
func (e *forecastEnv) teamLensManagerCtx(user ids.UUID, teams []ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:team-lead", UserID: user, TeamIDs: teams,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"lead": {Read: true}, "forecast": {Read: true},
				"installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeTeam,
		},
	})
}

func TestALeadCountNeverExceedsTheCallersOwnList(t *testing.T) {
	e := setupForecast(t)

	// The manager's own, and a colleague's who is on no team: the case a reader
	// expects a team lens to withhold.
	stranger := e.seedID(t, `INSERT INTO app_user (id, email, display_name)
		VALUES ($1, 'off-team@example.test', 'Off Team')`)
	for i := 0; i < 7; i++ {
		e.seedID(t, `INSERT INTO lead (id, full_name, status, source, captured_by, owner_id)
			VALUES ($1, 'Mine', 'new', 'inbound', 'human:x', $2)`, e.Rep1)
		e.seedID(t, `INSERT INTO lead (id, full_name, status, source, captured_by, owner_id)
			VALUES ($1, 'Theirs', 'new', 'inbound', 'human:x', $2)`, stranger)
	}

	manager := e.teamLensManagerCtx(e.Rep1, []ids.UUID{e.Team1})
	answer, err := e.askAnalytics(manager, t, analyticsquery.Query{
		Entity:   "leads-by-status",
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "leads"}},
	})
	if err != nil {
		t.Fatalf("a team lead's own leads-by-status was refused: %v", err)
	}
	if len(answer.Rows) != 1 {
		t.Fatalf("an ungrouped query answered %d rows: %+v", len(answer.Rows), answer.Rows)
	}
	var counted float64
	switch n := answer.Rows[0]["leads"].(type) {
	case float64:
		counted = n
	case int64:
		counted = float64(n)
	default:
		t.Fatalf("the count came back as %T (%v), which this suite cannot compare", n, n)
	}

	listed, _, err := contacts.NewStore(InstallationDB(e.Pool)).ListLeads(manager, contacts.ListLeadsInput{
		IncludeArchived: true, // the report counts archived leads, so the list must show them too
	})
	if err != nil {
		t.Fatalf("the same caller's own lead list was refused: %v", err)
	}

	if counted > float64(len(listed)) {
		t.Errorf("the count reached %v leads where this caller's own list shows %d — a population "+
			"figure is now disclosing leads the list withholds, which is exactly the leak the "+
			"identity-table reading of this report is mistaken for", counted, len(listed))
	}
}
