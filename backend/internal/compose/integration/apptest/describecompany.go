// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"net/http"
	"testing"
)

// DescribeCompany saves the installation's own company through the form an
// admin fills during onboarding. An installation adds no seat until it has done
// so, so every suite that invites a colleague calls this first.
func (e *AppEnv) DescribeCompany(t *testing.T) {
	t.Helper()
	if status := e.Call(t, http.MethodPut, "/v1/company", map[string]any{
		"display_name":  "Acme GmbH",
		"offer_summary": "Revenue operations software",
		"icp":           "RevOps at SaaS scale-ups",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("describing the company (PUT /v1/company) → %d, want 200", status)
	}
}
