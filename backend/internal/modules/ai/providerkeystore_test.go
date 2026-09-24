// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// keyStoreOver builds a vault-less store whose service-account check reaches
// endpoint. Vault-less on purpose: every refusal these cases prove comes
// before the seal, and a key that passes reads as ErrVaultUnavailable.
func keyStoreOver(endpoint *tokenEndpoint) *ProviderKeyStore {
	return &ProviderKeyStore{selectBrain: func(cfg ProviderConfig, keys config.Lookup) (model.Client, error) {
		return selectBrainOn(cfg, keys, &http.Client{Timeout: CallCeiling, Transport: endpoint})
	}}
}

func TestAServiceAccountKeyIsSealedOnlyOnceGoogleExchangesIt(t *testing.T) {
	t.Parallel()
	endpoint := &tokenEndpoint{respond: grantedToken("ya29.checked")}
	admin := keySeatCtx(principal.ObjectGrant{Read: true, Update: true})
	keyFile := "\n" + serviceAccountJSON(t, nil) + "\n"

	err := keyStoreOver(endpoint).Set(admin, providerGeminiVertex, ProviderCredential{ServiceAccountJSON: keyFile})

	if !errors.Is(err, ErrVaultUnavailable) {
		t.Fatalf("a key Google exchanged stopped short of the seal: %v", err)
	}
	if endpoint.exchanges.Load() != 1 || endpoint.elsewhere.Load() != 0 {
		t.Errorf("exchanges = %d, other calls = %d; want exactly one token exchange and nothing else",
			endpoint.exchanges.Load(), endpoint.elsewhere.Load())
	}
}

func TestAServiceAccountKeyGoogleRefusesIsTheCallersFaultAndIsNotRepeated(t *testing.T) {
	t.Parallel()
	endpoint := &tokenEndpoint{respond: func(*http.Request) tokenReply {
		return tokenReply{status: http.StatusBadRequest, body: `{"error":"invalid_grant","error_description":"Invalid JWT Signature."}`}
	}}
	admin := keySeatCtx(principal.ObjectGrant{Read: true, Update: true})
	keyFile := serviceAccountJSON(t, nil)

	err := keyStoreOver(endpoint).Set(admin, providerGeminiVertex, ProviderCredential{ServiceAccountJSON: keyFile})

	var invalid settings.InvalidValue
	if !errors.As(err, &invalid) {
		t.Fatalf("want a 422-shaped refusal, got %v", err)
	}
	if !strings.Contains(invalid.Reason, "not accepted by Google") || !strings.Contains(invalid.Reason, "invalid_grant") {
		t.Errorf("reason = %q, want it to say Google refused the key and why", invalid.Reason)
	}
	for _, secret := range []string{"PRIVATE KEY", "eyJ", "Invalid JWT Signature"} {
		if strings.Contains(invalid.Reason, secret) {
			t.Errorf("the refusal repeats %q: %q", secret, invalid.Reason)
		}
	}
}

// Through the HTTP layer: the 422 names the fault, and the body carries
// nothing that was sent.
func TestTheKeyRouteRefusesTheWrongKindWithoutEchoingIt(t *testing.T) {
	t.Parallel()
	endpoint := &tokenEndpoint{respond: grantedToken("ya29.unused")}
	h := Handlers{}.WithProviderKeys(keyStoreOver(endpoint))
	admin := keySeatCtx(principal.ObjectGrant{Read: true, Update: true})
	for name, tc := range map[string]struct{ provider, body, want string }{
		"an api key for vertex":   {providerGeminiVertex, `{"api_key":"sk-sent-secret"}`, "takes a service_account_json"},
		"a key file for gemini":   {providerGemini, `{"service_account_json":"sk-sent-secret"}`, "takes an api_key"},
		"both":                    {providerGeminiVertex, `{"api_key":"sk-sent-secret","service_account_json":"sk-sent-secret"}`, "not both"},
		"neither":                 {providerGeminiVertex, `{}`, "service-account key is empty"},
		"a key file that is none": {providerGeminiVertex, `{"service_account_json":"sk-sent-secret"}`, "not a JSON object"},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/v1/ai/provider-keys/"+tc.provider, strings.NewReader(tc.body)).WithContext(admin)
		h.SetAiProviderKey(rec, req, tc.provider)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: status = %d, want 422", name, rec.Code)
		}
		if body := rec.Body.String(); !strings.Contains(body, tc.want) || strings.Contains(body, "sk-sent-secret") {
			t.Errorf("%s: body = %s, want it to say %q and repeat nothing sent", name, body, tc.want)
		}
	}
	if n := endpoint.exchanges.Load() + endpoint.elsewhere.Load(); n != 0 {
		t.Errorf("a refused credential still reached Google %d time(s)", n)
	}
}
