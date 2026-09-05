// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Dispatch } from "react";
import { useState } from "react";
import type { components } from "../../api/schema";
import { useCanWrite } from "../../app/capability";
import { useInstallationSettings } from "../../app/uploadlimit";
import { Field, TextInput } from "../../design-system/atoms";
import { ordinalNumber } from "../../format/format";
import { useT } from "../../i18n";
import { problemFieldErrorsOf, problemMessageOf, QueryGate } from "../common";
import {
  currencyNote,
  useUpdateInstallationSettings,
} from "../installation-settings";
import type {
  ConversationEvent,
  ConversationState,
} from "./conversation-machine";
import { presenceFor } from "./presence";
import { railStops } from "./rail";
import { WayOnward } from "./way-onward";
import { ConversationWorkbench } from "./workbench";

// The basis act: the installation's reporting basis — base currency and
// reporting timezone — asked right after the company is confirmed, before any
// step about the person answering. It is the one installation-wide answer the
// setup needs, and it belongs with the company rather than at the end: every
// deal, report and brief that follows is priced and dated on it.
//
// Both fields are prefilled from what the installation already holds, so a
// reader who agrees presses Continue and nothing is written twice. They are
// the installation's, so they are shown only to a reader who may change them
// (the same grant Settings checks), and written as the one sparse patch
// Settings writes; a reader without the grant simply continues.
//
// Leaving is a server fact before it is a UI fact: the settings patch lands
// first (a refusal stays on its field), and only then does the act move on.

type BasisActProps = Readonly<{
  state: ConversationState;
  dispatch: Dispatch<ConversationEvent>;
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

export function BasisAct({ state, dispatch }: BasisActProps) {
  const t = useT();
  const canManage = useCanWrite("installation_settings", "update");
  const settings = useInstallationSettings(canManage);
  // What the reader typed over the stored values, and nothing else: the
  // fields show the edit where there is one and the server's value otherwise,
  // so an answer arriving late never overwrites what somebody is typing.
  const [currencyEdit, setCurrencyEdit] = useState<string | null>(null);
  const [timezoneEdit, setTimezoneEdit] = useState<string | null>(null);
  const update = useUpdateInstallationSettings(() => undefined);
  const [leaving, setLeaving] = useState(false);

  const stored = settings.data;
  const currency = currencyEdit ?? stored?.base_currency ?? "";
  const timezone = timezoneEdit ?? stored?.timezone ?? "";
  const reportingShown = canManage && stored !== undefined;
  const blockers = reportingShown
    ? [
        [!CURRENCY_CODE.test(currency), t("setup.baseCurrencyMalformed")],
        [timezone.trim() === "", t("ob.conv.basis.timezoneNeeded")],
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

  const leave = async () => {
    setLeaving(true);
    const patch = reportingShown
      ? reportingPatch(stored, currency, timezone)
      : null;
    if (patch !== null) {
      try {
        await update.mutateAsync(patch);
      } catch {
        // The refusal is on the field (or in the notice below); the reader
        // fixes it here rather than learning of it after the act moved on.
        setLeaving(false);
        return;
      }
    }
    setLeaving(false);
    dispatch({ type: "BASIS_DONE" });
  };

  const stops = railStops(state.memberPath);
  const eyebrow = t("ob.conv.scene.step", {
    n: ordinalNumber(stops.findIndex((stop) => stop.key === "basis") + 1),
    m: ordinalNumber(stops.length),
    label: t("ob.rail.basis"),
  });

  return (
    <ConversationWorkbench
      core={presenceFor(state).core}
      railState={state}
      status={t("ob.ai.ready")}
      eyebrow={eyebrow}
      title={t("ob.conv.basis.title")}
      sub={t("ob.conv.basis.body")}
    >
      <div className="ob-scene ob-prefs-scene">
        {canManage && (
          <QueryGate query={settings} pendingLabel={t("ob.conv.basis.title")}>
            {(current) => (
              <section className="ob-prefs-section">
                <h3>{t("ob.conv.basis.reportingTitle")}</h3>
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
                      disabled={current.base_currency_locked || leaving}
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
                      disabled={leaving}
                      onChange={(event) => setTimezoneEdit(event.target.value)}
                    />
                  )}
                </Field>
              </section>
            )}
          </QueryGate>
        )}
        <WayOnward
          label={t("ob.conv.basis.continue")}
          pending={leaving}
          blockers={blockers}
          stillNeeded={(why) => why.join(" ")}
          note={
            update.isError && refused.size === 0 ? (
              <p className="ob-stage-note" role="alert">
                {problemMessageOf(update.error, t)}
              </p>
            ) : undefined
          }
          onGo={() => void leave()}
        />
      </div>
    </ConversationWorkbench>
  );
}
