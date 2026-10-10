// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A passport refused for lacking a scope is told which one. The bare sentinel
// answered "scope exceeds grantor", which names no permission to ask the user
// for.
func TestAMissingScopeIsNamedInTheRefusal(t *testing.T) {
	for _, scope := range []principal.Scope{principal.ScopeRead, principal.ScopeDraft, principal.ScopeWrite} {
		fault, ok := Classify(&auth.ScopeRequiredError{Scope: scope})

		if !ok || fault.Status != http.StatusForbidden || fault.Code != "scope_exceeds_grantor" {
			t.Fatalf("scope %s classified as %+v (ok=%v), want 403 scope_exceeds_grantor", scope, fault, ok)
		}
		if !strings.Contains(fault.Detail, `"`+string(scope)+`"`) {
			t.Errorf("the refusal for %s does not name the scope: %q", scope, fault.Detail)
		}
	}
}
