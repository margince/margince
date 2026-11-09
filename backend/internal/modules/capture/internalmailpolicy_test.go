// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestInternalMailPolicyRequiresAnAdministratorOnEverySettingsOperation(t *testing.T) {
	for _, kind := range []principal.PrincipalType{principal.PrincipalHuman, principal.PrincipalAgent, principal.PrincipalSystem} {
		for _, role := range []string{"rep", "ops", "admin"} {
			ctx := principal.WithActor(context.Background(), principal.Principal{
				Type: kind,
				Permissions: principal.Permissions{RoleKeys: []string{role}, Objects: map[string]principal.ObjectGrant{
					captureSettingsObject: {Read: true, Update: true},
				}},
			})
			for _, action := range []principal.Action{principal.ActionRead, principal.ActionUpdate} {
				var err error
				if action == principal.ActionRead {
					err = settings.RequireRead(ctx, internalMailPolicy)
				} else {
					err = settings.RequireWrite(ctx, internalMailPolicy)
				}
				wantAllowed := role == "admin" && (kind == principal.PrincipalHuman || (action == principal.ActionRead && kind == principal.PrincipalAgent))
				if (err == nil) != wantAllowed {
					t.Errorf("%s %s %s: %v", kind, role, action, err)
				}
			}
		}
	}
}

func TestInternalMailFolderScopePreservesProviderCaseAndRefusesMixedProviders(t *testing.T) {
	for _, tc := range []struct {
		provider   string
		containers []string
		valid      bool
	}{
		{"graph", []string{"graph:AbCd", "graph:abcd"}, true},
		{"gmail", []string{"gmail:INBOX"}, true},
		{"imap", []string{"imap:Customer Operations"}, true},
		{"gmail", []string{"graph:AbCd"}, false},
		{"graph", []string{"graph:"}, false},
		{"imap", []string{"imap:Inbox", "imap:Inbox"}, false},
		{"imap", []string{"imap:Inbox\nPrivate"}, false},
	} {
		err := validateInternalMailContainers(tc.provider, tc.containers)
		if (err == nil) != tc.valid {
			t.Errorf("%s %q: %v", tc.provider, tc.containers, err)
		}
		if err != nil && !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Errorf("unclassified refusal: %v", err)
		}
	}
}

func TestInternalMailSourceFoldersRefuseAnAdministratorsAgentBeforeResolvingCredentials(t *testing.T) {
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalAgent, Permissions: principal.Permissions{RoleKeys: []string{"admin"}, Objects: map[string]principal.ObjectGrant{captureSettingsObject: {Read: true}}}})
	var registry Registry
	if _, _, err := registry.InternalMailSourceContainers(ctx, ids.NewV7(), "gmail"); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("agent source-folder read: %v", err)
	}
}
