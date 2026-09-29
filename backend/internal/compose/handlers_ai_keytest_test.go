// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A key test reaches the installation's credentials, so an agent is refused
// it whatever grant its passport carries.
func TestAnAgentCannotTestAProviderKey(t *testing.T) {
	h := aiRoutingHandlers{store: &ai.RoutingStore{}}
	rec := httptest.NewRecorder()

	h.TestAiProviderKey(rec, agentReq(""), "anthropic")

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d — an agent tested a stored credential", rec.Code, http.StatusForbidden)
	}
}

// The grant is checked before the routing document is read, so a seat without
// it learns nothing about which vendors this installation holds keys for.
func TestAKeyTestNeedsTheRoutingReadGrant(t *testing.T) {
	h := aiRoutingHandlers{store: &ai.RoutingStore{}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/ai/provider-keys/anthropic/test", nil)
	ctx := principal.WithWorkspaceID(req.Context(), ids.NewV7())
	req = req.WithContext(principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"automation": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	}))

	h.TestAiProviderKey(rec, req, "anthropic")

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d without ai_routing:read", rec.Code, http.StatusForbidden)
	}
}

func TestAnUnwiredKeyTestIsNotImplemented(t *testing.T) {
	var h aiRoutingHandlers
	rec := httptest.NewRecorder()
	h.TestAiProviderKey(rec, httptest.NewRequest(http.MethodPost, "/v1/ai/provider-keys/x/test", nil), "x")
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

// `model_count` and `reason` each describe one side of `ok`, and a client
// reading the other side's field would be reading a zero that means nothing.
func TestAKeyTestCarriesOnlyTheFieldsOfItsOutcome(t *testing.T) {
	passed := toContractKeyTest(ai.KeyTest{Provider: "openai", OK: true, ModelCount: 0, Counted: true})
	if !passed.Ok || passed.ModelCount == nil || *passed.ModelCount != 0 || passed.Reason != nil {
		t.Fatalf("a pass carries its count, zero included, and no reason: %+v", passed)
	}
	uncounted := toContractKeyTest(ai.KeyTest{Provider: "jev_compatible", OK: true, Unconfirmed: true})
	if !uncounted.Ok || uncounted.ModelCount != nil || uncounted.KeyConfirmed == nil || *uncounted.KeyConfirmed {
		t.Fatalf("an unconfirmed pass that listed nothing carries no count and key_confirmed false: %+v", uncounted)
	}
	if passed.KeyConfirmed == nil || !*passed.KeyConfirmed {
		t.Fatalf("a listing pass confirms the key: %+v", passed)
	}
	failed := toContractKeyTest(ai.KeyTest{Provider: "openai", Reason: ai.KeyTestAuthFailed})
	want := crmcontracts.AiProviderKeyTestResultReasonAuthFailed
	if failed.Ok || failed.ModelCount != nil || failed.Reason == nil || *failed.Reason != want {
		t.Fatalf("a failure carries its reason and no count: %+v", failed)
	}
}

// The wire enum is a declared mirror of the store's reasons: a reason the
// contract does not list would reach the screen as a word it has no copy for.
func TestEveryKeyTestReasonIsOnTheWire(t *testing.T) {
	for _, reason := range []ai.KeyTestReason{
		ai.KeyTestNoKey, ai.KeyTestProfileForbids, ai.KeyTestNotPublished, ai.KeyTestNoEndpoint,
		ai.KeyTestAuthFailed, ai.KeyTestRateLimited, ai.KeyTestUnreachable,
	} {
		if !crmcontracts.AiProviderKeyTestResultReason(reason).Valid() {
			t.Errorf("reason %q is not in the contract's enum", reason)
		}
	}
}
