// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// /me answers which teams' weeks the caller may open, so a settings screen that
// needs only that never assembles the whole worklist to read it. The answer is
// the caller's own, decided from the identity /me describes even where the
// request context carries no signed-in user yet, as on the login responses.
func TestMeSaysWhichTeamWeeksTheCallerMayOpen(t *testing.T) {
	read := principal.ObjectGrant{Read: true}
	cases := []struct {
		name string
		seat principal.Permissions
		want string
	}{
		{"rep", principal.Permissions{
			RowScope: principal.RowScopeOwn,
			Objects:  map[string]principal.ObjectGrant{"deal": read},
		}, "none"},
		{"team lead", principal.Permissions{
			RowScope: principal.RowScopeTeam,
			Objects:  map[string]principal.ObjectGrant{"deal": read, "team_lead": read},
		}, "teams_led"},
		{"management", principal.Permissions{
			RowScope: principal.RowScopeAll,
			Objects:  map[string]principal.ObjectGrant{"deal": read, "team_oversight": read},
		}, "every_team"},
		{"lead without deal read", principal.Permissions{
			RowScope: principal.RowScopeTeam,
			Objects:  map[string]principal.ObjectGrant{"team_lead": read},
		}, "none"},
	}
	for _, tc := range cases {
		id := Identity{Email: "seat@example.com", SeatType: "full", Permissions: tc.seat}
		raw, err := json.Marshal(NewHandlers(&Service{}).meResponse(context.Background(), id))
		if err != nil {
			t.Fatalf("%s: marshalling /me: %v", tc.name, err)
		}
		var decoded struct {
			TeamWeek string `json:"team_week"`
		}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("%s: decoding /me: %v", tc.name, err)
		}
		if decoded.TeamWeek != tc.want {
			t.Errorf("/me told the %s team_week %q, want %q", tc.name, decoded.TeamWeek, tc.want)
		}
	}
}
