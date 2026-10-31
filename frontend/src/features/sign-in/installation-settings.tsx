import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useRef, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useUnsavedGuard } from "../../components/unsaved";
import { Button, type FieldControl } from "../../design-system/atoms";
import { describedByIds } from "../../design-system/describedby";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { useToast } from "../../design-system/toast";
import { lockFor } from "../../format/settinglocks";
import { LOCALES, localeNameKey, useLocale, useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
// Installation facts are readable by every role; only admin/ops may change them.
// Each row opens the shared sparse-patch form with its own field focused.
import { useCurrencyNote } from "../../lib/installation-settings.currency";
import {
  problemFieldErrorsOf,
  problemMessageOf,
  unwrap,
} from "../../lib/problem";
// Shared with every upload form, which reads the ceiling off this same record.
// One query, one key: two hooks on one key would let whichever mounted first
// decide how a failure behaves for the other.
import {
  INSTALLATION_SETTINGS_KEY,
  useInstallationSettings,
} from "../../lib/uploadlimit";
import {
  LANGUAGE_DEPENDENT_QUERY_PREFIXES,
  SETTINGS_IN_SERVER_TEXT,
} from "./aicaches";
import { CallingSettingsRows } from "./installation-settings.calling";
import {
  ForecastSelfCorrectingRows,
  IndividualPerformanceRows,
} from "./installation-settings.individual";
import {
  fiscalYearStartSummary,
  InstallationProfileDialog,
} from "./installation-settings.profile";
import { RegionalSettingsRows } from "./installation-settings.regional";

export { fiscalYearStartSummary } from "./installation-settings.profile";

// Both shapes come from the generated contract rather than being restated
// here: a hand-written copy would drift the first time the contract gains a
// field, and drift silently, since nothing compares the two.
export type InstallationSettings =
  components["schemas"]["InstallationSettings"];

type Patch = components["schemas"]["UpdateInstallationSettingsRequest"];

// The facts this form writes, in the order the rows show them. ONE list: the
// dirty check, the sparse patch and the re-seed signature all walk it, so a
// field added to the form cannot be left out of one of the three — which is
// how a value silently stops saving, or stops noticing another admin's change.
const EDITABLE_FACTS = [
  "timezone",
  "base_currency",
  "base_language",
  "date_format",
  "time_format",
  "fiscal_year_start_month",
  "forecast_forward_measure",
] as const;

// Which fact the reader pressed Edit on. The dialog always edits all of them —
// one record, one sparse PATCH, one Save — so this decides nothing about what
// is written, only where focus lands when the dialog opens.
export type EditedFact = (typeof EDITABLE_FACTS)[number];

// Exported for the onboarding's preferences act, which writes the same sparse
// patch from the same fields: a second mutation would be a second answer to
// what a saved language invalidates.
export function useUpdateInstallationSettings(onSaved?: () => void) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (patch: Patch) => {
      // Thrown as a ProblemError, so a 422's `details.errors[]` lands on the field
      // it names and the failure sink does not log a refusal as a bug.
      const data = unwrap(
        await api.PATCH("/installation/settings", {
          body: patch,
        }),
        t,
      );
      return data;
    },
    onSuccess: (data, patch) => {
      queryClient.setQueryData(INSTALLATION_SETTINGS_KEY, data);
      // These texts are cached by record id alone, and focus does not refetch.
      // An open page would keep text written in the old language or zone.
      // The switch changes what every reading may show, so nothing cached
      // under the old answer may be drawn again.
      if (
        patch.individual_performance !== undefined ||
        patch.forecast_self_correcting !== undefined
      ) {
        void queryClient.invalidateQueries();
      }
      if (SETTINGS_IN_SERVER_TEXT.some((key) => patch[key] !== undefined)) {
        for (const prefix of LANGUAGE_DEPENDENT_QUERY_PREFIXES) {
          void queryClient.invalidateQueries({ queryKey: prefix });
        }
      }
      onSaved?.();
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

function InstallationSettingsForm({
  settings,
  canManage,
}: {
  settings: InstallationSettings;
  canManage: boolean;
}) {
  const t = useT();
  const currencyNote = useCurrencyNote();
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
  const calling = useUpdateInstallationSettings();
  const individual = useUpdateInstallationSettings();
  const selfCorrecting = useUpdateInstallationSettings();
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
    describedByIds(control["aria-describedby"], !canManage && denialId);

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

  const editVerb = (fact: EditedFact, field: string) => {
    const lock = lockFor(settings.locks, `installation.${fact}`, t);
    return (
      <Button
        variant="ghost"
        // Named by the fact it changes, not "Edit": three rows offering three
        // identically-named buttons make a screen reader's user count them.
        aria-label={t("installationSettings.editField", { field })}
        reason={lock.disabled ? lock.reason : undefined}
        reasonId={canManage || lock.disabled ? undefined : denialId}
        onClick={() => setEditing(fact)}
      >
        {t("installationSettings.edit")}
      </Button>
    );
  };

  return (
    <Panel title={t("installationSettings.companyTitle")}>
      {!canManage && (
        <PanelBody>
          <p id={denialId}>{t("installationSettings.readOnly")}</p>
        </PanelBody>
      )}
      <SettingList bleed="settings">
        <SettingRow
          label={t("installationSettings.timezone")}
          description={t("installationSettings.timezoneHint")}
          value={settings.timezone}
          control={editVerb("timezone", t("installationSettings.timezone"))}
        />
        <SettingRow
          label={t("installationSettings.baseCurrency")}
          description={currencyNote(settings)}
          value={settings.base_currency}
          control={editVerb(
            "base_currency",
            t("installationSettings.baseCurrency"),
          )}
        />
        <RegionalSettingsRows settings={settings} editVerb={editVerb} />
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
          // The month and the fiscal year it starts; a bare "4" leaves the
          // admin to work out what a report will say.
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
        <SettingRow
          label={t("installationSettings.forwardMeasure")}
          description={t("installationSettings.forwardMeasureHint")}
          // The measure's own sentence rather than the stored word: an admin
          // is deciding what a projection means.
          value={t(
            `installationSettings.forwardMeasure.${settings.forecast_forward_measure}`,
          )}
          control={editVerb(
            "forecast_forward_measure",
            t("installationSettings.forwardMeasure"),
          )}
        />
      </SettingList>
      <CallingSettingsRows
        settings={settings}
        canManage={canManage}
        denialId={denialId}
        update={calling}
      />
      <IndividualPerformanceRows
        settings={settings}
        canManage={canManage}
        denialId={denialId}
        update={individual}
      />
      <ForecastSelfCorrectingRows
        settings={settings}
        canManage={canManage}
        denialId={denialId}
        update={selfCorrecting}
      />
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
    </Panel>
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
