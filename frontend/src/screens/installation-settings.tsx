import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { LANGUAGE_DEPENDENT_QUERY_PREFIXES } from "../app/aicaches";
import { useCanWrite } from "../app/capability";
import { useUnsavedGuard } from "../app/unsaved";
// Shared with every upload form, which reads the ceiling off this same record.
// One query, one key: two hooks on one key would let whichever mounted first
// decide how a failure behaves for the other.
import {
  INSTALLATION_SETTINGS_KEY,
  useInstallationSettings,
} from "../app/uploadlimit";
import {
  Button,
  Field,
  type FieldControl,
  Modal,
  SectionHeader,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody } from "../design-system/panel";
import { Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { useToast } from "../design-system/toast";
import { fiscalYearLabel } from "../format/fiscalyear";
import { identifierNumber, monthName } from "../format/format";
import { LOCALES, type Locale, localeNameKey, useLocale, useT } from "../i18n";
import {
  problemFieldErrorsOf,
  problemMessageOf,
  QueryGate,
  throwProblem,
} from "./common";

// The installation settings surface (ADR-0090/A135): the organization's name,
// the IANA zone every reporting period is computed in, the ISO-4217 base
// currency every roll-up converts to, the language AI writes the shared record
// in, and the month its business year begins. Every role reads them — a rep reading amounts benefits from
// knowing which currency they are in — and only admin/ops may change them, so
// the facts are READ on the card for everyone and the verb that changes them is
// refused with a reason for everyone else. Refusing without a reason is the
// failure mode this avoids: it is indistinguishable from a bug, and a reader
// cannot act on it either way.
//
// FIVE ROWS AND ONE FORM. The card is a list of decisions — what the
// organization is called, when its periods start, which currency every amount
// is re-expressed in, which language AI writes for the whole team in, and when
// its financial year turns over — so each
// is a row that shows its own answer, which is what lets a reader audit the
// installation by travelling one column. The EDITING is one act: the server
// takes ONE sparse PATCH, so the fields are submitted together with one Save,
// and that belongs in a dialog rather than on the card (design-system README,
// `SettingList` / `SettingRow`: a control needing two inputs submitted together
// goes behind a verb, which keeps every row an answer). Each row's Edit opens
// that one dialog with its own field focused, so the verb beside a fact leads
// to the fact.
//
// The base currency carries a state the others do not: it stops being
// changeable once a conversion rate has been frozen against it — by a closed
// deal, a sent offer, a mirrored invoice, a contract, a commission entry or a
// loaded rate sheet (ADR-0085 §7). The server reports that as a flag and a
// reason, so the row and the field both carry the reason — an operator learns
// why before typing a value they cannot save, rather than discovering it from
// a 422. The base language never freezes: changing it re-means nothing already
// written.

// Both shapes come from the generated contract rather than being restated
// here: a hand-written copy would drift the first time the contract gains a
// field, and drift silently, since nothing compares the two.
type InstallationSettings = components["schemas"]["InstallationSettings"];
type Patch = components["schemas"]["UpdateInstallationSettingsRequest"];

// The facts this form writes, in the order the rows show them. ONE list: the
// dirty check, the sparse patch and the re-seed signature all walk it, so a
// field added to the form cannot be left out of one of the three — which is
// how a value silently stops saving, or stops noticing another admin's change.
const EDITABLE_FACTS = [
  "name",
  "timezone",
  "base_currency",
  "base_language",
  "fiscal_year_start_month",
] as const;

// Which fact the reader pressed Edit on. The dialog always edits all of them —
// one record, one sparse PATCH, one Save — so this decides nothing about what
// is written, only where focus lands when the dialog opens.
type EditedFact = (typeof EDITABLE_FACTS)[number];

// Exported for the onboarding's preferences act, which writes the same sparse
// patch from the same fields: a second mutation would be a second answer to
// what a saved language invalidates.
export function useUpdateInstallationSettings(onSaved: () => void) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (patch: Patch) => {
      const { data, error } = await api.PATCH("/installation/settings", {
        body: patch,
      });
      if (error) {
        // `throwProblem`, not `new Error(problemMessage(...))`. The wrapped form
        // flattened the server's answer to one sentence, which discarded
        // `details.errors[]` — so the per-field assertions the API already sends
        // on a 422 were unreachable, and every refusal on this three-field form
        // arrived as one paragraph at the bottom that named no field. It also
        // stopped being a ProblemError, which is what the global failure sink
        // uses to tell a server refusal from a bug worth logging.
        throwProblem(error, t);
      }
      return data;
    },
    onSuccess: (data) => {
      queryClient.setQueryData(INSTALLATION_SETTINGS_KEY, data);
      // The model-written surfaces are keyed on a record id and nothing else,
      // and the server rewrites them in the new language the next time it is
      // asked. Without this nobody asks: `refetchOnWindowFocus` is off, so a
      // company page left open keeps rendering its old-language brief until
      // some unrelated fact moves it. The setting would look like it did
      // nothing, which is the complaint base language exists to answer.
      //
      // Invalidated unconditionally rather than only when base_language moved.
      // Deciding that means diffing against the pre-patch value, and the cost
      // of being wrong is asymmetric: an extra refetch of one open page is
      // cheap, and a missed one is a reader quietly served the wrong language.
      for (const prefix of LANGUAGE_DEPENDENT_QUERY_PREFIXES) {
        void queryClient.invalidateQueries({ queryKey: prefix });
      }
      onSaved();
    },
    // A refused patch can still have committed nothing OR something: the
    // server applies the fields in one transaction, but a validation refusal
    // on one field is reported after others were accepted in an earlier
    // request. Refetching on failure means the form shows what the server
    // actually holds rather than the draft the user typed.
    onError: () => {
      void queryClient.invalidateQueries({
        queryKey: ["installation-settings"],
      });
    },
  });
}

export function InstallationSettingsCard() {
  const t = useT();
  const canManage = useCanWrite("installation_settings", "update");
  const query = useInstallationSettings();

  return (
    <QueryGate query={query} pendingLabel={t("nav.settings")}>
      {(settings) => (
        <InstallationSettingsForm settings={settings} canManage={canManage} />
      )}
    </QueryGate>
  );
}

// The base language, rendered as the language's own name.
//
// The endonyms come from the i18n catalog rather than a list here, because the
// personal language switcher already draws from it — two lists would disagree
// the day the product speaks a fourth language, and the settings page would be
// the one still offering three.
//
// A code the UI does not ship falls back to itself. That is reachable: the
// contract admits en/de/vi, but an installation whose row predates this build
// (or was written straight into the settings table) can hold anything, and a
// row that renders the raw code is honest where one that renders blank is not.
function languageName(code: string, t: ReturnType<typeof useT>): string {
  const known = LOCALES.find((locale) => locale === code);
  return known ? t(localeNameKey(known)) : code;
}

// The fiscal-year row's own answer: the month it starts, and what a report will
// then be labelled. "April — FY2026/27" rather than "4", because the number
// answers a question nobody asked; the admin is deciding what their reports
// will SAY, and this is the only place they see it before saving.
//
// The example year is a PARAMETER rather than `new Date()` read in here. The
// caller holds the clock, so a test can state which year it is asserting
// against — and the daily fe-clock-drift lane, which runs this suite 200 days
// ahead, does not turn a correct string into a failure every new year.
//
// A month outside 1..12 renders as itself. Reachable the same way a stray
// language code is: the contract and the setting both refuse it, but a row
// written straight into the settings table can hold anything, and a row showing
// the raw value is honest where a blank one is not.
// The twelve months a fiscal year may start in, as the picker offers them.
// Derived rather than typed out, so the list cannot go short of twelve.
const MONTHS = Array.from({ length: 12 }, (_, index) => index + 1);

export function fiscalYearStartSummary(
  month: number,
  locale: Locale,
  exampleYear: number,
): string {
  if (!Number.isInteger(month) || month < 1 || month > 12) {
    return String(month);
  }
  return `${monthName(month, locale)} — ${fiscalYearLabel(month, exampleYear)}`;
}

// A frozen currency is frozen for an admin too, so the lock reason replaces the
// advice about what to type — that advice is about a value nobody can set. The
// row and the field inside the dialog say the same thing, from one expression,
// because two copies of this rule would answer differently the day the server
// stops sending a reason.
export function currencyNote(
  settings: InstallationSettings,
  t: ReturnType<typeof useT>,
): string {
  if (!settings.base_currency_locked) {
    return t("installationSettings.baseCurrencyHint");
  }
  return (
    settings.base_currency_locked_reason ??
    t("installationSettings.baseCurrencyLocked")
  );
}

function InstallationSettingsForm({
  settings,
  canManage,
}: {
  settings: InstallationSettings;
  canManage: boolean;
}) {
  const t = useT();
  const { locale } = useLocale();
  const toast = useToast();
  const [editing, setEditing] = useState<EditedFact | null>(null);
  // The save's only visible answer was the button going disabled, because the
  // draft now matched the server. A control losing its affordance reads as the
  // form having given up, not as the write having landed — and on this form the
  // patch is SPARSE, so an operator who changed one field of three had no way to
  // tell which of them the installation now holds.
  const update = useUpdateInstallationSettings(() => {
    toast.show(t("settings.saved"));
    setEditing(null);
  });
  const [draft, setDraft] = useState(settings);
  const seeded = useRef(serverSignature(settings));

  // Re-seed when the server answers with DIFFERENT values than the draft was
  // built from — another admin's change arriving — and only then.
  //
  // The identity of `settings` is not that question. Every refetch mints a new
  // object, and `refetchOnWindowFocus` means a reader who tabs away to look up
  // their IANA zone and tabs back triggers one: the values come back
  // unchanged, the object does not, and the effect used to throw away
  // everything they had typed. Comparing what the server SAID, rather than
  // which object said it, leaves an unsaved draft alone across every refetch
  // that changes nothing.
  useEffect(() => {
    const signature = serverSignature(settings);
    if (seeded.current === signature) {
      return;
    }
    seeded.current = signature;
    setDraft(settings);
  }, [settings]);

  // The denial, said once and POINTED AT, from every control it refuses. It
  // refuses three Edit verbs on the card and — if the grant is withdrawn while
  // the dialog is open, which /me's focus refetch can do — the three fields
  // inside it. `Button`'s `reasonId` and a field's `aria-describedby` are the
  // same wiring: name the sentence once, point every refused control at it.
  // Printing it beside each of them would state one fact three times.
  const denialId = useId();
  const describe = (control: FieldControl): string | undefined =>
    canManage
      ? control["aria-describedby"]
      : [control["aria-describedby"], denialId].filter(Boolean).join(" ");

  // The server's per-field assertions, put on the FIELDS. A 422 on this form
  // names which of the three values it refused — an unknown IANA zone, a
  // currency that is not a currency — and until now all of that arrived as one
  // paragraph under the last field, so a reader had to guess which input to fix.
  // `Field`'s `error` slot has existed the whole time; nothing routed anything
  // into it, on any settings page.
  //
  // Keyed by the wire field name because that is what the server asserts. The
  // patch is sparse and built from the same names, so the two cannot drift
  // without the request itself being wrong.
  const refused = new Map(
    problemFieldErrorsOf(update.error).map((problem) => [
      problem.field,
      problem.message,
    ]),
  );

  const dirty = EDITABLE_FACTS.some((fact) => draft[fact] !== settings[fact]);
  // The claim the Save button's own condition was already making privately.
  // It outlives the dialog on purpose: dismissing the dialog does not destroy
  // what was typed (the draft is still here, and reopening Edit shows it), so
  // an unsaved edit is still an unsaved edit on the way out of the page.
  useUnsavedGuard(dirty);

  // Only changed fields are sent: the patch is sparse, and sending an
  // unchanged base currency would ask the server to write a value that may be
  // frozen — refused, for a field the operator never touched.
  const submit = () => {
    // Walks EDITABLE_FACTS rather than naming the fields again: the wire names
    // and the draft's keys are the same names, so a fact added to that list is
    // sent by this without a second edit. Enumerating them here is how a field
    // ends up dirty-checked, drawn, and never actually submitted.
    const patch: Patch = {};
    for (const fact of EDITABLE_FACTS) {
      if (draft[fact] !== settings[fact]) {
        Object.assign(patch, { [fact]: draft[fact] });
      }
    }
    update.mutate(patch);
  };

  const editVerb = (fact: EditedFact, field: string) => (
    <Button
      small
      variant="ghost"
      // Named by the fact it changes, not "Edit": three rows offering three
      // identically-named buttons make a screen reader's user count them.
      aria-label={t("installationSettings.editField", { field })}
      reasonId={canManage ? undefined : denialId}
      onClick={() => setEditing(fact)}
    >
      {t("installationSettings.edit")}
    </Button>
  );

  return (
    <Panel title={t("installationSettings.orgTitle")}>
      <PanelBody>
        <p className="settings-panel-sub">{t("installationSettings.orgSub")}</p>
        {!canManage && (
          <p className="t-caption" id={denialId}>
            {t("installationSettings.readOnly")}
          </p>
        )}
        <SettingList>
          <SettingRow
            label={t("installationSettings.name")}
            description={t("installationSettings.nameHint")}
            value={settings.name}
            control={editVerb("name", t("installationSettings.name"))}
          />
          <SettingRow
            label={t("installationSettings.timezone")}
            description={t("installationSettings.timezoneHint")}
            value={settings.timezone}
            control={editVerb("timezone", t("installationSettings.timezone"))}
          />
          <SettingRow
            label={t("installationSettings.baseCurrency")}
            description={currencyNote(settings, t)}
            value={settings.base_currency}
            control={editVerb(
              "base_currency",
              t("installationSettings.baseCurrency"),
            )}
          />
          <SettingRow
            label={t("installationSettings.baseLanguage")}
            description={t("installationSettings.baseLanguageHint")}
            // The language's own name, not its code: "Deutsch" is what an
            // operator recognises, and `en` is what the wire carries.
            value={languageName(settings.base_language, t)}
            control={editVerb(
              "base_language",
              t("installationSettings.baseLanguage"),
            )}
          />
          <SettingRow
            label={t("installationSettings.fiscalYearStart")}
            description={t("installationSettings.fiscalYearStartHint")}
            // The month's name and the span it produces, because the number
            // alone answers the wrong question: an admin is deciding what a
            // report will SAY, and "April — FY2026/27" shows that where "4"
            // makes them work it out.
            value={fiscalYearStartSummary(
              settings.fiscal_year_start_month,
              locale,
              new Date().getFullYear(),
            )}
            control={editVerb(
              "fiscal_year_start_month",
              t("installationSettings.fiscalYearStart"),
            )}
          />
        </SettingList>
        {editing !== null && (
          <InstallationProfileDialog
            settings={settings}
            draft={draft}
            focus={editing}
            canManage={canManage}
            dirty={dirty}
            pending={update.isPending}
            refused={refused}
            blanketError={
              update.isError && refused.size === 0
                ? problemMessageOf(update.error, t)
                : null
            }
            describe={describe}
            onChange={setDraft}
            onClose={() => setEditing(null)}
            onSubmit={submit}
          />
        )}
      </PanelBody>
    </Panel>
  );
}

// The one form the three rows lead to: ONE dialog, ONE submit — the patch
// already sends only the fields that changed, which is what keeps a save from
// touching a frozen currency nobody edited. A field per row's Edit would
// promise three independent writes the server does not offer.
function InstallationProfileDialog({
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
  const { locale } = useLocale();
  const titleId = useId();
  // Focus lands on the field whose Edit was pressed — programmatic rather than
  // the `autoFocus` attribute, the same way the sign-in page does it, so the
  // a11y lint's blanket rule against autofocus stays intact.
  const asked = useRef<HTMLInputElement>(null);
  // Two of the fields are a `Select` — a button and a portalled listbox, not an
  // input — so neither takes the ref the text fields share. Their triggers are
  // found through the form, which keeps the same promise the text fields keep:
  // the verb beside a fact leads to the fact.
  //
  // Found by the fact each one EDITS, not by `[role="combobox"]`. There are two
  // comboboxes here now, and a query for the role returns whichever the form
  // renders first — so pressing Edit beside the fiscal year focused the
  // language instead, and a third Select would have moved that again silently.
  //
  // `Field` generates its control id with `useId`, which is a React handle
  // (`:r3:`) and carries no fact name, so the marker is written at the call
  // site instead. Explicit rather than inferred: a selector guessing at a
  // generated id would break the day React changes the format, and nothing
  // would fail except the focus nobody tests by hand.
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
    <Modal open onClose={onClose} labelledBy={titleId}>
      <h2 id={titleId} className="t-h2 modal-title">
        {t("installationSettings.orgTitle")}
      </h2>
      <form
        ref={form}
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          onSubmit();
        }}
      >
        <Field
          label={t("installationSettings.name")}
          hint={t("installationSettings.nameHint")}
          error={refused.get("name")}
        >
          {(control) => (
            <TextInput
              {...control}
              aria-describedby={describe(control)}
              ref={focus === "name" ? asked : undefined}
              value={draft.name}
              disabled={!canManage}
              onChange={(event) =>
                onChange({ ...draft, name: event.target.value })
              }
            />
          )}
        </Field>
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

        {/* A section INSIDE the dialog's own heading: the currency rule needs
            the room to be explained, and level 3 is what keeps that from
            minting a second heading at the dialog's own rank. */}
        <SectionHeader
          level={3}
          title={t("installationSettings.currencyTitle")}
          sub={t("installationSettings.currencySub")}
        />
        <Field
          label={t("installationSettings.baseCurrency")}
          hint={currencyNote(settings, t)}
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

        <div data-fact="base_language">
          <Field
            label={t("installationSettings.baseLanguage")}
            hint={t("installationSettings.baseLanguageHint")}
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
                disabled={!canManage}
                // Language names are proper nouns and deliberately untranslated,
                // so every option is in a different language from the page around
                // it — `lang` is WCAG 2.2 AA 3.1.2, and our locale codes are the
                // BCP 47 subtags it wants.
                options={LOCALES.map((locale) => ({
                  value: locale,
                  label: t(localeNameKey(locale)),
                  lang: locale,
                }))}
                // `Select` reports a string; narrowing it back through LOCALES is
                // what makes it a locale without an assertion, and drops an
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
            hint={t("installationSettings.fiscalYearStartHint")}
            error={refused.get("fiscal_year_start_month")}
          >
            {(control) => (
              <Select
                {...control}
                aria-describedby={describe(control)}
                value={identifierNumber(draft.fiscal_year_start_month)}
                disabled={!canManage}
                // Twelve months, each labelled with what a report would then be
                // called — "April — FY2026/27". A bare number would make the
                // admin work out the consequence of every option; this states it.
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
                  const picked = MONTHS.find((month) => String(month) === next);
                  if (picked) {
                    onChange({ ...draft, fiscal_year_start_month: picked });
                  }
                }}
              />
            )}
          </Field>
        </div>

        {/* Only what no field claimed. A refusal shown BOTH on the input and
            again in a paragraph below states one problem twice, and the
            paragraph is the copy a reader stops reading. */}
        {blanketError !== null ? (
          <Callout tone="danger" live="alert">
            {blanketError}
          </Callout>
        ) : null}
        <div className="form-actions">
          <Button small variant="ghost" type="button" onClick={onClose}>
            {t("create.cancel")}
          </Button>
          <Button
            small
            type="submit"
            variant="primary"
            disabled={!pending && (!canManage || !dirty)}
            pending={pending}
            busyLabel={t("common.saving")}
          >
            {t("installationSettings.save")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

// What the server SAID, as one comparable string. Used to tell a refetch that
// changed nothing from one that did — see the re-seed effect above.
//
// Built from EDITABLE_FACTS rather than a list of its own: a field added to the
// form and forgotten here would leave another admin's change to it invisible,
// and the draft would keep overwriting it with a stale value on every save.
function serverSignature(settings: InstallationSettings): string {
  return JSON.stringify(EDITABLE_FACTS.map((fact) => settings[fact]));
}
