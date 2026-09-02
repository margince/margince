// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The installation's own settings (ADR-0090/A135). Identity owns them because
// it owns the installation: it is the module that bootstraps the singleton
// organization and resolves it on every boot (ADR-0061 §3).
//
// These moved off columns on the `workspace` row. Two of them were never
// reachable by a human at all — an installation that mistyped its base
// currency or timezone in margince.yaml on day one had no way to correct it
// through the product, which is the gap ADR-0085 §7 names.

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// installationSettingsObject is the RBAC object gating the installation
// settings surface. Read is broad — a rep seeing amounts in the base currency
// benefits from knowing which one it is — and write is admin/ops.
const installationSettingsObject = "installation_settings"

// SettingsObject is the same object, for compose.
//
// Exported because the installation-setup surface takes this gate itself: its
// answer must not depend on which stores happen to be composed, so the check
// lives in the transport rather than in whichever store answers first. The
// unexported spelling stays the one this package uses, so there is one value
// rather than two that happen to agree.
const SettingsObject = installationSettingsObject

// Name is the organization's display name. Seeded from margince.yaml at
// bootstrap; the row is authoritative afterwards, so renaming the
// organization does not require a redeployment.
var Name = settings.Define[string](
	"installation.name",
	installationSettingsObject,
	"update",
	"",
	func(v string) error {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("the organization needs a name")
		}
		return nil
	},
).AsInstallationIdentity()

// Timezone is the IANA reporting-period zone every period boundary is
// computed in. Distinct from a user's own timezone (app_user.timezone), which
// only affects how times are displayed to them.
var Timezone = settings.Define[string](
	"installation.timezone",
	installationSettingsObject,
	"update",
	"UTC",
	func(v string) error {
		// LoadLocation accepts two values that are not IANA zones and would
		// silently change what a reporting period means: "" resolves to UTC,
		// and "Local" resolves to whatever zone the SERVER happens to run in —
		// so the same installation would compute different period boundaries
		// on different hosts. Both are refused before the lookup.
		if v == "" || v == "Local" {
			return fmt.Errorf("%q is not an IANA zone name — use one like Europe/Berlin", v)
		}
		// Then validated by loading it: the tzdata the server actually has is
		// the only authority on whether a name resolves at report time. A name
		// that passes a regex and fails at midnight is worse than one refused
		// here.
		if _, err := time.LoadLocation(v); err != nil {
			return fmt.Errorf("%q is not an IANA timezone this server knows", v)
		}
		return nil
	},
).AsInstallationIdentity()

// BaseCurrency is the ISO-4217 currency every money roll-up converts to.
//
// It freezes once a deal has frozen a conversion rate against it (ADR-0085
// §7). Before that point it is freely changeable — which is the case this
// serves: an installation that chose wrong in its configuration on day one and
// noticed in week one. After it, changing the base would silently re-mean
// every historical roll-up.
//
// Identity declares the entry because it owns the installation, but it does
// NOT own the freeze predicate: what makes a conversion rate "frozen" is the
// deals module's business, and identity may not read its tables. Compose
// injects the probe, the way every cross-module edge is wired (ADR-0054).
// Until it does, this setting is changeable — which is why the injection is
// asserted by a fitness test rather than left to wiring discipline.
var BaseCurrency = settings.Define[string](
	"installation.base_currency",
	installationSettingsObject,
	"update",
	"EUR",
	func(v string) error {
		if !values.ValidCurrency(v) {
			return fmt.Errorf("a base currency is three uppercase ISO-4217 letters, like EUR")
		}
		return nil
	},
).AsInstallationIdentity()

// BaseLanguage is the language AI writes in when what it writes is read by the
// whole team rather than by one person.
//
// A model asked nothing about language answers in whatever language its input
// happened to be in, so a Vietnamese thread produced a Vietnamese claim on a
// record a German colleague then had to read. The installation names one
// language for that shared writing, the way it names one currency for money.
//
// It does NOT govern everything a model writes. Correspondence keeps the
// language of the correspondence — a German thread gets a German reply however
// this is set — and a brief cached for one reader keeps that reader's language.
// This is the language of the shared record.
//
// No freeze, unlike BaseCurrency. Changing it re-means nothing already stored:
// old artifacts stay in the language they were written in, and nothing converts
// against the answer the way money does.
var BaseLanguage = settings.Define[string](
	"installation.base_language",
	installationSettingsObject,
	"update",
	string(textlang.English),
	func(v string) error {
		if !textlang.Known(v) {
			return fmt.Errorf("a base language is one of en, de, vi")
		}
		return nil
	},
).AsInstallationIdentity()

// FiscalYearStartMonth is the month the installation's business year begins,
// 1..12. January is the default, which is what every installation reported by
// before this existed — so an installation that never touches it sees no
// change, and no saved report view moves under it.
//
// It buckets reports on READ and stores nothing, so changing it re-labels every
// period report immediately and re-means no stored row. That is the opposite of
// BaseCurrency, which freezes: a fiscal year is a way of cutting time, not a
// value anything has already converted against.
//
// It does re-point a SAVED report view, which is a real gap rather than a
// property of this setting: a period bucket's text travels out in a derivation
// handle and binds back as an equality filter (reportperiod.go), so a view
// saved under one fiscal start names a different span after it moves. Tracked
// as its own decision — re-point, invalidate, or warn — rather than settled
// here, because none of the three is obviously right and the label cannot fix
// it either way.
//
// What the label DOES fix is the reader's half: spelling both years means the
// answer they get is unambiguous about which twelve months it covers, even when
// the filter that produced it is stale.
var FiscalYearStartMonth = settings.Define[int](
	"installation.fiscal_year_start_month",
	installationSettingsObject,
	"update",
	int(time.January),
	func(month int) error {
		if month < int(time.January) || month > int(time.December) {
			return fmt.Errorf("a fiscal year starts in month 1..12, not %d", month)
		}
		return nil
	},
).AsInstallationIdentity()

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

// Definitions is identity's contribution to the settings registry.
func Definitions() []settings.Definition {
	return []settings.Definition{
		Name,
		Timezone,
		BaseCurrency,
		BaseLanguage,
		FiscalYearStartMonth,
		EnabledOidcProviders,
		SMTPPasswordRef,
		LicenseTokenRef,
	}
}

// BaseCurrencyOf resolves the installation's reporting currency inside a
// transaction the caller already holds.
//
// It lives here because identity OWNS the setting: the modules that convert
// money may not import this package, so compose injects this function into
// them (ADR-0054) — but the one spelling of "how the base currency is read"
// belongs with the entry that declares it, not copied into each wiring site.
//
// RequireTx rather than Get: an absent row refuses instead of reading as the
// registered default, because every caller of this is converting or freezing
// money against the answer.
func BaseCurrencyOf(ctx context.Context, tx pgx.Tx) (string, error) {
	return settings.RequireTx(ctx, tx, BaseCurrency)
}

// TimezoneOf resolves the installation's IANA zone inside a transaction the
// caller already holds — the zone a "today" is computed in.
//
// RequireTx, like BaseCurrencyOf: a close-date sweep or a forecast cutoff that
// silently fell back to UTC would move real dates for an installation that
// runs in Europe/Berlin, and would move them by a day only sometimes, which is
// the hardest kind of wrong to notice.
func TimezoneOf(ctx context.Context, tx pgx.Tx) (string, error) {
	return settings.RequireTx(ctx, tx, Timezone)
}

// NameOf resolves the installation's display name inside a transaction the
// caller already holds.
//
// RequireTx here too, though the name is display rather than arithmetic: an
// offer snapshot names its issuer, and an offer that went out identifying the
// installation as "" is not better than one that refused to go out. The three
// installation-identity settings are seeded together at bootstrap, so a tree
// where one is unset has the other two unset as well.
func NameOf(ctx context.Context, tx pgx.Tx) (string, error) {
	return settings.RequireTx(ctx, tx, Name)
}

// BaseLanguageOf resolves the language shared AI writing is written in, inside
// a transaction the caller already holds.
//
// GetTx rather than RequireTx, which is the opposite choice from the three
// above, and the reason is the upgrade rather than the value: every
// installation bootstrapped before this setting existed has no row for it. The
// three others are seeded together at bootstrap, so an absent row there means a
// broken installation and refusing is right. Here an absent row means an older
// one, and a brief that refuses to generate because nobody has named a language
// is worse than one that comes out in English — which is what those
// installations get today anyway.
func BaseLanguageOf(ctx context.Context, tx pgx.Tx) (string, error) {
	return settings.GetTx(ctx, tx, BaseLanguage)
}

// FiscalYearStartMonthOf resolves the month the installation's business year
// begins, inside a transaction the caller already holds.
//
// GetTx for the same reason BaseLanguageOf uses it: an installation
// bootstrapped before this setting existed carries no row, and the default is
// January — the calendar year those installations have always reported by. So
// an absent row is not a broken installation, it is an older one that agrees
// with the default.
func FiscalYearStartMonthOf(ctx context.Context, tx pgx.Tx) (int, error) {
	return settings.GetTx(ctx, tx, FiscalYearStartMonth)
}

// BaseLanguageForPrompt resolves the base language for a caller that holds a
// POOL rather than a transaction, opening the workspace transaction itself.
//
// It sits beside BaseLanguageOf rather than in either caller because both a
// compose engine and the deal-status service need exactly this, and the six
// lines are identical either way — two copies of one settings read is how one
// question comes to have two answers that drift.
//
// It never fails the caller. A prompt is being built, and the language is the
// least important thing in it: refusing to extract a meeting's next steps
// because a settings read timed out trades a whole feature for a formatting
// preference. On any error the answer is English, which is what these prompts
// produced before the setting existed.
//
// The failure IS logged, and it has to be: this returns a string and nothing
// else, so a caller has no way to notice a degraded resolve and say so itself.
// A missing row does NOT reach that line — BaseLanguageOf answers the
// registered default for one — so a log here always means something actually
// went wrong.
func BaseLanguageForPrompt(ctx context.Context, pool *pgxpool.Pool) string {
	lang := string(textlang.English)
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		resolved, err := BaseLanguageOf(ctx, tx)
		if err != nil {
			return err
		}
		lang = resolved
		return nil
	})
	if err != nil {
		slog.WarnContext(ctx, "the installation's base language could not be read; this prompt asks for English",
			"reason", err)
		return string(textlang.English)
	}
	return lang
}

// InstallationNameOf reads the installation's own display label inside a
// transaction the caller already holds.
//
// The setting row is read directly rather than through platform/settings:
// this answers surfaces that run with no principal to judge the
// installation_settings object gate — the login that has not built one
// yet, and the public preference page, which has no seat at all. The name
// is the installation's own label, not tenant data.
//
// Coalesced to the empty string rather than an error, for the same reason
// the login does it: this is a display label, and an installation with no
// stored name is a misconfiguration that must not turn a working page
// into a 500.
func InstallationNameOf(ctx context.Context, tx pgx.Tx) (string, error) {
	var name string
	err := tx.QueryRow(ctx,
		`SELECT coalesce((SELECT value #>> '{}' FROM setting WHERE key = $1), '')`, Name.Key(),
	).Scan(&name)
	return name, err
}

// InstallationNameForPublicPage answers the installation's label for a
// surface holding only a pool — the public preference centre's seam.
func InstallationNameForPublicPage(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	var name string
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		name, err = InstallationNameOf(ctx, tx)
		return err
	})
	return name, err
}
