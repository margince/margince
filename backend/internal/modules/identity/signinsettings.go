// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The sign-in policy SETTINGS: which external providers an installation offers,
// and the two enforcement switches. Split from settingsentry.go, which holds the
// installation's identity and reporting settings — these decide who may sign in
// and how, the same concern signinpolicy.go reads them back through.

import (
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/platform/settings"
)

// The ceilings on the provider list, mirroring the contract's maxItems and
// maxLength. Generous against any real deployment — nobody wires 32 identity
// providers — and small enough that the anonymous read behind the login screen
// cannot be made expensive by one admin write.
const (
	maxEnabledOidcProviders = 32
	maxProviderKeyLen       = 64
)

// EnabledOidcProviders is which external identity providers this installation
// offers on its login screen, of those the deployment holds credentials for.
// The effective list is the INTERSECTION: this setting can only ever narrow
// what the deployment composed, because an operator cannot invent a client id
// and secret from the settings screen.
//
// PASSWORD IS NOT A MEMBER OF THIS SET, and that is the whole reason the entry
// is named for providers rather than for login methods. Password is the method
// every installation always has and the one an admin must not be able to strand
// everybody by removing, so "it cannot be disabled" is a property of the shape
// here — there is no value of this setting that turns it off — rather than a
// validation rule a later change could relax. GetAuthCapabilities reports
// Password as a constant for the same reason.
//
// Absent (nil) means every provider the deployment configured, so an
// installation that upgrades into this setting keeps the login screen it had
// and nobody has to be told to go and re-enable Google.
//
// It SURVIVES A DATA RESET, which is what AsInstallationIdentity buys and is
// the reason for it here — the marker reads as "identity" but what it decides
// is whether a wipe spares the row. Absent means every configured provider, so
// a reset that deleted this would silently re-open a sign-in method an admin
// had deliberately closed. A data reset clears customers and deals; it is not a
// decision to change who may sign in.
var EnabledOidcProviders = settings.Define[[]string](
	"identity.enabled_oidc_providers",
	installationSettingsObject,
	"update",
	nil,
	func(keys []string) error {
		// Bounded HERE and not only in the contract, because this value is read
		// back on an ANONYMOUS request: the capabilities probe unmarshals it on
		// every login screen, so an oversized list stored once would be paid for
		// by every stranger who loads the page. The entry binds the non-HTTP
		// writer too, which the contract's own limits cannot reach.
		if len(keys) > maxEnabledOidcProviders {
			return fmt.Errorf("at most %d providers may be listed, not %d", maxEnabledOidcProviders, len(keys))
		}
		for _, key := range keys {
			if len(key) > maxProviderKeyLen {
				return fmt.Errorf("a provider key is at most %d characters", maxProviderKeyLen)
			}
			if strings.TrimSpace(key) == "" {
				return fmt.Errorf("a provider key cannot be blank")
			}
			// Refused rather than trimmed, because the match downstream is
			// exact: a key saved as " google" would store cleanly, report
			// success, and enable nothing — a setting that lies about having
			// been applied. Saying so is better than silently repairing it,
			// since the repaired value may not be the one they meant.
			if strings.TrimSpace(key) != key {
				return fmt.Errorf("the provider key %q carries surrounding whitespace, which would match no provider", key)
			}
		}
		return nil
	},
).AsInstallationIdentity()

// RequireSSO closes the password path: when true, an ordinary member may sign
// in only through a configured provider, and only an admin keeps the password
// form — the break-glass that stops a broken IdP from locking out the people
// who fix it (ssoenforcement.go holds the enforcement).
//
// Defined on installation_settings/update like EnabledOidcProviders, and read
// back only through the authentication_policy-gated projection, so who may sign
// in and how never rides on the aggregate every role can read.
//
// It SURVIVES A DATA RESET for the same reason its sibling does: a wipe clears
// customers and deals, never a deliberate decision about who may sign in, and
// re-opening the password path on a reset would be exactly that.
var RequireSSO = settings.Define[bool](
	"identity.require_sso",
	installationSettingsObject,
	"update",
	false,
	nil,
).AsInstallationIdentity()

// RequireMFA makes a second factor mandatory: a member without a confirmed
// authenticator is admitted only to the MFA enrolment routes until they set one
// up, the same confinement must_change_password uses for an operator-set
// password. Defined and read on the same terms as RequireSSO, and it survives a
// data reset for the same reason — a wipe clears customers, never a decision
// about how members must sign in.
var RequireMFA = settings.Define[bool](
	"identity.require_mfa",
	installationSettingsObject,
	"update",
	false,
	nil,
).AsInstallationIdentity()
