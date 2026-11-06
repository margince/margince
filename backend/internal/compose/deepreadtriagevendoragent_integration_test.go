// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// An agent answers the question its user was asked, and the row says an agent
// answered it, whichever way it went.
func TestAnAgentAnsweringADomainQuestionNamesItself(t *testing.T) {
	for _, tc := range []struct {
		verb string
		want int
	}{
		{"keep-vendor", http.StatusNoContent},
		{"keep", http.StatusOK},
	} {
		t.Run(tc.verb, func(t *testing.T) {
			e := integration.Setup(t)
			now := pinClock(t, e)
			captureStaffMail(t, e, staffMail{
				sourceID: "vendor-agent", from: hostingStaff, mailbox: e.Rep1,
				to: []string{seatEmail(t, e.Rep1)}, occurred: now.Add(-time.Hour),
			})
			runVendorTriage(t, e, hostingDomain)
			if code := answerDomain(actingAgentOf(t, e, e.Rep1), e, tc.verb, hostingDomain); code != tc.want {
				t.Fatalf("the agent's answer → %d, want %d", code, tc.want)
			}
			if source := e.WsScalar(t, `SELECT source FROM company_domain_disposition WHERE domain = $1`, hostingDomain); source != "agent" {
				t.Errorf("the answer's source = %q, want agent", source)
			}
		})
	}
}

func actingAgentOf(t *testing.T, e *integration.Env, user ids.UUID) context.Context {
	t.Helper()
	seat := e.As(user, nil, integration.AdminPerms)
	agent, ok := principal.Actor(seat)
	if !ok {
		t.Fatal("no actor")
	}
	agent.Type, agent.ID = principal.PrincipalAgent, "agent:"+ids.NewV7().String()
	agent.PassportID, agent.OnBehalfOf = ids.NewV7(), user
	return principal.WithActor(seat, agent)
}
