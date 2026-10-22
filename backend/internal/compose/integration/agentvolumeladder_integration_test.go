// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The volume ladder over the real stack. It uses a live Redis counter, a real
// approval row and a real decision by a real human. Then the same passport
// changes again.
//
// The unit suites prove each half against a stub. Only this lane proves that the
// halves meet. The window the gate refused on must be the window the approval
// widened, through the approvals HTTP surface, with one meter pointer at both
// ends. A wiring fault shows as an approval that reads as granted while the
// agent stays refused.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/redistest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ladderApp is the app under live counters at thresholds a test can actually
// cross, plus the meter so a test can put a window into the state it is about.
func ladderApp(t *testing.T, slug string, limits agentvolume.Limits) (*apptest.AppEnv, *agentvolume.Meter) {
	t.Helper()
	// The DEFAULT window, not a short one: these tests spend, stage, approve and
	// re-read on the REAL clock, and a one-hour bucket resets under any run that
	// crosses the top of the hour — a flake that would read as the release
	// having failed.
	meter := agentvolume.New(redistest.Client(t), limits, agentvolume.DefaultWindow)
	// The CONNECTOR composition, because half the ladder only exists behind
	// /mcp: the REST door refuses on the volume budget but has no tool to name, so the
	// step-up it would stage has no question in it. The hosted transport is
	// where a refusal becomes something a human can answer.
	e := apptest.SetupAppWithOriginOptions(t, func(origin string) []compose.Option {
		return []compose.Option{
			compose.WithMCPConnector(), compose.WithMCPResource(origin + "/mcp"),
			compose.WithAgentVolume(meter),
		}
	})
	apptest.BootstrapWorkspaceSession(t, e, "Volume Ladder", slug+"@fable.test", "Admin")
	return e, meter
}

// spendCounter charges one counter against one passport, as the surface would.
func spendCounter(t *testing.T, e *apptest.AppEnv, meter *agentvolume.Meter, passport ids.UUID, c agentvolume.Counter, n int) {
	t.Helper()
	var ws ids.UUID
	if err := e.Owner.QueryRow(t.Context(), `SELECT id FROM workspace LIMIT 1`).Scan(&ws); err != nil {
		t.Fatalf("reading the workspace id: %v", err)
	}
	ctx := principal.WithWorkspaceID(t.Context(), ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + passport.String(), PassportID: passport,
	})
	if err := meter.Consume(ctx, c, n); err != nil {
		t.Fatalf("spending the %s window: %v", c, err)
	}
}

// pendingStepUp reads the staged step-up out of the database. It goes to the
// table rather than the inbox API because the inbox is scoped to the DECIDING
// human, and half of what this file proves is which human that is.
func pendingStepUp(t *testing.T, e *apptest.AppEnv) (id ids.ApprovalID, passport ids.UUID, summary string) {
	t.Helper()
	err := e.Owner.QueryRow(t.Context(), `
		SELECT id, passport_id, coalesce(summary, '') FROM approval
		 WHERE kind = $1 AND status = 'pending'`, approvals.KindVolumeRelease).Scan(&id, &passport, &summary)
	if err != nil {
		t.Fatalf("reading the staged step-up: %v", err)
	}
	return id, passport, summary
}

// callTool makes one tools/call over the mounted MCP surface, under the
// presented passport. It ignores the answer: every caller here is about what
// the call left BEHIND — a staged question, or the absence of one — and a tool
// refusal is the expected outcome in all of them.
func callTool(t *testing.T, e *apptest.AppEnv, bearer map[string]string, tool string, args AnyMap) {
	t.Helper()
	status := e.Call(t, "POST", "/mcp", AnyMap{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": AnyMap{"name": tool, "arguments": args},
	}, bearer, nil)
	if status != http.StatusOK {
		t.Fatalf("tools/call %s → %d, want 200 with the refusal inside the result", tool, status)
	}
}

// BYO-STEP-2, whole: a change past the threshold is refused and the question
// reaches the human who lent the passport. Their approval widens that window,
// and the same passport changes again through the same door.
func TestAWritePastItsThresholdIsReleasedByTheHumanWhoLentThePassport(t *testing.T) {
	e, meter := ladderApp(t, "ladder-write", agentvolume.Limits{Writes: 10})
	bearer, passport := passportWithID(t, e, "writing agent", "read", "write")
	spendCounter(t, e, meter, passport, agentvolume.Writes, 12)

	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": "Refused"}, bearer, nil); status != http.StatusTooManyRequests {
		t.Fatalf("a write past its threshold → %d, want 429", status)
	}

	// The REST door refuses without staging, because it has no tool to name. The
	// question is put through the MCP door, which the ladder is written for.
	callTool(t, e, bearer, "create_record", AnyMap{
		"record_type": "contact", "fields": AnyMap{"source": "manual", "full_name": "Metered"},
	})

	id, staged, summary := pendingStepUp(t, e)
	if staged != passport {
		t.Fatalf("the step-up was staged against passport %s, want the one that was refused (%s)", staged, passport)
	}
	if summary == "" {
		t.Error("the step-up carries no sentence for a human to answer")
	}

	// The human decides through the real surface (the same POST the inbox
	// makes) and the release is applied as a consequence of that decision.
	if status := e.Call(t, "POST", "/v1/approvals/"+id.String()+"/approve", nil, nil, nil); status != http.StatusOK {
		t.Fatalf("approving the step-up → %d", status)
	}

	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": "Admitted"}, bearer, nil); status != http.StatusCreated {
		t.Errorf("the agent is still refused after its human approved the step-up → %d; "+
			"the window the gate reads and the window the approval widened are not the same window", status)
	}
}

// The other half of the same property, and the one that would fail silently:
// approving does not hand out an UNBOUNDED continuation. One release is one
// more allowance, so the agent that spends it is refused again and its human is
// asked again.
func TestOneReleaseIsOneMoreAllowanceAndNotAStandingPermission(t *testing.T) {
	e, meter := ladderApp(t, "ladder-once", agentvolume.Limits{Writes: 10})
	bearer, passport := passportWithID(t, e, "writing agent", "read", "write")
	spendCounter(t, e, meter, passport, agentvolume.Writes, 12)
	callTool(t, e, bearer, "create_record", AnyMap{
		"record_type": "contact", "fields": AnyMap{"source": "manual", "full_name": "Metered"},
	})
	id, _, _ := pendingStepUp(t, e)
	if status := e.Call(t, "POST", "/v1/approvals/"+id.String()+"/approve", nil, nil, nil); status != http.StatusOK {
		t.Fatalf("approving the step-up → %d", status)
	}

	// The released allowance is spent too.
	spendCounter(t, e, meter, passport, agentvolume.Writes, 10)

	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": "Refused again"}, bearer, nil); status != http.StatusTooManyRequests {
		t.Errorf("a second crossing after one release → %d, want 429: approving once granted a standing permission", status)
	}
}

// BYO-STEP-4: a spent CALL ceiling suspends the passport for every verb and
// reaches no inbox. Releasing it would release every other counter at once.
func TestASpentCallCeilingAsksNobodyAnything(t *testing.T) {
	e, meter := ladderApp(t, "ladder-calls", agentvolume.Limits{Calls: 5})
	bearer, passport := passportWithID(t, e, "calling agent", "read", "write")
	seedContacts(t, e, 2)
	spendCounter(t, e, meter, passport, agentvolume.Calls, 5)

	if status := e.Call(t, "GET", "/v1/contacts", nil, bearer, nil); status != http.StatusTooManyRequests {
		t.Fatalf("a read past the call ceiling → %d, want 429", status)
	}
	callTool(t, e, bearer, "create_record", AnyMap{
		"record_type": "contact", "fields": AnyMap{"source": "manual", "full_name": "Suspended"},
	})

	var staged int
	if err := e.Owner.QueryRow(t.Context(),
		`SELECT count(*) FROM approval WHERE kind = $1`, approvals.KindVolumeRelease).Scan(&staged); err != nil {
		t.Fatalf("counting staged step-ups: %v", err)
	}
	if staged != 0 {
		t.Errorf("a suspension reached a human's inbox %d times; no approval lifts it", staged)
	}
}

// BYO-STEP-3: a spent EGRESS ceiling reaches no inbox. Approving it is not a
// thing the meter will do, so staging it would put a question in front of a
// human whose answer changes nothing — and leave the agent waiting for it.
func TestASpentEgressCeilingAsksNobodyAnything(t *testing.T) {
	e, meter := ladderApp(t, "ladder-egress", agentvolume.Limits{Egress: 1})
	bearer, passport := passportWithID(t, e, "sending agent", "read", "send")
	spendCounter(t, e, meter, passport, agentvolume.Egress, 5)

	callTool(t, e, bearer, "send_email", AnyMap{
		"to": "someone@example.com", "subject": "s", "body": "b",
	})

	var staged int
	if err := e.Owner.QueryRow(t.Context(),
		`SELECT count(*) FROM approval WHERE kind = $1`, approvals.KindVolumeRelease).Scan(&staged); err != nil {
		t.Fatalf("counting staged step-ups: %v", err)
	}
	if staged != 0 {
		t.Errorf("a hard stop reached a human's inbox %d times; no approval lifts it", staged)
	}
}
