// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// mcpCallsPerMinute is the build's per-minute tool-call pacing, as the
// allowances settings report it.
const mcpCallsPerMinute = 240

// An Admin's passport and a User's, authenticated as the API does and judged
// by the allowances the installation reads. The Admin's tool calls are never
// paced per minute; the User's are, at the build's ceiling.
//
// The edge counts in fixed one-minute windows. The clock stands still, so every
// call falls in one window however long the calls take.
func TestAnAdminsAgentIsUnpacedAndAUsersIsPacedOnMCP(t *testing.T) {
	at := time.Now()
	d := roleBoundDoors(t, "mcp-pacing", compose.WithMCPClockForTest(func() time.Time { return at }))
	d.E.DescribeCompany(t)
	const email, password = "rep@mcp-pacing.test", "mcp-pacing-password"
	rep := createdID(t, d.E, "/v1/users", AnyMap{"email": email, "display_name": "Robin Rep", "role": "rep"})
	apptest.SetPasswordByLink(t, d.E, rep, password)
	repBearer := apptest.PassportBearer(t, d.E.AsSecondSeat(t, email, password), "rep agent", "read")
	list := AnyMap{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}
	pastTheCeiling := mcpCallsPerMinute + 1

	for i := 1; i <= pastTheCeiling; i++ {
		if status := d.E.Call(t, "POST", "/mcp", list, d.MCP.Bearer(), nil); status != http.StatusOK {
			t.Fatalf("the Admin's call %d → %d, want 200 with no per-minute pacing", i, status)
		}
	}

	for i := 1; i < pastTheCeiling; i++ {
		if status := d.E.Call(t, "POST", "/mcp", list, repBearer, nil); status != http.StatusOK {
			t.Fatalf("the User's call %d → %d, want 200 up to the ceiling of %d", i, status, mcpCallsPerMinute)
		}
	}
	var refusal limitRefusal
	status := d.E.Call(t, "POST", "/mcp", list, repBearer, &refusal)
	if status != http.StatusTooManyRequests || refusal.Details.Limit.Kind != "pacing" ||
		refusal.Details.Limit.Ceiling != mcpCallsPerMinute {
		t.Fatalf("the User's call %d → %d %+v, want 429 naming the pacing of %d", pastTheCeiling, status, refusal, mcpCallsPerMinute)
	}
}
