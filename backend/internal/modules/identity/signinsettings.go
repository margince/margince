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
	"unicode/utf8"

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
			// Runes, not bytes: the contract's maxLength counts characters, and
			// the two limits must refuse the same values.
			if utf8.RuneCountInString(key) > maxProviderKeyLen {
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
// form — the break-glass that stops a broken IdP from locking out the admins
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

// The ceilings on the group→role map, mirroring the contract's maxProperties.
// Bounded for the same reason the provider list is: the map is read on the
// sign-in path, so an oversized value stored once would be paid for on every
// corporate login. 64 groups is generous — a map is one entry per role a
// directory hands out, and there are six roles.
const (
	maxGroupRoleMapEntries = 64
	maxGroupKeyLen         = 255
)

// OidcGroupRoleMap grants roles from the corporate directory: each key is a
// group exactly as the IdP spells it in the ID token's `groups` claim, each
// value the system role it grants. At sign-in the member's token groups are
// intersected with this map and every mapped role is ADDED to what they hold.
//
// GRANT-ONLY, NEVER REVOKE — the honest cost of this setting, stated in its
// contract description too. Removing a member from an IdP group does not take
// the role away here; revocation stays a deliberate admin action. And it
// admits nobody: an email with no live app_user is refused exactly as before,
// whatever groups the token carries — the map only widens what an
// already-invited member holds.
//
// It SURVIVES A DATA RESET like its siblings above: who a directory group
// makes an admin is a decision about who may do what, not customer data, and
// a wipe must not silently stop granting what an admin deliberately mapped.
//
// GATED ON authentication_policy, NOT installation_settings like its siblings —
// this is the one entry here that GRANTS ROLES, so writing it is granting them.
// The role-granting paths are admin-only on purpose (ChangeUserRole takes
// user_admin, SetRoleObjectGrant takes role_admin): a holder of a lesser editor
// must not be able to grant themselves anything the editor can express. ops
// holds installation_settings/update but only authentication_policy/READ, so
// putting the map here refuses the ops-writes-itself-admin escalation that
// installation_settings/update would have allowed. SetRawTx re-checks this
// object per field, so the rest of the installation patch keeps its own gate.
// The read stays free for the same reason its siblings' do — SignInPolicy takes
// authentication_policy/read once and then reads as the installation, and the
// login-path read is anonymous by construction.
var OidcGroupRoleMap = settings.Define[map[string]string](
	"identity.oidc_group_role_map",
	authenticationPolicyObject,
	"update",
	nil,
	validateGroupRoleMap,
).AsInstallationIdentity()

// validateGroupRoleMap holds the map's bounds and vocabulary at the write.
// The allowed values are DERIVED from systemRoles rather than restated, so a
// role added to the seeded set is mappable without touching this file — and a
// key that was never a role cannot be stored, only become stale later by a
// role being retired after the map was saved (the sign-in sync skips those).
func validateGroupRoleMap(m map[string]string) error {
	if len(m) > maxGroupRoleMapEntries {
		return fmt.Errorf("at most %d groups may be mapped, not %d", maxGroupRoleMapEntries, len(m))
	}
	for group, roleKey := range m {
		// Runes, not bytes, for the reason the provider keys count them: a
		// bound on characters must refuse the same values everywhere.
		if utf8.RuneCountInString(group) > maxGroupKeyLen {
			return fmt.Errorf("a group name is at most %d characters", maxGroupKeyLen)
		}
		if strings.TrimSpace(group) == "" {
			return fmt.Errorf("a group name cannot be blank")
		}
		// Refused rather than trimmed, exactly like a provider key: the match
		// against the token's groups claim is byte-exact, so " sales" would
		// store cleanly, report success, and grant nothing.
		if strings.TrimSpace(group) != group {
			return fmt.Errorf("the group name %q carries surrounding whitespace, which would match no token group", group)
		}
		if !isSystemRoleKey(roleKey) {
			return fmt.Errorf("%q is not a role this installation defines, so mapping %q onto it would grant nothing", roleKey, group)
		}
	}
	return nil
}

// isSystemRoleKey answers from the seeded role set (service.go) rather than a
// hand-typed copy of the keys here, which would drift the day a role is added
// or renamed.
func isSystemRoleKey(key string) bool {
	for _, role := range systemRoles {
		if role.key == key {
			return true
		}
	}
	return false
}

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
