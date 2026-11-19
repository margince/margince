// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A gate's refusal reaches REST and MCP callers as a sentence. The grant it
// lacks is a field an agent can hand to an admin.
func TestAGateRefusalIsASentenceThatNamesTheMissingGrant(t *testing.T) {
	rep := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:rep", Permissions: principal.Permissions{
			RoleKeys: []string{"rep"}, RowScope: principal.RowScopeOwn,
			Objects: map[string]principal.ObjectGrant{"deal": {Read: true}},
		},
	})
	oneOf, err := auth.ParseCheck("deal.delete,admin")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		gate       string
		refusal    error
		detail     string
		wantsGrant string
	}{
		{"Require", auth.Require(rep, "data_task", principal.ActionUpdate),
			"your role does not grant update on data task", "data_task.update"},
		{"RequireAdmin", auth.RequireAdmin(rep), "only an admin may do this", "admin"},
		{"RequireRowScope", auth.RequireRowScope(rep, principal.RowScopeTeam),
			"this needs a role that sees team records or more, and yours sees fewer", "scope:team"},
		{"RequireOneOf", auth.RequireOneOf(rep, oneOf.OneOf...),
			"your role grants none of what this needs: delete on deal, or admin", "deal.delete,admin"},
	} {
		t.Run(tc.gate, func(t *testing.T) {
			if !errors.Is(tc.refusal, apperrors.ErrPermissionDenied) {
				t.Fatalf("refusal %v does not wrap ErrPermissionDenied", tc.refusal)
			}
			fault, ok := httperr.Classify(tc.refusal)
			if !ok || fault.Status != http.StatusForbidden {
				t.Fatalf("classified as %+v, want a 403", fault)
			}
			if fault.Detail != tc.detail {
				t.Errorf("detail = %q, want %q", fault.Detail, tc.detail)
			}
			if got := fault.Details["missing_grant"]; got != tc.wantsGrant {
				t.Errorf("missing_grant = %v, want %q", got, tc.wantsGrant)
			}
		})
	}
}

// A Deal Room participant is refused for who it is, not for a grant a role
// could add. Its refusal names no grant to ask for.
func TestABuyersRefusalNamesNoGrant(t *testing.T) {
	buyer := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalBuyer, ID: "buyer:room"})
	fault, ok := httperr.Classify(auth.Require(buyer, "deal", principal.ActionRead))
	if !ok || fault.Status != http.StatusForbidden {
		t.Fatalf("classified as %+v, want a 403", fault)
	}
	if _, named := fault.Details["missing_grant"]; named {
		t.Errorf("a buyer's refusal names a grant: %v", fault.Details)
	}
}
