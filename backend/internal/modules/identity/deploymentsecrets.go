// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Where an earlier release sealed the two deployment-wide credentials: the
// outbound-mail password and the license token.
//
// The REF is what these entries store, never the credential. A ref is opaque
// and workspace-bound — the vault refuses one presented under another
// workspace — so a reader who somehow saw one of these rows learns that a
// credential exists and nothing whatever about it.
//
// They are product-written state, not configuration: nothing writes them now,
// and the deployment's declaration of each credential always wins. A copy
// sealed before is read only where the deployment no longer declares one, and
// `password: ${none}` deletes the relay's. So they stay settings entries.
//
// Both are marked AsInstallationIdentity, and not because a relay password is
// identity in the sense the installation's name and timezone are. It is a
// pairing with the reset: `vault_secret` is in preservedResetTables, so the
// ciphertext survives a data reset — and an unmarked ref row would not, which
// would leave every sealed deployment credential orphaned and the next boot
// without a license. The two halves have to agree, and this is the half that
// is easy to drop.

import (
	"errors"

	"github.com/margince/margince/backend/internal/platform/settings"
)

// SMTPPasswordRefKey and LicenseTokenRefKey are the storage keys.
const (
	SMTPPasswordRefKey = "installation.smtp_password_ref"
	LicenseTokenRefKey = "installation.license_token_ref"
)

// SMTPPasswordRef holds the vault ref for the outbound relay's password.
//
// Gated by installation_settings, which every seeded role may read. That is
// the right gate and not a compromise: the row holds an opaque ref, no surface
// returns it, and the fact an installation has configured outbound mail is
// already visible to anyone who can be sent a password-reset email.
var SMTPPasswordRef = settings.Define[string](
	SMTPPasswordRefKey,
	installationSettingsObject,
	"update",
	"",
	validateSecretRef,
).AsInstallationIdentity().AsSecretReference()

// LicenseTokenRef holds the vault ref for the installation's entitlement token.
//
// Gated by `license`, the object that already governs the entitlement surface.
// No seeded role holds update on it, so no principal can write this row.
var LicenseTokenRef = settings.Define[string](
	LicenseTokenRefKey,
	licenseObject,
	"update",
	"",
	validateSecretRef,
).AsInstallationIdentity().AsSecretReference()

// validateSecretRef refuses an empty ref.
//
// An empty string is how these entries read when nothing is sealed. It is a
// valid default and an invalid stored value: such a row says a credential was
// sealed and names nowhere to find it. To unseal, delete the row.
func validateSecretRef(ref string) error {
	if ref == "" {
		return errors.New("identity: a sealed credential reference cannot be empty; delete the row to unseal")
	}
	return nil
}
