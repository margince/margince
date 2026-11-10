// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// mcpCallsPerMinute is the build's per-minute tool-call pacing, as the
// allowances settings report it.
const mcpCallsPerMinute = 240

// An Admin's passport and a User's, authenticated as the API does and judged
// by the allowances the installation reads. The Admin's tool calls are never
// paced per minute; the User's are, at the build's ceiling.
//
// The edge counts in fixed one-minute windows on the real clock, so a run may
// cross one boundary. Two windows of calls then always reach a User's refusal,
// and the first refusal can never come before the ceiling.
func TestAnAdminsAgentIsUnpacedAndAUsersIsPacedOnMCP(t *testing.T) {
	d := roleBoundDoors(t, "mcp-pacing")
	d.E.DescribeCompany(t)
	const email, password = "rep@mcp-pacing.test", "mcp-pacing-password"
	rep := createdID(t, d.E, "/v1/users", AnyMap{"email": email, "display_name": "Robin Rep", "role": "rep"})
	apptest.SetPasswordByLink(t, d.E, rep, password)
	repBearer := apptest.PassportBearer(t, d.E.AsSecondSeat(t, email, password), "rep agent", "read")
	list := AnyMap{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}
	twoWindows := 2*mcpCallsPerMinute + 1

	for i := 1; i <= twoWindows; i++ {
		if status := d.E.Call(t, "POST", "/mcp", list, d.MCP.Bearer(), nil); status != http.StatusOK {
			t.Fatalf("the Admin's call %d → %d, want 200 with no per-minute pacing", i, status)
		}
	}

	for i := 1; i <= twoWindows; i++ {
		var refusal limitRefusal
		status := d.E.Call(t, "POST", "/mcp", list, repBearer, &refusal)
		if status == http.StatusOK {
			continue
		}
		if status != http.StatusTooManyRequests || refusal.Details.Limit.Kind != "pacing" ||
			refusal.Details.Limit.Ceiling != mcpCallsPerMinute {
			t.Fatalf("the User's call %d → %d %+v, want 429 naming the pacing of %d", i, status, refusal, mcpCallsPerMinute)
		}
		if i <= mcpCallsPerMinute {
			t.Fatalf("the User's call %d was paced, before the ceiling of %d", i, mcpCallsPerMinute)
		}
		return
	}
	t.Fatalf("%d of the User's calls in at most two windows met no pacing", twoWindows)
}
