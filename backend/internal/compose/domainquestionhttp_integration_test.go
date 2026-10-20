// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestDomainQuestionHTTPKeepsTheReadersBoundary(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)
	const domain = "private-question.test"
	openTriageQuestion(t, e, domain, "hello@"+domain, "Sender")
	if _, err := e.Contacts.ResolveUnreadableDomainTriage(ctx, contacts.ResolveDomainTriageInput{Domain: domain}); err != nil {
		t.Fatal(err)
	}
	invalidCursor := "not-a-cursor"
	agent := principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalAgent, ID: "agent:test"})
	for _, tc := range []struct {
		name          string
		ctx           context.Context
		cursor        *string
		status, count int
	}{
		{"owner", ctx, nil, 200, 1},
		{"other reader", e.As(e.Rep2, nil, integration.AdminPerms), nil, 200, 0},
		{"agent", agent, nil, 403, 0},
		{"malformed cursor", ctx, &invalidCursor, 422, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/capture/domain-questions", nil).WithContext(tc.ctx)
			response := httptest.NewRecorder()
			domainQuestionHandlers{contacts: e.Contacts}.ListDomainQuestions(response, request, crmcontracts.ListDomainQuestionsParams{Cursor: tc.cursor})
			if response.Code != tc.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if tc.status != 200 {
				return
			}
			var page crmcontracts.DomainQuestionPage
			if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
			if len(page.Data) != tc.count {
				t.Fatalf("questions=%d, want%d", len(page.Data), tc.count)
			}
		})
	}
}
