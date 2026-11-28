// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/messengeraccounts"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestDispactPersonalConsentVerifiesIdentityAndCannotFollowAnotherInstallation(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.AdminUser, "admin")
	ctx := integration.OnSeat(e.Admin(), principal.SeatFull)
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/auth/me" || r.Header.Get("Authorization") != "Bearer pat_synthetic" {
			t.Error("consent used another identity or API")
		}
		if _, err := w.Write([]byte(`{"id":"REP","workspace_id":"TEAM","display_name":"Rita","is_active":true,"email":"rita@example.test"}`)); err != nil {
			t.Error(err)
		}
	}))
	defer provider.Close()
	installation := capture.LiveBot{ConnectionID: ids.NewV7(), TeamID: "TEAM", Address: provider.URL}
	binding := func(context.Context) (capture.LiveBot, error) { return installation, nil } //nolint:unparam // The resolver keeps the production error-returning callback signature.
	vault := keyvault.NewMemory()
	store := chatlinks.NewStore(e.DB(), nil, nil, nil).WithMessenger(func() keyvault.Vault { return vault }, messengeraccounts.Clients(true, func(ctx context.Context, _ string) (capture.LiveBot, error) { return binding(ctx) })).WithMemberAuthority(messengeraccounts.MemberAuthority)
	handlers := messengerOAuthHandlers{store: store, http: provider.Client(), dispact: binding}
	server := Server{messengerOAuthHandlers: handlers}
	request := httptest.NewRequest(http.MethodPost, "/v1/messenger/dispact/credential", strings.NewReader(`{"personal_token":"pat_synthetic"}`)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.ConnectDispactParticipant(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("connect: %d %s", response.Code, response.Body.String())
	}
	grant, _, err := store.OpenMember(ctx, "dispact")
	if err != nil || grant.Member != "REP" || grant.Workspace != "TEAM" {
		t.Fatalf("verified grant: %+v %v", grant, err)
	}
	if e.WsScalar(t, `SELECT count(*)::text FROM audit_log WHERE after::text LIKE '%pat_synthetic%'`) != "0" {
		t.Fatal("personal token entered the audit log")
	}
	beforeCalls := calls
	installation.ConnectionID = ids.NewV7()
	if _, _, err := store.OpenMember(ctx, "dispact"); !errors.Is(err, messaging.ErrUnauthorized) {
		t.Fatalf("old personal grant followed another installation: %v", err)
	}
	if calls != beforeCalls {
		t.Fatal("retired installation sent a personal credential to a new connection")
	}
}

func TestOutcomeHTTPUsesTheComposedFilerAndRefusesMissingPersonalConsent(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.AdminUser, "admin")
	company := e.SeedCompany(t, "Pilot", &e.AdminUser)
	s := Server{vault: keyvault.NewMemory()}
	WithChannelSurface()(&s, e.Pool)
	WithChatLinks(nil, true, nil)(&s, e.Pool)
	ctx := integration.OnSeat(e.Admin(), principal.SeatFull)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"conversation_id":"PRIVATE","message_ids":["M1"]}`)).WithContext(ctx)
	response := httptest.NewRecorder()
	s.DraftMessengerOutcome(response, request, crmcontracts.MessengerProvider("slack"), crmcontracts.ChannelLinkRecordType("company"), crmcontracts.Id(company))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "messenger_consent_required") {
		t.Fatalf("outcome wiring did not check personal consent: %d %s", response.Code, response.Body.String())
	}
}
