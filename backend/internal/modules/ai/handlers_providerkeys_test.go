// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// keySeatCtx binds a human holding exactly the given ai_routing grant.
func keySeatCtx(g principal.ObjectGrant) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test",
		Permissions: principal.Permissions{
			RoleKeys: []string{"fixture"},
			Objects:  map[string]principal.ObjectGrant{providerKeysObject: g},
		},
	})
}

// keyRoutes names the credential surface's three routes so a case about the
// unwired shape covers all of them rather than whichever one was written first.
// A fourth route added without a line here is simply untested by these two
// cases, which the coverage report shows and no claim in this comment hides.
func keyRoutes(h Handlers) map[string]func(http.ResponseWriter, *http.Request) {
	return map[string]func(http.ResponseWriter, *http.Request){
		"GET /ai/provider-keys": h.ListAiProviderKeys,
		"PUT /ai/provider-keys/{provider}": func(w http.ResponseWriter, r *http.Request) {
			h.SetAiProviderKey(w, r, "gemini")
		},
		"DELETE /ai/provider-keys/{provider}": func(w http.ResponseWriter, r *http.Request) {
			h.DeleteAiProviderKey(w, r, "gemini")
		},
	}
}

func keyRequest(ctx context.Context) *http.Request {
	return httptest.NewRequest(http.MethodGet, "/v1/ai/provider-keys", nil).WithContext(ctx)
}

// An entitled seat on an installation with no key vault is told the vault is
// missing, on every route.
//
// 503 and never 501. The distinction is what an integrator does next: 501 says
// this BUILD does not implement the operation, which sends them looking for a
// newer version; 503 says the installation is missing something its operator can
// supply. The contract declares 503 on all three, so a 501 would also be the
// server contradicting its own document.
//
// A zero-value Handlers is exactly the shape compose produces for a role with no
// vault — the store is built inside WithKeyvault — so this is the real unwired
// case rather than a stand-in for it.
func TestTheKeyRoutesReadAsUnavailableWithoutAVault(t *testing.T) {
	ctx := keySeatCtx(principal.ObjectGrant{Read: true, Update: true})

	for route, call := range keyRoutes(Handlers{}) {
		t.Run(route, func(t *testing.T) {
			rec := httptest.NewRecorder()
			call(rec, keyRequest(ctx))

			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
			}
			// The body has to name what is missing. "Service unavailable" alone
			// tells an operator to retry, which will never work.
			var problem struct {
				Detail string `json:"detail"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
				t.Fatalf("decoding the problem body: %v", err)
			}
			if problem.Detail != ErrVaultUnavailable.Error() {
				t.Errorf("detail = %q, want the vault explanation %q", problem.Detail, ErrVaultUnavailable.Error())
			}
		})
	}
}

// A seat with no grant is REFUSED before it learns anything about the vault.
//
// The unwired shape answers 503, and that answer is information: without the
// gate below, an unentitled seat gets 503 where a vault is absent and 403 where
// one is configured, so anybody holding a session can read off whether this
// installation has a vault root key. The remedy is ordering — authorize first,
// then report the wiring — and it is only observable on the unwired shape, which
// is why it belongs here rather than in the integration lane.
func TestAnUnentitledSeatIsRefusedBeforeLearningTheVaultPosture(t *testing.T) {
	// No grant on the object at all, which is what a rep seat holds: the seeded
	// roles give ai_routing read and update together, to admin and ops only.
	ctx := keySeatCtx(principal.ObjectGrant{})

	for route, call := range keyRoutes(Handlers{}) {
		t.Run(route, func(t *testing.T) {
			rec := httptest.NewRecorder()
			call(rec, keyRequest(ctx))

			if rec.Code == http.StatusServiceUnavailable {
				t.Fatalf("an unentitled seat was told the vault posture (503); the status is an oracle for whether a root key is configured")
			}
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
			}
		})
	}
}
