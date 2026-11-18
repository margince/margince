// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// An Admin's passport and a User's, authenticated as the API does and judged
// by the allowances the installation reads. The Admin's tool calls are never
// paced per minute; the User's are, at the ceiling the settings report.
//
// The edge counts in fixed one-minute windows on the real clock, so a run may
// cross one boundary. Two windows of calls then always reach a User's refusal,
// and the first refusal can never come before the ceiling. Each call is a
// ping: pacing counts every authenticated call alike, and a ping costs the edge
// and the passport lookup only, so the calls fit in two windows on a slow host.
func TestAnAdminsAgentIsUnpacedAndAUsersIsPacedOnMCP(t *testing.T) {
	d := roleBoundDoors(t, "mcp-pacing")
	d.E.DescribeCompany(t)
	var settings struct {
		Allowances struct {
			PerMinute int `json:"mcp_calls_per_minute"`
		} `json:"agent_allowances"`
	}
	if status := d.E.Call(t, "GET", "/v1/installation/settings", nil, nil, &settings); status != http.StatusOK ||
		settings.Allowances.PerMinute <= 0 {
		t.Fatalf("reading the pacing ceiling → %d %+v, want 200 naming it", status, settings)
	}
	ceiling := settings.Allowances.PerMinute
	const email, password = "rep@mcp-pacing.test", "mcp-pacing-password"
	rep := createdID(t, d.E, "/v1/users", AnyMap{"email": email, "display_name": "Robin Rep", "role": "rep"})
	apptest.SetPasswordByLink(t, d.E, rep, password)
	repBearer := apptest.PassportBearer(t, d.E.AsSecondSeat(t, email, password), "rep agent", "read")
	ping := AnyMap{"jsonrpc": "2.0", "id": 1, "method": "ping"}
	twoWindows := 2*ceiling + 1
	// Past two windows the counts no longer prove either claim, so a host that
	// slow fails on its time rather than on a missing refusal.
	inTwoWindows := func(who string, began time.Time) {
		if took := time.Since(began); took >= 2*time.Minute {
			t.Fatalf("the %s's %d calls took %s, longer than the two pacing windows they must fit in", who, twoWindows, took)
		}
	}

	began := time.Now()
	for i := 1; i <= twoWindows; i++ {
		if status := d.E.Call(t, "POST", "/mcp", ping, d.MCP.Bearer(), nil); status != http.StatusOK {
			t.Fatalf("the Admin's call %d → %d, want 200 with no per-minute pacing", i, status)
		}
	}
	inTwoWindows("Admin", began)

	began = time.Now()
	for i := 1; i <= twoWindows; i++ {
		var refusal limitRefusal
		status := d.E.Call(t, "POST", "/mcp", ping, repBearer, &refusal)
		if status == http.StatusOK {
			continue
		}
		if status != http.StatusTooManyRequests || refusal.Details.Limit.Kind != "pacing" ||
			refusal.Details.Limit.Ceiling != ceiling {
			t.Fatalf("the User's call %d → %d %+v, want 429 naming the pacing of %d", i, status, refusal, ceiling)
		}
		if i <= ceiling {
			t.Fatalf("the User's call %d was paced, before the ceiling of %d", i, ceiling)
		}
		inTwoWindows("User", began)
		return
	}
	inTwoWindows("User", began)
	t.Fatalf("%d of the User's calls in at most two windows met no pacing", twoWindows)
}
