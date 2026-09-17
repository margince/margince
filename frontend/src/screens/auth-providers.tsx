// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import {
  ProviderMark,
  providerBrandName,
} from "../design-system/provider-mark";
import { useT } from "../i18n";
import "./auth.css";

// The federated half of the sign-in surface: the provider block, the divider
// that labels what is under it, and the whole card for an installation whose
// only way in is a provider.
//
// Its own module rather than more of auth.tsx, because the two halves are
// separable in a way the screen is not: what a provider button is called and
// what pressing it does are decided by the installation and the contract, while
// the password form is this product's own form. The screen composes them.

// The installation's operational federated providers, exactly as
// /auth/capabilities serves them. `label` is the SERVER's string — the contract
// documents it as the button text, so the installation owns the wording and t()
// is not involved. Only the MARK is ours to choose, from `key`.
export type OidcProviders =
  components["schemas"]["AuthCapabilities"]["oidc_providers"];

// The product's answer to "which providers are unavailable": none. A module-level
// constant rather than an inline `new Set()` default, so every render of the
// federated block compares equal instead of allocating a fresh set.
const NO_UNAVAILABLE_PROVIDERS: ReadonlySet<string> = new Set();

/**
 * Federated sign-in, above the password form (§11).
 *
 * Placement is an argument, not a preference: if the installation runs SSO the
 * password form is the FALLBACK path, and putting it first tells every user at
 * that installation to take the slower door. Hence the divider below, which
 * labels the form rather than the buttons.
 *
 * **Renders nothing when the capability is empty**, and that is the §19
 * enforcement point rather than a convenience: `oidc_providers` is served by
 * `/auth/capabilities`, so a control for a flow this installation cannot
 * complete never reaches the screen. An installation with no Google OAuth app
 * configured (or where the deployment's state-signing key/redirect base are
 * incomplete) serves `[]`, and this component draws nothing for it — exactly
 * as an installation with a configured app draws the button. Do not "fix"
 * this to render a disabled button, and do not seed a provider list into the
 * capability response — the empty list IS the gate, and this component must
 * keep asking only "did I get providers?".
 *
 * The one thing that may put providers here without a server is
 * `app/ui-preview.ts`, and it is not an exception to the above: it substitutes
 * at the CALL SITE in `AuthScreen`, off unless `VITE_UI_PREVIEW_OIDC` is set at
 * build time, and it draws the block without making the flow work. This
 * component cannot tell the difference and must not try to.
 *
 * **The label is the server's string, not ours.** The contract types it as
 * `{ key, label }` and documents `label` as the button text, so the installation
 * owns the wording and `t()` is not involved. The consequence is real: a German
 * reader sees the installation's English label. Only the MARK is ours to choose,
 * from `key`.
 */
export function ProviderButtons({
  providers,
  disabled = false,
  passwordFormBelow = true,
  unavailable = NO_UNAVAILABLE_PROVIDERS,
  onSelect,
}: Readonly<{
  providers: OidcProviders;
  disabled?: boolean;
  /**
   * Whether the password form follows this block, which is what the divider
   * below announces. False at an installation that closed the password door.
   */
  passwordFormBelow?: boolean;
  /**
   * Provider keys to render as not-yet-available. **Empty in the product**, and
   * structurally so: the capability's items are `{ key, label }` with no
   * availability field, so nothing on the wire can populate this — only
   * `app/ui-preview.ts` can, for a design review, and §3.3 keeps a dead provider
   * control illegal on the shipped surface. This component never infers
   * availability from a key: a provider it has no logo for is still a working one.
   */
  unavailable?: ReadonlySet<string>;
  onSelect: (providerKey: string) => void;
}>) {
  const t = useT();
  if (providers.length === 0) {
    return null;
  }
  return (
    <>
      <div className="auth-sso">
        {providers.map((provider) => {
          const isUnavailable = unavailable.has(provider.key);
          return (
            <Button
              key={provider.key}
              variant="federated"
              /* The whole of what this screen still decides about the box: it is
                 an item in a VERTICAL stack, so a default flex-shrink is a
                 shrink in height. The shape — full width, unfilled, the border
                 that owes 1.4.11 its 3:1, the mark's size, the hover, the two
                 dim states — belongs to the variant, which is what the sign-in
                 surface used to redeclare for itself. */
              className="auth-social"
              /* Two unavailabilities, and Button keeps them apart because they
                 want opposite treatments: `disabled` is how the form marks every
                 provider while a sign-in is in flight — momentary, and the
                 control is coming back — while `unavailable` is a provider this
                 installation advertises with nothing behind it, which is a
                 resting state. Neither appends copy, so the accessible name
                 stays the installation's own label. */
              disabled={disabled}
              unavailable={isUnavailable}
              onClick={() => onSelect(provider.key)}
            >
              <ProviderMark providerKey={provider.key} />
              {/* Two labels, and which one SHOWS is the stylesheet's business.
                  The served label is the installation's own string and is what
                  the button is called: it is the accessible name at every width.
                  The brand word is the short form for a key we recognise, so a
                  phone can show "Google" side by side instead of wrapping
                  "Continue with Google" over three lines. An unrecognised key has
                  no brand word and falls back to the served label, which is then
                  the only text present and needs no second copy. Either way the
                  button appends nothing of its own, so the accessible name stays
                  the installation's label — including for an unavailable one. */}
              <ProviderLabel
                label={provider.label}
                providerKey={provider.key}
              />
            </Button>
          );
        })}
      </div>
      {/* Labels the path BELOW it, so a screen reader hears what the divider
          separates rather than a decorative rule — which is also why it goes
          when that path does: an installation that closed the password door has
          nothing under this rule, and a divider separating the buttons from the
          bottom of the card would announce a second way in that is not there. */}
      {passwordFormBelow && (
        <p className="auth-or">
          <span>{t("auth.orDivider")}</span>
        </p>
      )}
    </>
  );
}

/**
 * The two forms of a provider's name, with the accessible name pinned to the
 * server's.
 *
 * When a brand word exists, the served label goes into an `.sr-only` span and the
 * visible text is `aria-hidden`, so the button announces the installation's own
 * words however narrow the layout gets. When it does not, there is one span and
 * one string — a duplicate that says the same thing twice would be read twice.
 */
function ProviderLabel({
  label,
  providerKey,
}: Readonly<{ label: string; providerKey: string }>) {
  const brand = providerBrandName(providerKey);
  // The short form is used ONLY when the served label already contains it.
  // WCAG 2.2 SC 2.5.3 (Label in Name) wants the accessible name to contain the
  // visible text, and an installation is free to label its `google` provider
  // "Firmen-Login" — showing "Google" there would both break that and put a
  // brand claim on screen that the operator never made.
  if (!brand || !label.toLowerCase().includes(brand.toLowerCase())) {
    return <span>{label}</span>;
  }
  return (
    <>
      <span className="sr-only">{label}</span>
      <span aria-hidden>
        <span>{label}</span>
        <span className="auth-social-brand">{brand}</span>
      </span>
    </>
  );
}

// startFederatedSignIn is the hand-off itself: a full-page navigation to
// `/v1/auth/oidc/{provider}/start` (identity/ssologin.go), never an XHR — the
// server's redirect chain to the provider's consent screen and back has to
// carry the browser's whole address bar, not a fetch response this app could
// read. `location.assign` (not a hash route) because the target is outside
// this SPA's own address space entirely.
//
// `synthesized` is the caller's answer to "did previewedOidcProviders invent
// the button this click came from?" — NOT the global preview-build flag on
// its own. An installation that genuinely serves OIDC providers must keep
// working buttons even under a preview build; the flag alone would make
// every real provider inert the moment someone enabled it against a real
// backend. Only a button that stands in for a server with none configured
// (app/ui-preview.ts's own reason for existing) has nowhere real to go.
export function startFederatedSignIn(
  providerKey: string,
  synthesized: boolean,
): void {
  if (synthesized) {
    return;
  }
  globalThis.location.assign(`/v1/auth/oidc/${providerKey}/start`);
}

/**
 * The sign-in card at an installation that closed the password door
 * (`auth.password.enabled=false` — it signs its members in through an identity
 * provider and does not want a credential the provider never issued to open a
 * session).
 *
 * A card and not a form: there is nothing to submit from here. Every route the
 * password path would post to refuses at this installation, so rendering the
 * fields disabled — or rendering them at all — would offer a door the server
 * has closed, which is the misleading affordance the capability probe exists to
 * prevent.
 *
 * The empty case is drawn rather than left blank. A provider block can be empty
 * even here: the deployment mounts a provider before an admin stores its OAuth
 * app, and boot only guarantees a provider is MOUNTED. A card with nothing on
 * it would read as a broken page; the line says the installation signs in
 * through its provider and that the provider is not offering the flow yet,
 * which is a sentence an administrator can act on.
 */
export function FederatedOnlyCard({
  providers,
  providersSynthesized,
  unavailableProviders,
}: Readonly<{
  providers: OidcProviders;
  providersSynthesized: boolean;
  unavailableProviders: ReadonlySet<string>;
}>) {
  const t = useT();
  return (
    <div className="auth-card">
      {/* The same two lines the password card keeps in the accessibility tree
          and out of the composition: the greeting above is this page's visible
          heading. */}
      <h1 className="sr-only">{t("auth.loginTitle")}</h1>
      <p className="card-sub sr-only">{t("auth.loginSub")}</p>
      <ProviderButtons
        providers={providers}
        passwordFormBelow={false}
        unavailable={unavailableProviders}
        onSelect={(providerKey) =>
          startFederatedSignIn(providerKey, providersSynthesized)
        }
      />
      {providers.length === 0 && (
        <p className="auth-notice t-sub" role="status">
          {t("auth.noMethodOffered")}
        </p>
      )}
    </div>
  );
}
