// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the settings screen is told about sign-in providers. The DEPLOYMENT's
// list is the spine and the admin's choice only marks it: credentials decide
// what is possible, and the admin decides what is offered.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/identity"
)

func TestSignInProvidersMarksTheDeploymentsListWithTheAdminsChoice(t *testing.T) {
	h := installationSettingsHandlers{
		configuredProviders: []identity.OIDCProviderConfig{
			{Key: "google", Label: "Google"},
			{Key: "microsoft", Label: "Microsoft"},
		},
	}

	// Never chosen: every configured provider is offered, so an installation
	// that upgrades into the setting keeps the login screen it had.
	all := h.signInProviders(nil)
	if len(all) != 2 || !all[0].Enabled || !all[1].Enabled {
		t.Fatalf("with no choice recorded, providers = %+v; want both offered", all)
	}

	// A choice marks the same spine rather than replacing it: the provider the
	// admin switched off is still LISTED, because the credentials are still
	// there and they have to be able to switch it back on.
	narrowed := h.signInProviders([]string{"microsoft"})
	if len(narrowed) != 2 {
		t.Fatalf("a narrowed choice dropped a row: %+v — an admin cannot re-enable what is not shown", narrowed)
	}
	if narrowed[0].Key != "google" || narrowed[0].Enabled {
		t.Errorf("google = %+v, want listed and not offered", narrowed[0])
	}
	if narrowed[1].Key != "microsoft" || !narrowed[1].Enabled {
		t.Errorf("microsoft = %+v, want listed and offered", narrowed[1])
	}

	// A key the deployment never composed is not invented into the list: an
	// admin cannot add a provider from this screen, and showing one would offer
	// a control that can never work.
	stray := h.signInProviders([]string{"okta"})
	if len(stray) != 2 {
		t.Fatalf("providers = %+v, want only the two the deployment composed", stray)
	}
	for _, p := range stray {
		if p.Enabled {
			t.Errorf("%s is offered on the strength of a choice naming a provider this deployment cannot serve", p.Key)
		}
	}
}

// A deployment that composed none offers none, and the screen has nothing to
// toggle rather than a list nobody can use.
func TestSignInProvidersIsEmptyWhenTheDeploymentComposedNone(t *testing.T) {
	var h installationSettingsHandlers
	if got := h.signInProviders(nil); len(got) != 0 {
		t.Errorf("providers = %+v, want none", got)
	}
}
