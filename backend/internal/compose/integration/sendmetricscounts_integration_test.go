// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	everySendOutcome  = "margince_email_send_outcomes_total{"
	everyDecisionTime = "margince_email_send_decision_seconds_count{"
)

// A replay, a scheduled send coming due and an agent's send are no first Send
// of a human. Each of them leaves the outcome and decision counts as they were.
func TestOnlyAHumansFirstSendIsCounted(t *testing.T) {
	p := setupPreflight(t, compose.WithMetricsToken(sendMetricsToken))
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	p.stakeADeal(t)
	route := "/v1/activities/" + p.activityID + "/send-email"
	message := AnyMap{
		"subject": "Your quote", "body": "The figures you asked for.",
		"to": []string{"buyer@preflight.test"}, "consent_purpose": "transactional",
	}
	counts := func() (int, int) {
		return sendSeriesSum(t, p.AppEnv, everySendOutcome), sendSeriesSum(t, p.AppEnv, everyDecisionTime)
	}

	queued := `margince_email_send_outcomes_total{flow="legacy",outcome="queued"} `
	queuedBefore := sendSeriesSum(t, p.AppEnv, queued)
	outcomes, decisions := counts()
	keyed := map[string]string{"Idempotency-Key": "first-send-once"}
	for range 2 {
		if status := p.Call(t, "POST", route, message, keyed, nil); status != http.StatusAccepted {
			t.Fatalf("the keyed Send answered %d, want 202", status)
		}
	}
	if got := sendSeriesSum(t, p.AppEnv, queued) - queuedBefore; got != 1 {
		t.Errorf("a Send and its replay counted %d queued first Sends, want 1", got)
	}
	if o, d := counts(); o-outcomes != 1 || d-decisions != 1 {
		t.Errorf("a Send and its replay counted %d outcomes and %d decisions, want 1 and 1", o-outcomes, d-decisions)
	}

	scheduledSeries := `margince_email_send_outcomes_total{flow="legacy",outcome="scheduled"} `
	scheduledBefore := sendSeriesSum(t, p.AppEnv, scheduledSeries)
	later := AnyMap{
		"subject": "Monday morning", "body": "Written the night before.",
		"to": []string{"buyer@preflight.test"}, "consent_purpose": "transactional",
		"scheduled_at": time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339), "scheduled_tz": "Europe/Berlin",
	}
	var scheduled struct {
		ID string `json:"id"`
	}
	if status := p.Call(t, "POST", route, later, nil, &scheduled); status != http.StatusCreated {
		t.Fatalf("scheduling answered %d, want 201", status)
	}
	if got := sendSeriesSum(t, p.AppEnv, scheduledSeries) - scheduledBefore; got != 1 {
		t.Errorf("scheduling counted %d scheduled first Sends, want 1", got)
	}
	outcomes, decisions = counts()
	id := uuidOf(t, scheduled.ID)
	p.makeDue(t, id)
	p.fire(t, id)
	if !p.sent(t, id) {
		t.Fatal("the scheduled message did not go when it came due")
	}
	if o, d := counts(); o != outcomes || d != decisions {
		t.Errorf("a scheduled send coming due counted %d outcomes and %d decisions, want none", o-outcomes, d-decisions)
	}

	// Refused or sent, the engine decides it at staging, and neither counts.
	_, err := compose.ScheduleAsAgentForTest(context.Background(), p.Pool, p.workspaceID(t), p.sendingAgent(t),
		ids.From[ids.ActivityKind](uuidOf(t, p.activityID)),
		activities.SendEmailInput{
			Recipients: []string{"buyer@preflight.test"}, Subject: "From a tool", Body: "Sent by an agent.",
			ConsentPurpose: "transactional",
		}, time.Now().Add(-time.Minute), compose.SendOrigin{PublicBaseURL: preflightBaseURL})
	if err != nil && !errors.Is(err, apperrors.ErrConsentNotGranted) {
		t.Fatalf("the agent's send through the tool path: %v", err)
	}
	if o, d := counts(); o != outcomes || d != decisions {
		t.Errorf("an agent's send counted %d outcomes and %d decisions, want none", o-outcomes, d-decisions)
	}
}

// sendingAgent is an agent on a live passport of the fixture's user, the
// principal the tool surface sends as.
func (p *preflightEnv) sendingAgent(t *testing.T) principal.Principal {
	t.Helper()
	passport := p.issuePassport(t)
	return principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + passport.String(), PassportID: passport,
		UserID: uuidOf(t, p.user), OnBehalfOf: uuidOf(t, p.user), SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true, Read: true, Update: true},
				"contact":  {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	}
}
