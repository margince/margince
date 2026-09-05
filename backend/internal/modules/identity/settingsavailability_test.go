// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// /me reports the company-context availability it was injected with, and reports
// unavailable when nothing injected one.
//
// The fail-closed direction is the half worth a test. An installation that
// never wired the rollout must hide the Company page rather than offer one its
// own endpoints would refuse, and the way that breaks is a struct default
// flipping or the field being read from somewhere else — neither of which
// surfaces as an error.
func TestMeReportsTheCompanyContextAvailabilityItWasGiven(t *testing.T) {
	for _, tt := range []struct {
		name  string
		build func() Handlers
		want  bool
	}{
		{"nothing injected", func() Handlers { return NewHandlers(&Service{}) }, false},
		{"injected available", func() Handlers {
			return NewHandlers(&Service{}).WithCompanyContextAvailable(true)
		}, true},
		{"injected unavailable", func() Handlers {
			return NewHandlers(&Service{}).WithCompanyContextAvailable(false)
		}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.build().meResponse(Identity{}, crmcontracts.Native)
			if got.SettingsAvailability == nil {
				t.Fatal("/me carries no settings_availability, so every client reading it fails " +
					"closed and the Company page disappears whatever the installation configured")
			}
			if got.SettingsAvailability.CompanyContext != tt.want {
				t.Errorf("company_context = %v, want %v",
					got.SettingsAvailability.CompanyContext, tt.want)
			}
		})
	}
}

// The row scope /me publishes is the one the principal actually holds, and an
// unresolved scope publishes the narrowest rather than an empty enum value.
//
// A cast would put "" on the wire for a principal whose scope never resolved.
// The client reads absent as `own` and so agrees in the end — but by accident,
// through a value the contract does not declare, and the next reader cannot tell
// an unresolved scope from a deliberate one.
func TestMePublishesTheRowScopeAndFailsClosedOnAnUnknownOne(t *testing.T) {
	for _, tt := range []struct {
		scope string
		want  crmcontracts.AuthorizationRowScope
	}{
		{"own", crmcontracts.AuthorizationRowScopeOwn},
		{"team", crmcontracts.AuthorizationRowScopeTeam},
		{"all", crmcontracts.AuthorizationRowScopeAll},
		{"", crmcontracts.AuthorizationRowScopeOwn},
		{"workspace", crmcontracts.AuthorizationRowScopeOwn},
	} {
		t.Run("scope "+tt.scope, func(t *testing.T) {
			id := Identity{}
			id.Permissions.RowScope = principal.RowScope(tt.scope)
			got := NewHandlers(&Service{}).meResponse(id, crmcontracts.Native)
			if got.Authorization == nil {
				t.Fatal("/me carries no authorization block")
			}
			if got.Authorization.RowScope != tt.want {
				t.Errorf("row_scope = %q, want %q", got.Authorization.RowScope, tt.want)
			}
			if !got.Authorization.RowScope.Valid() {
				t.Errorf("row_scope %q is not a value the contract enum declares",
					got.Authorization.RowScope)
			}
		})
	}
}

// The access preview narrows an unresolved scope exactly as /me does.
//
// One column, two wire shapes: oapi-codegen renders the same three values as two
// Go types, and the preview used to cast where /me maps. A cast puts "" on the
// wire for a principal whose scope never resolved, so the same defect had one
// fixed side and one open one — which is the shape review-loop rule 1 exists to
// close.
func TestTheAccessPreviewNarrowsAnUnknownScopeLikeMeDoes(t *testing.T) {
	for _, tt := range []struct {
		scope string
		want  crmcontracts.AccessPreviewRowScope
	}{
		{"own", crmcontracts.AccessPreviewRowScopeOwn},
		{"team", crmcontracts.AccessPreviewRowScopeTeam},
		{"all", crmcontracts.AccessPreviewRowScopeAll},
		{"", crmcontracts.AccessPreviewRowScopeOwn},
		{"workspace", crmcontracts.AccessPreviewRowScopeOwn},
	} {
		t.Run("scope "+tt.scope, func(t *testing.T) {
			got := accessPreviewRowScope(principal.RowScope(tt.scope))
			if got != tt.want {
				t.Errorf("row_scope = %q, want %q", got, tt.want)
			}
			if !got.Valid() {
				t.Errorf("row_scope %q is not a value the contract enum declares", got)
			}
			// The two shapes carry the same decision, so a change to one that
			// does not reach the other is the drift this pins.
			if string(got) != string(contractRowScope(principal.RowScope(tt.scope))) {
				t.Errorf("the preview says %q where /me says %q for the same stored scope",
					got, contractRowScope(principal.RowScope(tt.scope)))
			}
		})
	}
}

// Every wire shape carrying a row scope maps it; none casts.
//
// The test above proves accessPreviewRowScope narrows correctly, which is not
// the same claim: reverting the handler to a cast leaves that test green,
// because it calls the helper directly. What has to hold is that no producer
// bypasses the mapping, and that is a property of the call sites.
//
// A cast is invisible in the result for every scope the store actually holds —
// the three values map to themselves — and wrong only for the unresolved one,
// which is exactly the case no fixture thinks to write.
func TestNoWireShapeCastsARowScope(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("listing the package: %v", err)
	}
	cast := regexp.MustCompile(`crmcontracts\.\w*RowScope\((?:a|id|h)?\.?\w*\.?Permissions\.RowScope\)`)
	var found []string
	for _, name := range sources {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, readErr := os.ReadFile(name)
		if readErr != nil {
			t.Fatalf("reading %s: %v", name, readErr)
		}
		for _, hit := range cast.FindAllString(string(body), -1) {
			found = append(found, name+": "+hit)
		}
	}
	if len(found) > 0 {
		t.Errorf("a stored row scope is cast onto a contract enum at %v — a cast puts the "+
			"unresolved \"\" on the wire, which the enum does not declare. Use contractRowScope "+
			"or accessPreviewRowScope, which narrow it to `own`.", found)
	}
}
