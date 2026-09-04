// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Dispatch } from "react";
import { useState } from "react";
import type { components } from "../../api/schema";
import { useCanWrite } from "../../app/capability";
import { navigate } from "../../app/router";
import { useInstallationSettings } from "../../app/uploadlimit";
import { Field, TextInput } from "../../design-system/atoms";
import { ordinalNumber } from "../../format/format";
import { useT } from "../../i18n";
import { AutonomyChoices, useAutonomy } from "../autonomy-settings";
import { problemFieldErrorsOf, problemMessageOf, QueryGate } from "../common";
import {
  currencyNote,
  useUpdateInstallationSettings,
} from "../installation-settings";
import { EMPTY_DRAFT } from "../onboarding";
import { BuildScene } from "../onboarding-build-scene";
import type {
  ConversationEvent,
  ConversationState,
} from "./conversation-machine";
import { presenceFor } from "./presence";
import { railStops } from "./rail";
import type { WizardPersistInput } from "./use-wizard-state";
import { WayOnward } from "./way-onward";
import { ConversationWorkbench } from "./workbench";

// The preferences act: the last word before the app. Two things are asked,
// both prefilled from what the installation already holds, so a reader who
// agrees presses Done and nothing is written twice.
//
// The reporting basis — base currency and reporting timezone — is the
// installation's, so it is shown only to a reader who may change it (the same
// grant Settings checks), and written as the one sparse patch Settings writes.
// What the agent may change on its own is the reader's own, every seat has it,
// and each switch records itself the moment it moves, as it does in Settings.
//
// Finishing is a server fact before it is a UI fact: the settings patch lands
// first (a refusal stays on its field), then step "complete", then the handoff
// plays. A completion written before the patch would send a reload into the
// app with a currency the admin believed they had changed.

type PrefsActProps = Readonly<{
  state: ConversationState;
  dispatch: Dispatch<ConversationEvent>;
  persist: (input: WizardPersistInput) => Promise<boolean>;
}>;

// A currency is three letters, whatever case it was typed in; the server
// compares the upper-cased code, so the field sends what the server keeps.
const CURRENCY_CODE = /^[A-Za-z]{3}$/;

type InstallationSettings = components["schemas"]["InstallationSettings"];
type SettingsPatch = components["schemas"]["UpdateInstallationSettingsRequest"];

// The sparse patch Settings writes: only what the reader changed, or null
// when nothing did, so agreeing with the prefilled values writes nothing.
function reportingPatch(
  stored: InstallationSettings,
  currency: string,
  timezone: string,
): SettingsPatch | null {
  const code = currency.toUpperCase();
  const zone = timezone.trim();
  const patch: SettingsPatch = {
    ...(code !== stored.base_currency ? { base_currency: code } : {}),
    ...(zone !== stored.timezone ? { timezone: zone } : {}),
  };
  return Object.keys(patch).length > 0 ? patch : null;
}

export function PrefsAct({ state, dispatch, persist }: PrefsActProps) {
  const t = useT();
  const canManage = useCanWrite("installation_settings", "update");
  const settings = useInstallationSettings(canManage);
  const autonomy = useAutonomy();
  // What the reader typed over the stored values, and nothing else: the
  // fields show the edit where there is one and the server's value otherwise,
  // so an answer arriving late never overwrites what somebody is typing.
  const [currencyEdit, setCurrencyEdit] = useState<string | null>(null);
  const [timezoneEdit, setTimezoneEdit] = useState<string | null>(null);
  const update = useUpdateInstallationSettings(() => undefined);
  const [finishing, setFinishing] = useState(false);
  const [finishFailed, setFinishFailed] = useState(false);
  const [entering, setEntering] = useState(false);

  const stored = settings.data;
  const currency = currencyEdit ?? stored?.base_currency ?? "";
  const timezone = timezoneEdit ?? stored?.timezone ?? "";
  const reportingShown = canManage && stored !== undefined;
  const blockers = reportingShown
    ? [
        [!CURRENCY_CODE.test(currency), t("setup.baseCurrencyMalformed")],
        [timezone.trim() === "", t("ob.conv.prefs.timezoneNeeded")],
      ]
        .filter((need): need is [true, string] => need[0] === true)
        .map(([, why]) => why)
    : [];
  const refused = new Map(
    problemFieldErrorsOf(update.error).map((problem) => [
      problem.field,
      problem.message,
    ]),
  );

  const finish = async () => {
    setFinishing(true);
    setFinishFailed(false);
    const patch = reportingShown
      ? reportingPatch(stored, currency, timezone)
      : null;
    if (patch !== null) {
      try {
        await update.mutateAsync(patch);
      } catch {
        // The refusal is on the field (or in the notice below); the reader
        // fixes it here rather than learning of it after the handoff.
        setFinishing(false);
        return;
      }
    }
    const persisted = await persist({
      step: "complete",
      values: EMPTY_DRAFT.values,
    });
    setFinishing(false);
    if (!persisted) {
      setFinishFailed(true);
      return;
    }
    dispatch({ type: "PREFS_DONE" });
    setEntering(true);
  };

  if (entering || state.phase === "pf.done") {
    return <BuildScene onDone={() => navigate({ screen: "home" })} />;
  }

  const stops = railStops(state.memberPath);
  const eyebrow = t("ob.conv.scene.step", {
    n: ordinalNumber(stops.findIndex((stop) => stop.key === "prefs") + 1),
    m: ordinalNumber(stops.length),
    label: t("ob.rail.prefs"),
  });
  const autonomyRows = autonomy.data?.data ?? [];

  return (
    <ConversationWorkbench
      core={presenceFor(state).core}
      railState={state}
      status={t("ob.ai.ready")}
      eyebrow={eyebrow}
      title={t("ob.conv.prefs.title")}
      sub={t("ob.conv.prefs.body")}
    >
      <div className="ob-scene ob-prefs-scene">
        {canManage && (
          <QueryGate query={settings} pendingLabel={t("ob.conv.prefs.title")}>
            {(current) => (
              <section className="ob-prefs-section">
                <h3>{t("ob.conv.prefs.reportingTitle")}</h3>
                <Field
                  label={t("installationSettings.baseCurrency")}
                  hint={currencyNote(current, t)}
                  error={refused.get("base_currency")}
                >
                  {(control) => (
                    <TextInput
                      {...control}
                      value={currency}
                      maxLength={3}
                      autoComplete="off"
                      // A frozen currency is frozen for an admin too: the
                      // hint above says why, and the field says it cannot.
                      disabled={current.base_currency_locked || finishing}
                      onChange={(event) => setCurrencyEdit(event.target.value)}
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
                      value={timezone}
                      autoComplete="off"
                      disabled={finishing}
                      onChange={(event) => setTimezoneEdit(event.target.value)}
                    />
                  )}
                </Field>
              </section>
            )}
          </QueryGate>
        )}
        {/* An empty set is this seat's own answer — nothing is routed to it
            of any kind — and on this surface that is nothing to ask, not an
            empty card. */}
        {autonomyRows.length > 0 && (
          <section className="ob-prefs-section">
            <h3>{t("ob.conv.prefs.autonomyTitle")}</h3>
            <p className="ob-prefs-lead">{t("ob.conv.prefs.autonomyBody")}</p>
            <AutonomyChoices rows={autonomyRows} />
          </section>
        )}
        <WayOnward
          label={t("ob.conv.prefs.done")}
          pending={finishing}
          blockers={blockers}
          stillNeeded={(why) => why.join(" ")}
          note={
            finishFailed || (update.isError && refused.size === 0) ? (
              <p className="ob-stage-note" role="alert">
                {finishFailed
                  ? t("ob.conv.prefs.persistFailed")
                  : problemMessageOf(update.error, t)}
              </p>
            ) : undefined
          }
          onGo={() => void finish()}
        />
      </div>
    </ConversationWorkbench>
  );
}
