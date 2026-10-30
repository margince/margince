import { useQuery } from "@tanstack/react-query";
import { useEffect, useId, useRef } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  Button,
  Field,
  type FieldControl,
  Modal,
  SectionHeader,
  TextInput,
} from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import {
  DrawerBody,
  DrawerFoot,
  DrawerHead,
} from "../../design-system/drawerbands";
import { Heading } from "../../design-system/heading";
import { Select } from "../../design-system/select";
import { formatNumber, identifierNumber, monthName } from "../../format/format";
import { lockFor } from "../../format/settinglocks";
import {
  LOCALES,
  type Locale,
  localeNameKey,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import { useCurrencyNote } from "../../lib/installation-settings.currency";
import { unwrap } from "../../lib/problem";
import { fiscalYearLabel } from "./fiscalyear";
import type { EditedFact, InstallationSettings } from "./installation-settings";
import { RegionalSettingsFields } from "./installation-settings.regional";

// The twelve months a fiscal year may start in, as the picker offers them.
// Derived rather than typed out, so the list cannot go short of twelve.
const MONTHS = Array.from({ length: 12 }, (_, index) => index + 1);

// Which remaining deals a projected landing is built from, in the order the
// server offers them: strictest first, then the two that soften it.
//
// Typed as the schema's own union with `satisfies`. A measure added to the
// contract and not to this list fails the typecheck, rather than going unoffered
// with no sign.
type ForwardMeasure = NonNullable<
  components["schemas"]["InstallationSettings"]["forecast_forward_measure"]
>;

const FORWARD_MEASURES = [
  "commit_evidence",
  "weighted",
  "manager_call",
] as const satisfies readonly ForwardMeasure[];

// The fiscal-year row's own answer: the month it starts, and what a report will
// then be labelled. It shows April with FY2026/27 rather than "4", because the
// admin is deciding what their reports will say. This is the only place they
// see it before saving.
//
// The example year is a parameter rather than `new Date()` read in here. The
// caller holds the clock, so a test can state which year it asserts against.
// The daily fe-clock-drift lane runs this suite 200 days ahead. With the year
// passed in, it does not fail a correct string every new year.
//
// A month outside the range one to twelve renders as itself. As with a stray
// language code, the contract and the setting both refuse it. But a row written
// straight into the settings table can hold anything. Showing the raw value says
// what is stored; a blank row hides it.
export function fiscalYearStartSummary(
  month: number,
  locale: Locale,
  exampleYear: number,
): string {
  if (!Number.isInteger(month) || month < 1 || month > 12) {
    return String(month);
  }
  return `${monthName(month, locale)} (${fiscalYearLabel(month, exampleYear)})`;
}

// The one form the three rows lead to: one dialog, one submit. The patch sends
// only the fields that changed, so a save never touches a frozen currency nobody
// edited. A field per row's Edit would promise three independent writes the
// server does not offer.
export function InstallationProfileDialog({
  settings,
  draft,
  focus,
  canManage,
  dirty,
  pending,
  refused,
  blanketError,
  describe,
  onChange,
  onClose,
  onSubmit,
}: Readonly<{
  settings: InstallationSettings;
  draft: InstallationSettings;
  /** Which field the reader pressed Edit on, and so where focus lands. */
  focus: EditedFact;
  canManage: boolean;
  dirty: boolean;
  pending: boolean;
  refused: Map<string, string>;
  blanketError: string | null;
  describe: (control: FieldControl) => string | undefined;
  onChange: (next: InstallationSettings) => void;
  onClose: () => void;
  onSubmit: () => void;
}>) {
  const t = useT();
  const currencyNote = useCurrencyNote();
  const refusalTitle = t("installationSettings.saveFailed");
  const lockOf = (fact: EditedFact) =>
    lockFor(settings.locks, `installation.${fact}`, t);
  const baseLanguage = lockOf("base_language");
  const fiscalYearStart = lockOf("fiscal_year_start_month");
  const forwardMeasure = lockOf("forecast_forward_measure");
  const { locale } = useLocale();
  const titleId = useId();
  const formId = useId();
  // Focus lands on the field whose Edit was pressed. Not `autoFocus`: the a11y
  // lint refuses it.
  const asked = useRef<HTMLInputElement>(null);
  // A `Select` takes no ref, so its trigger is found by the `data-fact` it
  // edits: a query for the role alone returns whichever combobox renders first.
  const form = useRef<HTMLFormElement>(null);
  useEffect(() => {
    const picker = form.current?.querySelector<HTMLElement>(
      `[data-fact="${focus}"] [role="combobox"]`,
    );
    if (picker) {
      picker.focus();
      return;
    }
    asked.current?.focus();
  }, [focus]);
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="drawer">
      <DrawerHead>
        <Heading size="large" id={titleId} className="t-h2">
          {t("installationSettings.companyTitle")}
        </Heading>
      </DrawerHead>
      <DrawerBody>
        <form
          id={formId}
          ref={form}
          className="form-stack"
          onSubmit={(event) => {
            event.preventDefault();
            onSubmit();
          }}
        >
          <Field
            label={t("installationSettings.timezone")}
            hint={t("installationSettings.timezoneHint")}
            error={refused.get("timezone")}
          >
            {(control) => (
              <TextInput
                {...control}
                aria-describedby={describe(control)}
                ref={focus === "timezone" ? asked : undefined}
                value={draft.timezone}
                disabled={!canManage}
                onChange={(event) =>
                  onChange({ ...draft, timezone: event.target.value })
                }
              />
            )}
          </Field>

          {/* A section inside the dialog's own heading: the currency rule needs
              the room to be explained, and level 3 is what keeps that from
              minting a second heading at the dialog's own rank. */}
          <SectionHeader
            level={3}
            title={t("installationSettings.currencyTitle")}
          />
          <Field
            label={t("installationSettings.baseCurrency")}
            hint={currencyNote(settings)}
            error={refused.get("base_currency")}
          >
            {(control) => (
              <TextInput
                {...control}
                aria-describedby={describe(control)}
                ref={focus === "base_currency" ? asked : undefined}
                value={draft.base_currency}
                disabled={!canManage || settings.base_currency_locked}
                onChange={(event) =>
                  onChange({ ...draft, base_currency: event.target.value })
                }
              />
            )}
          </Field>

          <RegionalSettingsFields
            locks={settings.locks}
            draft={draft}
            canManage={canManage}
            refused={refused}
            onChange={onChange}
          />
          <div data-fact="base_language">
            <Field
              label={t("installationSettings.baseLanguage")}
              hint={
                baseLanguage.disabled
                  ? baseLanguage.reason
                  : t("installationSettings.baseLanguageHint")
              }
              error={refused.get("base_language")}
            >
              {(control) => (
                <Select
                  {...control}
                  // `control.id` is spread through UNCHANGED: `Field` renders its
                  // label with `htmlFor` pointing at it, so replacing it would
                  // leave the combobox with no accessible name. The focus effect
                  // reads that same id back out of the DOM.
                  aria-describedby={describe(control)}
                  value={draft.base_language}
                  disabled={!canManage || baseLanguage.disabled}
                  // Language names are proper nouns and stay untranslated, so each
                  // option is in a language other than the page's. WCAG 2.2 AA
                  // 3.1.2 asks for `lang`, and our locale codes are its BCP 47 subtags.
                  options={LOCALES.map((locale) => ({
                    value: locale,
                    label: t(localeNameKey(locale)),
                    lang: locale,
                  }))}
                  // `Select` reports a string. Narrowing it back through LOCALES
                  // makes it a locale without an assertion. It also drops an
                  // answer the control was never offering.
                  onChange={(next) => {
                    const picked = LOCALES.find((locale) => locale === next);
                    if (picked) {
                      onChange({ ...draft, base_language: picked });
                    }
                  }}
                />
              )}
            </Field>
          </div>

          <div data-fact="fiscal_year_start_month">
            <Field
              label={t("installationSettings.fiscalYearStart")}
              hint={
                fiscalYearStart.disabled
                  ? fiscalYearStart.reason
                  : t("installationSettings.fiscalYearStartHint")
              }
              error={refused.get("fiscal_year_start_month")}
            >
              {(control) => (
                <Select
                  {...control}
                  aria-describedby={describe(control)}
                  value={identifierNumber(draft.fiscal_year_start_month)}
                  disabled={!canManage || fiscalYearStart.disabled}
                  // Twelve months, each labelled with what a report would then be
                  // called, such as April with FY2026/27. A bare number would make the admin work out the
                  // consequence of every option; this states it.
                  options={MONTHS.map((month) => ({
                    value: identifierNumber(month),
                    label: fiscalYearStartSummary(
                      month,
                      locale,
                      new Date().getFullYear(),
                    ),
                  }))}
                  // `Select` reports a string. Narrowed back through MONTHS rather
                  // than parsed, so an answer the control never offered cannot
                  // reach the draft.
                  onChange={(next) => {
                    const picked = MONTHS.find(
                      (month) => String(month) === next,
                    );
                    if (picked) {
                      onChange({ ...draft, fiscal_year_start_month: picked });
                    }
                  }}
                />
              )}
            </Field>
          </div>

          {draft.fiscal_year_start_month !==
            settings.fiscal_year_start_month && (
            <FiscalYearImpact enabled={canManage} />
          )}

          <div data-fact="forecast_forward_measure">
            <Field
              label={t("installationSettings.forwardMeasure")}
              hint={
                forwardMeasure.disabled
                  ? forwardMeasure.reason
                  : t("installationSettings.forwardMeasureHint")
              }
              error={refused.get("forecast_forward_measure")}
            >
              {(control) => (
                <Select
                  {...control}
                  aria-describedby={describe(control)}
                  value={draft.forecast_forward_measure}
                  disabled={!canManage || forwardMeasure.disabled}
                  options={FORWARD_MEASURES.map((measure) => ({
                    value: measure,
                    label: t(`installationSettings.forwardMeasure.${measure}`),
                  }))}
                  // Narrowed back through the offered set rather than cast. A value
                  // this build does not know never reaches the draft. So it is not
                  // saved as a measure the server would refuse on every read.
                  onChange={(next) => {
                    const picked = FORWARD_MEASURES.find(
                      (measure) => measure === next,
                    );
                    if (picked) {
                      onChange({ ...draft, forecast_forward_measure: picked });
                    }
                  }}
                />
              )}
            </Field>
          </div>

          {/* Only what no field claimed: a refusal on the input and again below
              states one problem twice, and the second is what nobody reads. */}
          {blanketError !== null ? (
            <Callout tone="danger" kind="outcome" title={refusalTitle}>
              {blanketError}
            </Callout>
          ) : null}
        </form>
      </DrawerBody>
      <DrawerFoot className="actions">
        <Button variant="ghost" type="button" onClick={onClose}>
          {t("create.cancel")}
        </Button>
        <Button
          type="submit"
          form={formId}
          variant="primary"
          disabled={!pending && (!canManage || !dirty)}
          pending={pending}
          busyLabel={t("common.saving")}
        >
          {t("installationSettings.save")}
        </Button>
      </DrawerFoot>
    </Modal>
  );
}

/**
 * What a new fiscal start does to saved work, counted before the admin saves
 * it: saved periods keep their dates, and reports that read this quarter
 * follow the new start.
 */
function FiscalYearImpact({ enabled }: Readonly<{ enabled: boolean }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const impact = useQuery({
    queryKey: ["installation-fiscal-year-impact"],
    enabled,
    queryFn: async ({ signal }) =>
      unwrap(
        await api.GET("/installation/settings/fiscal-year-impact", { signal }),
      ),
  });
  if (!enabled || impact.isError) return null;
  return (
    <Callout tone="info" title={t("installationSettings.fiscalImpactTitle")}>
      {impact.isPending ? (
        t("installationSettings.fiscalImpactLoading")
      ) : (
        <>
          <p>
            {plural(
              "installationSettings.fiscalImpactQuestions",
              impact.data.saved_questions,
              { count: formatNumber(impact.data.saved_questions, locale) },
            )}
          </p>
          <p>
            {plural(
              "installationSettings.fiscalImpactKept",
              impact.data.reports_with_kept_periods,
              {
                count: formatNumber(
                  impact.data.reports_with_kept_periods,
                  locale,
                ),
              },
            )}
          </p>
          <p>
            {plural(
              "installationSettings.fiscalImpactRelative",
              impact.data.reports_with_relative_periods,
              {
                count: formatNumber(
                  impact.data.reports_with_relative_periods,
                  locale,
                ),
              },
            )}
          </p>
        </>
      )}
    </Callout>
  );
}
