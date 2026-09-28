// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a bulk change costs an agent: every record it changed, on the write
// counter, and a refusal rather than an inbox question when the counter cannot
// pay for the change.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func writesCharged(t *testing.T, e *apptest.AppEnv, meter *agentvolume.Meter, agent bulkAgent) int {
	t.Helper()
	return meter.Read(asPassport(t, e, agent.id), agentvolume.Writes).Observed
}

func TestABulkChangeChargesEveryRecordItChangedAgainstTheAgentsWrites(t *testing.T) {
	e, meter, _ := bulkDoorsApp(t, "bulk-charge", agentvolume.Limits{Writes: 60})
	colleague := seedColleague(t, e)
	agent := mintPassport(t, e)
	client := mcpBulkDoor(apptest.NewMCPClient(e, agent.token))

	items := seedBulkContacts(t, e, 25)
	change := reassignContacts(colleague, items)
	preview, refusal := client.preview(t, change)
	if refusal != "" {
		t.Fatalf("preview refused %q", refusal)
	}
	if got := writesCharged(t, e, meter, agent); got != 0 {
		t.Fatalf("a preview charged %d writes, want none — it changes nothing", got)
	}
	change["confirm_token"] = preview.ConfirmToken
	if out, refusal := client.execute(t, change, ""); refusal != "" || out.Changed != 25 {
		t.Fatalf("executing → %+v, refused %q", out, refusal)
	}
	if got := writesCharged(t, e, meter, agent); got != 25 {
		t.Errorf("a 25-record change charged %d writes, want 25", got)
	}

	// The REST door charges the same way.
	more := seedBulkContacts(t, e, 3)
	var out bulkResultDTO
	if status := e.Call(t, "POST", "/v1/bulk/execute", reassignContacts(colleague, more), agent.header, &out); status != http.StatusOK {
		t.Fatalf("executing over REST as the agent → %d", status)
	}
	if got := writesCharged(t, e, meter, agent); got != 28 {
		t.Errorf("after three more over REST the window reads %d writes, want 28", got)
	}
}

func TestABulkChangeTheWriteBudgetCannotPayForIsRefusedAndAsksNobody(t *testing.T) {
	e, meter, _ := bulkDoorsApp(t, "bulk-refuse", agentvolume.Limits{Writes: 30})
	colleague := seedColleague(t, e)
	agent := mintPassport(t, e)
	client := mcpBulkDoor(apptest.NewMCPClient(e, agent.token))
	spendCounter(t, e, meter, agent.id, agentvolume.Writes, 25)

	// Ten records fit no confirmation, and do not fit the five writes left.
	items := seedBulkContacts(t, e, 10)
	if _, refusal := client.execute(t, reassignContacts(colleague, items), ""); refusal == "" {
		t.Fatal("a change past the write budget ran")
	}
	if n := countOwned(t, e, `SELECT count(*) FROM contact WHERE owner_id = $1`, colleague); n != 0 {
		t.Errorf("the refused change still moved %d contacts", n)
	}

	// A window already spent refuses on the way in, on both doors, and puts no
	// release question in anybody's inbox.
	spendCounter(t, e, meter, agent.id, agentvolume.Writes, 5)
	if _, refusal := client.preview(t, reassignContacts(colleague, items[:2])); refusal == "" {
		t.Error("the tool ran for an agent past its write budget")
	}
	if status := e.Call(t, "POST", "/v1/bulk/execute", reassignContacts(colleague, items[:2]), agent.header, nil); status != http.StatusTooManyRequests {
		t.Errorf("the REST door for an agent past its write budget → %d, want 429", status)
	}
	if n := countOwned(t, e, `SELECT count(*) FROM approval WHERE kind = $1`, approvals.KindVolumeRelease); n != 0 {
		t.Errorf("%d release questions were staged for a bulk change; it is confirmed in the conversation, never the inbox", n)
	}
}

// bulkAgent is one passport, as both doors present it.
type bulkAgent struct {
	id     ids.UUID
	token  string
	header map[string]string
}

func mintPassport(t *testing.T, e *apptest.AppEnv) bulkAgent {
	t.Helper()
	header, id := passportWithID(t, e, "bulk agent", "read", "write")
	return bulkAgent{id: id, token: strings.TrimPrefix(header["Authorization"], "Bearer "), header: header}
}
