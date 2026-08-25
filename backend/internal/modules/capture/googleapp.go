// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Where the Google OAuth app's credentials live.
//
// One app per installation, supplied by whoever operates it — Google issues the
// pair, and every mailbox a rep connects rides it. It was the process
// environment (MARGINCE_GMAIL_CLIENT_ID / _CLIENT_SECRET), which meant setting
// up Gmail took shell access to the server and a restart, and there was no way
// for the person who owns the Google project to do it themselves.
//
// The SECRET is sealed in the key vault and this row records the ref. The client
// ID is stored in the clear, deliberately: it travels in every authorization
// redirect a browser makes, so treating it as a secret would be a fiction that
// costs an operator the ability to see which app their installation is using.
//
// The environment still WORKS: an installation exporting both keeps running with
// no action, because the connect transport prefers the stored app and falls back
// to what the deployment composed. It is not migrated, though — nothing seals the
// environment's pair into the vault the way compose.SealProviderKeys does for a
// BYOK key, so those variables stay the live source until somebody stores an app.
// Writing that seeder is worth doing and is not this change.

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gradionhq/margince/backend/internal/platform/settings"
)

// GoogleAppKey is the settings key the app's credentials are stored under.
const GoogleAppKey = "capture.google_app"

// GoogleApp is the installation's Google OAuth app, as stored.
//
// A struct rather than two entries because the two halves are useless apart: a
// client id with no secret cannot refresh a token, and a secret with no id
// cannot be presented. Storing them together means a reader cannot see a
// half-configured app that no code path would accept.
type GoogleApp struct {
	// ClientID is Google's public identifier for the app.
	ClientID string `json:"client_id"`
	// ClientSecretRef addresses the sealed secret in the key vault. The secret
	// itself is never stored here and no surface returns it.
	ClientSecretRef string `json:"client_secret_ref"`
}

// Configured reports whether both halves are present, which is the only state
// the connector can act on.
func (a GoogleApp) Configured() bool {
	return a.ClientID != "" && a.ClientSecretRef != ""
}

// GoogleAppSetting holds the installation's Google app.
//
// Gated by capture_settings, the object that already governs what this
// installation captures and from where. A second object would be a distinction
// the role matrix does not make, and every object costs a backfill migration
// that has to reach installations that already exist.
//
// AsInstallationIdentity pairs it with the data reset: `vault_secret` is in
// preservedResetTables, so the sealed secret survives a reset — and an unmarked
// ref row would not, which would leave the ciphertext orphaned and the
// installation's Gmail integration broken by an operation that was supposed to
// clear tenant DATA. The two halves have to agree.
//
// AsSecretReference keeps the ref out of `audit_log`, which is admin-readable
// over /audit-log and exportable. It redacts the client id along with it, which
// is a real cost — the trail records that the app changed and not to what — and
// the alternative is a capability handle in the one place an installation hands
// out wholesale.
var GoogleAppSetting = settings.Define[GoogleApp](
	GoogleAppKey,
	captureSettingsObject,
	"update",
	GoogleApp{},
	validateGoogleApp,
).AsInstallationIdentity().AsSecretReference()

// validateGoogleApp refuses a half-configured app and a malformed client id.
//
// The empty value is the legitimate default — an installation that has not set
// one up reads as empty — so what is refused is a value that claims to be an
// app while being unusable: one half present without the other, which no code
// path accepts and which would make Configured() and the screen disagree.
func validateGoogleApp(app GoogleApp) error {
	if app == (GoogleApp{}) {
		return nil
	}
	if app.ClientID == "" {
		return errors.New("capture: a Google app needs its client id; the sealed secret alone cannot be presented to Google")
	}
	if app.ClientSecretRef == "" {
		return errors.New("capture: a Google app needs its client secret; the client id alone cannot refresh a token")
	}
	return validateClientID(app.ClientID)
}

// validateClientID checks the shape Google issues.
//
// Split out so the store can apply it BEFORE sealing a secret, without having to
// hand this function a fake ref to get past the pairing check above — a sentinel
// passed to dodge a rule is a rule the next reader cannot trust.
//
// Checked at all because a pasted value that is not a client id is almost always
// the wrong field off the same credentials screen — the project number or an API
// key — and it would otherwise fail much later, as an opaque invalid_client from
// Google on somebody's first connect attempt.
func validateClientID(clientID string) error {
	if !strings.HasSuffix(clientID, googleClientIDSuffix) {
		return fmt.Errorf("capture: %q does not look like a Google OAuth client id (they end in %s) — check you copied the Client ID and not the project number or an API key",
			clientID, googleClientIDSuffix)
	}
	return nil
}

// googleClientIDSuffix is the tail every Google OAuth client id carries.
const googleClientIDSuffix = ".apps.googleusercontent.com"
