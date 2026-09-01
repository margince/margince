// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// What a stored OAuth app may be, per vendor. The mechanics — sealing,
// rotation, retirement — are proven against a real database in compose's
// integration suite; these are the rules that need no vault.

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// A vendor this build does not serve is a 404 rather than a default. Defaulting
// would store somebody's Entra secret under the Google key, where the Google
// connector would present it and the Microsoft one would never find it.
func TestAnUnknownVendorIsRefusedRatherThanDefaulted(t *testing.T) {
	for _, name := range []string{"", "gmail", "Google", "microsoft-365", "azure"} {
		if _, err := ParseAppProvider(name); !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("ParseAppProvider(%q) = %v, want a not-found refusal", name, err)
		}
	}
	for _, want := range []AppProvider{AppProviderGoogle, AppProviderMicrosoft} {
		got, err := ParseAppProvider(string(want))
		if err != nil || got != want {
			t.Errorf("ParseAppProvider(%q) = (%q, %v), want it accepted", want, got, err)
		}
	}
}

// Each vendor's console shows several identifiers side by side and only one of
// them authenticates. Refusing the wrong shape here names the mistake, where
// accepting it surfaces much later as an opaque invalid_client on somebody's
// first connect attempt.
func TestEachVendorRefusesTheNeighbouringFieldOnItsOwnConsole(t *testing.T) {
	const (
		googleID = "111-abc.apps.googleusercontent.com"
		entraID  = "11111111-2222-3333-4444-555555555555"
	)
	for name, tc := range map[string]struct {
		vendor   appVendor
		clientID string
		ok       bool
	}{
		"google's own id":                  {googleVendor, googleID, true},
		"a project number":                 {googleVendor, "000000000000", false},
		"an api key off the same screen":   {googleVendor, "AIzaSyD-0000000000000000000000000", false},
		"an entra application id":          {microsoftVendor, entraID, true},
		"the app's display name":           {microsoftVendor, "Margince CRM", false},
		"a guid with a label copied along": {microsoftVendor, "Application (client) ID " + entraID, false},
		// The two vendors' shapes are not interchangeable, and a screen that
		// accepted either would let an operator paste a Google id into the
		// Microsoft card and find out at the consent screen.
		"google's id on the microsoft app": {microsoftVendor, googleID, false},
		"an entra id on the google app":    {googleVendor, entraID, false},
	} {
		t.Run(name, func(t *testing.T) {
			err := tc.vendor.validateClientID(tc.vendor, tc.clientID)
			if tc.ok && err != nil {
				t.Errorf("%q was refused: %v", tc.clientID, err)
			}
			if !tc.ok && err == nil {
				t.Errorf("%q was accepted as a %s client id", tc.clientID, tc.vendor.name)
			}
		})
	}
}

// The tenant is Microsoft's alone, and the three authority aliases are refused
// BY NAME: each is legal at Microsoft's endpoint and each widens the app to a
// population nobody here vetted. An operator who wants that leaves it empty.
func TestOnlyAMicrosoftAppCarriesADirectoryAndNeverAnAlias(t *testing.T) {
	const (
		googleID  = "111-abc.apps.googleusercontent.com"
		entraID   = "11111111-2222-3333-4444-555555555555"
		directory = "99999999-8888-7777-6666-555555555555"
		ref       = "vault-ref"
	)
	for name, tc := range map[string]struct {
		vendor appVendor
		app    ConnectorApp
		ok     bool
	}{
		"a microsoft app pinned to a directory": {
			microsoftVendor, ConnectorApp{ClientID: entraID, ClientSecretRef: ref, Tenant: directory}, true,
		},
		"a microsoft app pinned to nothing": {
			microsoftVendor, ConnectorApp{ClientID: entraID, ClientSecretRef: ref}, true,
		},
		"the common alias": {
			microsoftVendor, ConnectorApp{ClientID: entraID, ClientSecretRef: ref, Tenant: "common"}, false,
		},
		"the organizations alias, however spelled": {
			microsoftVendor, ConnectorApp{ClientID: entraID, ClientSecretRef: ref, Tenant: "Organizations"}, false,
		},
		"the consumers alias": {
			microsoftVendor, ConnectorApp{ClientID: entraID, ClientSecretRef: ref, Tenant: "consumers"}, false,
		},
		"a directory that is not a guid": {
			microsoftVendor, ConnectorApp{ClientID: entraID, ClientSecretRef: ref, Tenant: "contoso.onmicrosoft.com"}, false,
		},
		// Google has no directories. Accepting one would be a field that
		// silently does nothing, and an operator who filled it in would believe
		// they had narrowed something.
		"a google app carrying a directory": {
			googleVendor, ConnectorApp{ClientID: googleID, ClientSecretRef: ref, Tenant: directory}, false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := tc.vendor.validate(tc.app)
			if tc.ok && err != nil {
				t.Errorf("%+v was refused: %v", tc.app, err)
			}
			if !tc.ok && err == nil {
				t.Errorf("%+v was accepted", tc.app)
			}
		})
	}
}

// The empty value is the legitimate default — an installation that has not set
// one up reads as empty — and half of one is what no code path accepts.
func TestAnAppIsWholeOrAbsentAndNeverHalf(t *testing.T) {
	const entraID = "11111111-2222-3333-4444-555555555555"
	if err := microsoftVendor.validate(ConnectorApp{}); err != nil {
		t.Errorf("an unconfigured installation was refused: %v", err)
	}
	for name, app := range map[string]ConnectorApp{
		"an id with no secret": {ClientID: entraID},
		"a secret with no id":  {ClientSecretRef: "vault-ref"},
		// A tenant alone claims to be an app while being unusable, which is the
		// state Configured() and the screen would disagree about.
		"a directory and nothing else": {Tenant: entraID},
	} {
		t.Run(name, func(t *testing.T) {
			if err := microsoftVendor.validate(app); err == nil {
				t.Errorf("%+v was accepted as an app", app)
			}
		})
	}
}

// Each vendor's app is stored under its OWN key. One key for both would mean
// storing a Microsoft app silently replaced the Google one.
func TestEachVendorStoresUnderItsOwnKey(t *testing.T) {
	google, err := appKindFor(AppProviderGoogle)
	if err != nil {
		t.Fatalf("resolving google: %v", err)
	}
	microsoft, err := appKindFor(AppProviderMicrosoft)
	if err != nil {
		t.Fatalf("resolving microsoft: %v", err)
	}
	if google.entry.Key() == microsoft.entry.Key() {
		t.Fatalf("both vendors store under %q; one write would blank the other", google.entry.Key())
	}
}

// The rule is spelled twice, and it has to be: checkAppInput refuses a value
// BEFORE a secret is sealed for it, where the settings validator runs on the
// write — by which point the blob exists and would need retiring. Two spellings
// that disagree is a value sealed and then refused, or worse, one stored that no
// code path accepts.
//
// So they are held against each other over the whole matrix rather than
// separately: whatever one refuses, the other must.
func TestTheTwoSpellingsOfTheAppRuleRefuseTheSameValues(t *testing.T) {
	const (
		googleID  = "111-abc.apps.googleusercontent.com"
		entraID   = "11111111-2222-3333-4444-555555555555"
		directory = "99999999-8888-7777-6666-555555555555"
		secret    = "a-client-secret"
	)
	ids := []string{googleID, entraID, "", "Margince CRM", "000000000000", strings.Repeat("x", maxAppFieldBytes+1)}
	tenants := []string{"", directory, "common", "Organizations", "consumers", "contoso.onmicrosoft.com"}

	for _, p := range []AppProvider{AppProviderGoogle, AppProviderMicrosoft} {
		k, err := appKindFor(p)
		if err != nil {
			t.Fatalf("appKindFor(%q): %v", p, err)
		}
		for _, id := range ids {
			for _, tenant := range tenants {
				// BOTH directions, over the fields both can see. A value one
				// accepts and the other refuses is either a secret sealed and
				// then thrown away, or — the worse half — a row stored that no
				// code path will act on.
				validatorRefused := k.validate(ConnectorApp{
					ClientID: id, ClientSecretRef: "vault-ref", Tenant: tenant,
				}) != nil
				storeRefused := checkAppInput(k, id, secret, tenant) != nil
				if validatorRefused != storeRefused {
					t.Errorf("%s: (id=%q tenant=%q) — the settings validator refuses=%v, the store refuses=%v",
						p, id, tenant, validatorRefused, storeRefused)
				}
			}
		}
	}
}
