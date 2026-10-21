// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
)

// A consent route is human-only when it holds or changes a contact's decision.
// It is a read tool when it names the workspace's vocabulary.
// The contract says which, and the policy table derives from it, so this test
// reads the table and keeps no list of its own.
//
// The purposes catalog is configuration and holds no one's consent. An agent
// reads it to name a purpose on the send tools. Its writes stay human-only.
func TestConsentRoutesSplitByWhoseDecisionTheyHold(t *testing.T) {
	const contactConsent = "/v1/contacts/{id}/consent"
	seen := 0
	for route, pol := range agentPolicies {
		if _, pattern, _ := strings.Cut(route, " "); strings.HasPrefix(pattern, contactConsent) {
			seen++
			if pol.Access != accessHumanOnly {
				t.Errorf("%s (%s) holds a contact's consent and is admitted as %q; it must be human-only",
					route, pol.Op, pol.Access)
			}
		}
	}
	if seen == 0 {
		t.Fatal("no per-contact consent route in the policy table: the pin no longer covers anything")
	}

	define, known := agentPolicies[http.MethodPost+" /v1/consent-purposes"]
	if !known || define.Access != accessHumanOnly {
		t.Errorf("defining a purpose is a human decision, got %+v (known %v)", define, known)
	}

	catalog, known := agentPolicies[http.MethodGet+" /v1/consent-purposes"]
	if !known {
		t.Fatal("the purposes catalog left the policy table")
	}
	if catalog.Access != accessTool || catalog.Tool != agents.ReadViewTool || catalog.Scope != scopeRead {
		t.Errorf("the purposes catalog is configuration an agent reads through %s with %s scope, got %+v",
			agents.ReadViewTool, scopeRead, catalog)
	}
}
