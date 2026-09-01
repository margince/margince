// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Where a connector's OAuth app credentials live.
//
// One app per provider per installation, supplied by whoever operates it — the
// vendor issues the pair, and every mailbox or calendar a rep connects rides it.
// It was the process environment (MARGINCE_GMAIL_CLIENT_ID / _CLIENT_SECRET,
// MARGINCE_GRAPH_CLIENT_ID / _CLIENT_SECRET), which meant setting up capture
// took shell access to the server and a restart, and there was no way for the
// person who owns the Google project or the Entra registration to do it
// themselves.
//
// The SECRET is sealed in the key vault and this row records the ref. The client
// ID is stored in the clear, deliberately: it travels in every authorization
// redirect a browser makes, so treating it as a secret would be a fiction that
// costs an operator the ability to see which app their installation is using.
//
// The environment still WORKS: an installation exporting a pair keeps running
// with no action, because the connect transport prefers the stored app and falls
// back to what the deployment composed. It is not migrated, though — nothing
// seals the environment's pair into the vault the way compose.SealProviderKeys
// does for a BYOK key, so those variables stay the live source until somebody
// stores an app.
//
// Two providers, ONE set of mechanics. What differs between them is declared in
// appKinds below — the storage key, the vendor's name, the shape of a client id,
// and whether the app is pinned to a directory — so adding a third is a table
// entry rather than a second copy of the sealing, rotation and validation.

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// AppProvider names a vendor whose OAuth app this installation may hold.
//
// The connector names (`gmail`, `gcal`, `graph`, `graphcal`) are deliberately
// NOT these: one Google app backs both Gmail and Calendar, and one Entra
// registration backs both Outlook mail and Outlook calendar. Keying storage by
// connector would let one installation hold two Google projects and hand a rep
// whichever their first consent happened to reach.
type AppProvider string

const (
	// AppProviderGoogle is the Google Cloud OAuth client behind Gmail and
	// Google Calendar.
	AppProviderGoogle AppProvider = "google"
	// AppProviderMicrosoft is the Entra app registration behind Outlook mail
	// and Outlook calendar.
	AppProviderMicrosoft AppProvider = "microsoft"
)

// GoogleAppKey and MicrosoftAppKey are the settings keys each app is stored
// under. Named constants because settings.LockForWrite takes the key as a
// string, and a locked key that is not the written key serializes nothing.
const (
	GoogleAppKey    = "capture.google_app"
	MicrosoftAppKey = "capture.microsoft_app"
)

// ConnectorApp is one installation's OAuth app for one provider, as stored.
//
// A struct rather than separate entries because the halves are useless apart: a
// client id with no secret cannot refresh a token, and a secret with no id
// cannot be presented. Storing them together means a reader cannot see a
// half-configured app that no code path would accept.
type ConnectorApp struct {
	// ClientID is the vendor's public identifier for the app.
	ClientID string `json:"client_id"`
	// ClientSecretRef addresses the sealed secret in the key vault. AS STORED
	// it is a ref and never the secret, and no surface returns it.
	//
	// ConnectorAppStore.Credentials reuses this field for the UNSEALED secret,
	// which is where the caller looks for it — nothing downstream re-reads a ref
	// it was handed, and a second struct differing in one field's MEANING is how
	// a ref reaches a vendor. The rule is one-directional: a value that came out
	// of Credentials is never written back.
	ClientSecretRef string `json:"client_secret_ref"`
	// Tenant pins a Microsoft app to ONE Entra directory. Empty means the
	// app authorizes any organization, which is what a multi-tenant
	// registration is for. Google apps carry none and validation refuses one:
	// a field that silently does nothing is worse than an absent one, because
	// an operator who fills it in believes they narrowed something.
	Tenant string `json:"tenant,omitempty"`
}

// Configured reports whether both halves are present, which is the only state
// the connector can act on.
func (a ConnectorApp) Configured() bool {
	return a.ClientID != "" && a.ClientSecretRef != ""
}

// appVendor is everything that differs between one vendor's app and another's,
// apart from where it is stored.
//
// The list is the whole difference: everything else — sealing, rotation,
// retiring the superseded blob, the RBAC gate, the field ceilings — is shared.
// Separate from appKind because the settings entries are DEFINED with this
// vendor's validator, so a vendor that named its own entry would be a cycle.
type appVendor struct {
	// name is the company an operator-facing message names, so a refusal says
	// "a Microsoft app needs…" rather than naming an internal key.
	name string
	// console names where the operator copies the fields from, and appears in
	// the refusals below: "check the console" is useless to somebody who has two
	// of them open.
	console string
	// validateClientID refuses a value that is not this vendor's client id. It
	// takes the vendor so a refusal can name the console the operator is
	// copying from — "check the console" is useless to somebody who has two of
	// them open.
	validateClientID func(appVendor, string) error
	// directoryPinned reports whether Tenant means anything for this vendor.
	directoryPinned bool
	// connectors names the capture connectors this one app authorizes. One
	// Google project backs Gmail and Google Calendar; one Entra registration
	// backs Outlook mail and Outlook calendar — which is why the app is keyed
	// by VENDOR and not by connector.
	//
	// Held here because a refresh token belongs to the client that issued it:
	// replacing the client id makes every connection under these names
	// unrefreshable, and this is the list of what a replacement would strand.
	connectors []string
}

// appKind is one vendor together with the settings entry its app is stored in.
type appKind struct {
	appVendor
	entry *settings.Entry[ConnectorApp]
}

var (
	googleVendor = appVendor{
		name:             "Google",
		console:          "the Google Cloud console",
		validateClientID: appVendor.validateGoogleClientID,
		connectors:       []string{"gmail", "gcal"},
	}
	microsoftVendor = appVendor{
		name:             "Microsoft",
		console:          "the Entra app registration",
		validateClientID: appVendor.validateEntraClientID,
		directoryPinned:  true,
		connectors:       []string{"graph", "graphcal"},
	}
)

// GoogleAppSetting and MicrosoftAppSetting hold each installation's app.
//
// Gated by capture_settings, the object that already governs what this
// installation captures and from where. A second object would be a distinction
// the role matrix does not make, and every object costs a backfill migration
// that has to reach installations that already exist.
//
// AsInstallationIdentity pairs each with the data reset: `vault_secret` is in
// preservedResetTables, so the sealed secret survives a reset — and an unmarked
// ref row would not, which would leave the ciphertext orphaned and the
// installation's mail integration broken by an operation that was supposed to
// clear tenant DATA. The two halves have to agree.
//
// AsSecretReference keeps the ref out of `audit_log`, which is admin-readable
// over /audit-log and exportable. It redacts the client id along with it, which
// is a real cost — the trail records that the app changed and not to what — and
// the alternative is a capability handle in the one place an installation hands
// out wholesale.
var (
	GoogleAppSetting = settings.Define[ConnectorApp](
		GoogleAppKey, captureSettingsObject, "update", ConnectorApp{},
		googleVendor.validate,
	).AsInstallationIdentity().AsSecretReference()

	MicrosoftAppSetting = settings.Define[ConnectorApp](
		MicrosoftAppKey, captureSettingsObject, "update", ConnectorApp{},
		microsoftVendor.validate,
	).AsInstallationIdentity().AsSecretReference()
)

// appKinds is the provider table. Declared after the entries it points at, and
// read through appKindFor so an unknown provider is refused in one place.
var appKinds = map[AppProvider]appKind{
	AppProviderGoogle:    {appVendor: googleVendor, entry: GoogleAppSetting},
	AppProviderMicrosoft: {appVendor: microsoftVendor, entry: MicrosoftAppSetting},
}

// ErrUnknownAppProvider is a provider nothing in this build serves. Returned
// rather than defaulted: guessing Google for an unrecognized name would store
// somebody's Entra secret under the Google key.
var ErrUnknownAppProvider = fmt.Errorf(
	"capture: no OAuth app is held for that provider: %w", apperrors.ErrNotFound,
)

// AppProviders reads the vendor table's keys, in a stable order.
//
// Read FROM the table rather than restated beside it: a caller keeping its own
// list would go on reporting two after the table grew, and answer confidently
// while measuring less than it means to.
func AppProviders() []AppProvider {
	out := make([]AppProvider, 0, len(appKinds))
	for p := range appKinds {
		out = append(out, p)
	}
	// Sorted because Go randomizes map iteration and a caller may render this:
	// two boots of one binary must not disagree about the order.
	slices.Sort(out)
	return out
}

// ParseAppProvider reads a provider name off the wire, or refuses it.
//
// The refusal is ErrUnknownAppProvider, which the error contract maps to 404:
// the caller named a resource this build does not serve, which is a different
// thing from a malformed request. A default would be worse than either — it
// would store somebody's Entra secret under the Google key.
func ParseAppProvider(name string) (AppProvider, error) {
	p := AppProvider(name)
	if _, err := appKindFor(p); err != nil {
		return "", err
	}
	return p, nil
}

// appKindFor resolves a provider, or refuses it.
func appKindFor(p AppProvider) (appKind, error) {
	k, ok := appKinds[p]
	if !ok {
		return appKind{}, fmt.Errorf("%w: %q", ErrUnknownAppProvider, string(p))
	}
	return k, nil
}

// validate is the settings validator for one vendor's entry.
//
// The empty value is the legitimate default — an installation that has not set
// one up reads as empty — so what is refused is a value that claims to be an
// app while being unusable: one half present without the other, which no code
// path accepts and which would make Configured() and the screen disagree.
func (v appVendor) validate(app ConnectorApp) error {
	if app == (ConnectorApp{}) {
		return nil
	}
	if app.ClientID == "" {
		return fmt.Errorf("capture: a %s app needs its client id; the sealed secret alone cannot be presented", v.name)
	}
	// The same ceiling the store applies before sealing. Stated here too, on the
	// fields this validator can see: a settings write that reached the row by
	// another road would otherwise store a value the store would have refused,
	// and every read would hand it back.
	if len(app.ClientID) > maxAppFieldBytes || len(app.Tenant) > maxAppFieldBytes {
		return fmt.Errorf("capture: a %s client id and tenant are each at most %d bytes", v.name, maxAppFieldBytes)
	}
	if app.ClientSecretRef == "" {
		return fmt.Errorf("capture: a %s app needs its client secret; the client id alone cannot refresh a token", v.name)
	}
	if app.Tenant != "" {
		if !v.directoryPinned {
			return fmt.Errorf("capture: a %s app is not pinned to a directory; the tenant field does nothing here", v.name)
		}
		if err := validateEntraTenant(app.Tenant); err != nil {
			return err
		}
	}
	return v.validateClientID(v, app.ClientID)
}

// validateGoogleClientID checks the shape Google issues.
//
// Split from the pairing check so the store can apply it BEFORE sealing a
// secret, without having to hand the validator a fake ref to get past the
// pairing rule — a sentinel passed to dodge a rule is a rule the next reader
// cannot trust.
//
// Checked at all because a pasted value that is not a client id is almost always
// the wrong field off the same credentials screen — the project number or an API
// key — and it would otherwise fail much later, as an opaque invalid_client from
// Google on somebody's first connect attempt.
func (v appVendor) validateGoogleClientID(clientID string) error {
	if !strings.HasSuffix(clientID, googleClientIDSuffix) {
		return fmt.Errorf("capture: %q does not look like a Google OAuth client id (they end in %s) — check you copied the Client ID from %s, and not the project number or an API key beside it",
			clientID, googleClientIDSuffix, v.console)
	}
	return nil
}

// validateEntraClientID checks the shape Entra issues: the Application (client)
// ID is a GUID.
//
// The same mistake in Microsoft's shape: the overview blade shows the
// application id, the object id and the directory id side by side, all three
// GUIDs, and only one of them authenticates. A GUID check cannot tell those
// apart — what it does catch is the other neighbour on that screen, the display
// name, which is what an operator copies when they are reading the page rather
// than the field labels.
func (v appVendor) validateEntraClientID(clientID string) error {
	if !guid.MatchString(clientID) {
		return fmt.Errorf("capture: %q does not look like an Entra application (client) id — those are GUIDs, and this is the Application (client) ID field on %s's Overview, not the app's display name",
			clientID, v.console)
	}
	return nil
}

// validateEntraTenant checks the directory id, which is a GUID like the client
// id — with the three authority aliases refused BY NAME.
//
// `common`, `organizations` and `consumers` are legal values at Microsoft's
// endpoint and each widens the app to a population nobody here vetted. An
// operator who wants that leaves the field empty, which says the same thing
// without an alias a reader has to know to interpret.
func validateEntraTenant(tenant string) error {
	switch strings.ToLower(tenant) {
	case "common", "organizations", "consumers":
		return fmt.Errorf("capture: %q is a Microsoft authority alias, not a directory — leave the tenant empty to authorize any organization, which is what that alias means", tenant)
	}
	if !guid.MatchString(tenant) {
		return fmt.Errorf("capture: %q does not look like an Entra directory (tenant) id — those are GUIDs, from the Overview blade of the directory the app is registered in", tenant)
	}
	return nil
}

// googleClientIDSuffix is the tail every Google OAuth client id carries.
const googleClientIDSuffix = ".apps.googleusercontent.com"

// guid is the shape Microsoft issues every identifier in. Anchored, because an
// unanchored match would accept a GUID with anything pasted around it — which is
// exactly what a copy that caught a neighbouring label looks like.
var guid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
