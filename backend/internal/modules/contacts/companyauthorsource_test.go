// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestACompanyFactNamesTheAuthorWhoTypedIt(t *testing.T) {
	for _, tc := range []struct {
		name string
		ctx  context.Context
		want string
	}{
		{"a signed-in user", principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman}), CompanySourceHuman},
		{"an agent on a passport", principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalAgent}), CompanySourceAgent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := companyAuthorSource(tc.ctx); got != tc.want {
				t.Fatalf("source = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAnAgentsCompanySaveEventSaysTheAgentMadeIt(t *testing.T) {
	payload := companySaveEventPayload(false, map[string]any{}, "agent:test", CompanySourceAgent)
	updated, ok := payload.(crmcontracts.PublicEventCompanyUpdated)
	if !ok {
		t.Fatalf("payload is %T, want the company-updated event", payload)
	}
	if got := updated.ChangedFields[auditKeySource]; got != CompanySourceAgent {
		t.Fatalf("event source = %v, want %q", got, CompanySourceAgent)
	}
}
