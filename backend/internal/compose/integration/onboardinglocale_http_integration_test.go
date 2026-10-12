// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// The refusal for an unknown locale lists every locale the contract accepts,
// because the list comes from the contract and not from the handler.
func TestTheOnboardingLocaleRefusalNamesEveryAcceptedLocale(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var problem AnyMap
	if status := e.Call(t, "GET", "/v1/onboarding/company/proposal?locale=zz", nil, nil, &problem); status != http.StatusUnprocessableEntity {
		t.Fatalf("an unknown locale → %d, want 422", status)
	}
	raw, err := json.Marshal(problem)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "expected one of: en, de, vi") {
		t.Errorf("the refusal %s does not list en, de and vi", raw)
	}
}
