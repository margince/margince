import { useState } from "react";
import { Button, Field, TextInput } from "../design-system/atoms";
import { usePasswordReveal } from "../design-system/passwordreveal";
import { Select } from "../design-system/select";
import { viewerZone } from "../format/timezone";
import {
  detectLocale,
  LOCALES,
  type Locale,
  localeNameKey,
  useT,
} from "../i18n";
import { usePageTitle, Wordmark } from "./auth";
import { AuthExperience } from "./auth-core";
import { isTooShort } from "./passwordrule";
import "./auth.css";

// The first-run claim (ADR-0105). An installation whose deployment file names no
// bootstrap_admin holds no organization, so /v1/me answers 503 and the boundary
// would otherwise render "installation not ready" — true, but a dead end. When
// the installation is instead WAITING to be claimed, this screen is what stands
// there.
//
// It is the only screen in the product that creates an account without one, and
// it says so: the operator's setup token is the authorization, and the account
// it creates is the installation's root. An interface that mints root quietly is
// not honest about what it is doing.

export type SetupStatus = { claimable: boolean };

// A first guess at the currency this installation bills in, from the region the
// claiming browser reports.
//
// It is a GUESS, and the form treats it as one: the value is shown in an
// editable field with the consequence spelled out beside it. What it replaces
// is worse than a guess — a literal "EUR" that no human ever saw, on an
// installation that may bill in none of it.
//
// The table is deliberately short. It covers the regions this product is sold
// into and answers EUR elsewhere, because a longer table would still be
// incomplete and the field is editable either way. `Intl` knows no
// region→currency mapping, so there is nothing here to delegate to.
const REGION_CURRENCY: Readonly<Record<string, string>> = {
  CH: "CHF",
  GB: "GBP",
  US: "USD",
  VN: "VND",
  AU: "AUD",
  CA: "CAD",
  JP: "JPY",
  SG: "SGD",
  PL: "PLN",
  CZ: "CZK",
  DK: "DKK",
  SE: "SEK",
  NO: "NOK",
};

function currencyForRegion(
  locale: string | undefined = globalThis.navigator?.language,
): string {
  const region = locale?.toUpperCase().split("-")[1];
  return (region && REGION_CURRENCY[region]) || "EUR";
}

/**
 * fetchSetupStatus asks whether this installation is waiting to be claimed.
 *
 * A failure is reported as "not claimable" rather than thrown: this runs only
 * on a boundary that has ALREADY decided the installation is unavailable, and
 * the honest fallback there is the availability screen the user would otherwise
 * have seen. A probe that cannot answer must not replace a true message with a
 * blank one.
 */
export async function fetchSetupStatus(): Promise<SetupStatus> {
  try {
    const response = await fetch("/setup/status", {
      headers: { Accept: "application/json" },
    });
    if (!response.ok) return { claimable: false };
    const body: unknown = await response.json();
    // Narrowed, not asserted: this body arrives from the network, and an `as`
    // here would promise the compiler a shape nothing checked. Anything that is
    // not literally `true` reads as not-claimable, which is the safe direction —
    // the cost of getting it wrong is a claim screen on an installation that
    // cannot be claimed.
    if (typeof body === "object" && body !== null && "claimable" in body) {
      return { claimable: body.claimable === true };
    }
    return { claimable: false };
  } catch {
    return { claimable: false };
  }
}

type ClaimFields = {
  organizationName: string;
  adminName: string;
  adminEmail: string;
  adminPassword: string;
  setupToken: string;
  baseCurrency: string;
  baseLanguage: Locale;
  timezone: string;
};

// What the browser can tell us about where this installation is being claimed
// from. Offered as the answer, never taken as one: each is a field the operator
// sees and can change before the claim is sent.
//
// The currency is the one worth spelling out. It used to be the literal "EUR",
// on no evidence at all, and it is the hardest of the three to correct later —
// the base currency stops being changeable once anything has converted against
// it. A guess from the browser's own region is not right either, but it is
// shown, which is the whole difference.
function claimDefaults(): ClaimFields {
  return {
    organizationName: "",
    adminName: "",
    adminEmail: "",
    adminPassword: "",
    setupToken: "",
    baseCurrency: currencyForRegion(),
    baseLanguage: detectLocale(),
    timezone: viewerZone(),
  };
}

export function SetupClaimScreen({
  onClaimed,
}: Readonly<{ onClaimed: () => void }>) {
  const t = useT();
  usePageTitle(t("setup.pageTitle"));
  // Seeded once, from the browser. A lazy initializer rather than a call in the
  // render body: re-guessing on every keystroke would overwrite what the
  // operator typed with what their laptop thinks.
  const [fields, setFields] = useState<ClaimFields>(claimDefaults);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  // The root credential is typed once with no confirm field to disagree with
  // it, so reading it back is the only check there is.
  const reveal = usePasswordReveal({
    show: t("auth.showPassword"),
    hide: t("auth.hidePassword"),
  });

  const set = (key: keyof ClaimFields) => (value: string) =>
    setFields((current) => ({ ...current, [key]: value }));

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      const response = await fetch("/setup/claim", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          setup_token: fields.setupToken.trim(),
          organization_name: fields.organizationName.trim(),
          timezone: fields.timezone.trim(),
          // Upper-cased on the way out: the server takes ISO-4217 exactly, and
          // refusing "chf" for its case would be a rule about typing rather
          // than about currencies.
          base_currency: fields.baseCurrency.trim().toUpperCase(),
          base_language: fields.baseLanguage,
          admin_email: fields.adminEmail.trim(),
          admin_name: fields.adminName.trim(),
          admin_password: fields.adminPassword,
        }),
      });
      if (response.ok) {
        onClaimed();
        return;
      }
      // Each refusal means something different to the person reading it, and
      // each implies a different next action: find the right token, sign in
      // instead, fix a field, or wait and try again. Collapsing them would
      // leave three of the four telling someone to correct a form that is
      // already correct — a 500 above all, which is not their fault and not
      // theirs to fix.
      setError(
        t(
          response.status === 401
            ? "setup.errorToken"
            : response.status === 409
              ? "setup.errorAlready"
              : response.status >= 500
                ? "setup.errorServer"
                : "setup.errorFields",
        ),
      );
    } catch {
      setError(t("setup.errorNetwork"));
    } finally {
      setSubmitting(false);
    }
  }

  const passwordShort = isTooShort(fields.adminPassword);
  // The server refuses a currency that is not three letters, so saying so here
  // keeps the operator from spending a round-trip to learn it. The check is
  // shape only: which three letters are a real currency is the server's to
  // know, and duplicating its table here would be a second answer that drifts.
  const currencyMalformed =
    fields.baseCurrency.trim() !== "" &&
    !/^[A-Za-z]{3}$/.test(fields.baseCurrency.trim());
  const complete =
    fields.setupToken.trim() !== "" &&
    fields.organizationName.trim() !== "" &&
    fields.adminName.trim() !== "" &&
    fields.adminEmail.trim() !== "" &&
    fields.timezone.trim() !== "" &&
    fields.baseCurrency.trim() !== "" &&
    !currencyMalformed &&
    !passwordShort &&
    fields.adminPassword !== "";

  return (
    <AuthExperience phase="unavailable">
      <Wordmark alt={t("auth.title")} />
      <form className="auth-card" onSubmit={submit}>
        <h1>{t("setup.title")}</h1>
        <p className="card-sub">{t("setup.body")}</p>
        {error && (
          <p className="auth-error" role="alert">
            {error}
          </p>
        )}
        <div className="auth-fields">
          <Field label={t("setup.token")} required hint={t("setup.tokenHint")}>
            {(control) => (
              <TextInput
                {...control}
                name="setup-token"
                autoComplete="off"
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                value={fields.setupToken}
                onChange={(event) => set("setupToken")(event.target.value)}
              />
            )}
          </Field>
          <Field label={t("setup.organization")} required>
            {(control) => (
              <TextInput
                {...control}
                name="organization"
                value={fields.organizationName}
                onChange={(event) =>
                  set("organizationName")(event.target.value)
                }
              />
            )}
          </Field>
          {/* What the installation is MEASURED in, asked once, here. Every one
              of these was decided silently before — the currency as a literal
              nobody saw, the zone from whichever machine happened to claim.
              They are all editable afterwards in Settings, with one exception
              the currency's own hint names. */}
          <Field
            label={t("setup.baseCurrency")}
            required
            hint={currencyMalformed ? undefined : t("setup.baseCurrencyHint")}
            error={
              currencyMalformed ? t("setup.baseCurrencyMalformed") : undefined
            }
          >
            {(control) => (
              <TextInput
                {...control}
                name="base-currency"
                autoCapitalize="characters"
                autoCorrect="off"
                spellCheck={false}
                value={fields.baseCurrency}
                onChange={(event) => set("baseCurrency")(event.target.value)}
              />
            )}
          </Field>
          <Field
            label={t("setup.baseLanguage")}
            required
            hint={t("setup.baseLanguageHint")}
          >
            {(control) => (
              <Select
                {...control}
                value={fields.baseLanguage}
                // Language names are proper nouns and untranslated, so each
                // option is in a different language from the page — `lang` is
                // WCAG 2.2 AA 3.1.2, and our locale codes are the BCP 47
                // subtags it wants.
                options={LOCALES.map((locale) => ({
                  value: locale,
                  label: t(localeNameKey(locale)),
                  lang: locale,
                }))}
                onChange={(next) => {
                  const picked = LOCALES.find((locale) => locale === next);
                  if (picked) {
                    setFields((current) => ({
                      ...current,
                      baseLanguage: picked,
                    }));
                  }
                }}
              />
            )}
          </Field>
          <Field
            label={t("setup.timezone")}
            required
            hint={t("setup.timezoneHint")}
          >
            {(control) => (
              <TextInput
                {...control}
                name="timezone"
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                value={fields.timezone}
                onChange={(event) => set("timezone")(event.target.value)}
              />
            )}
          </Field>
          <Field label={t("setup.adminName")} required>
            {(control) => (
              <TextInput
                {...control}
                name="admin-name"
                autoComplete="name"
                value={fields.adminName}
                onChange={(event) => set("adminName")(event.target.value)}
              />
            )}
          </Field>
          <Field label={t("setup.adminEmail")} required>
            {(control) => (
              <TextInput
                {...control}
                type="email"
                name="admin-email"
                autoComplete="username"
                inputMode="email"
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                value={fields.adminEmail}
                onChange={(event) => set("adminEmail")(event.target.value)}
              />
            )}
          </Field>
          <Field
            label={t("setup.adminPassword")}
            required
            error={passwordShort ? t("setup.passwordShort") : undefined}
            // The rule, until the rule is being broken — at which point the
            // refusal restates it in the danger tone and a second grey copy of
            // the same sentence underneath is noise.
            hint={passwordShort ? undefined : t("setup.passwordHint")}
            trailing={reveal.trailing}
          >
            {(control) => (
              <TextInput
                {...control}
                type={reveal.type}
                name="admin-password"
                autoComplete="new-password"
                value={fields.adminPassword}
                onChange={(event) => set("adminPassword")(event.target.value)}
              />
            )}
          </Field>
        </div>
        <p className="card-sub">{t("setup.rootWarning")}</p>
        <div className="auth-actions">
          <Button
            type="submit"
            variant="primary"
            disabled={!submitting && !complete}
            pending={submitting}
            busyLabel={t("setup.claiming")}
          >
            {t("setup.claim")}
          </Button>
        </div>
      </form>
    </AuthExperience>
  );
}
