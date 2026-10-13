// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "@composition/schema";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { api } from "../api/client";
import { Badge } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody } from "../design-system/panel";
import { QueryGate } from "../design-system/querystates";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { problemMessageOf, unwrap } from "../lib/problem";
import {
  readStored,
  removeStored,
  STORAGE_KEYS,
  writeStored,
} from "../lib/storage";

// The rep's standing answer: may an agent work as them overnight?
//
// ONE QUESTION, ASKED IN TWO PLACES: beside the mailbox connect in onboarding,
// where the rep is already deciding what Margince may see, and again in
// Settings, where an answer given once has to be findable later. Both surfaces
// read and write through this file so the two cannot drift apart.
//
// Both draw a Switch row. Settings writes on flip; onboarding's switch holds the
// answer until the step continues, and its description says so in words.

type MyAgentGrant = components["schemas"]["MyAgentGrant"];

/** The agent this question is about. The morning brief is the one a rep
 * recognises by name, and the sweep that feeds it runs under the same answer. */
export const MORNING_BRIEF = "morning_brief" as const;

const GRANTS_KEY = ["agent-grants"] as const;

// A real OAuth "allow" leaves the page entirely for the provider's consent
// screen, so React state does not survive the trip back. The rep's answer to
// the overnight question has to, or an opt-out is silently reversed: they
// untick the box, click Google, come back, and the step grants anyway because
// the component remounted at its preselected default.
//
// Same mechanism and same scope as the OAuth attempt mark beside it
// (`onboarding-connect-panels.tsx`): sessionStorage, belonging to the ONE tab
// that started the trip.

/** Remembers the rep's answer across the OAuth round trip. */
export function rememberOvernightChoice(wanted: boolean): void {
  writeStored(STORAGE_KEYS.overnightChoice, wanted ? "yes" : "no");
}

/** The remembered answer, or undefined when this tab has none. Undefined is
 * distinct from `false`: it means "not answered here", which is what makes the
 * preselected default correct rather than an override of an opt-out. */
export function rememberedOvernightChoice(): boolean | undefined {
  const stored = readStored(STORAGE_KEYS.overnightChoice);
  return stored === null ? undefined : stored === "yes";
}

/** Drops the mark once the step has acted on it, so a later visit to
 * onboarding in the same tab starts from the default rather than from an
 * answer given to a question that has already been settled. */
export function forgetOvernightChoice(): void {
  removeStored(STORAGE_KEYS.overnightChoice);
}

/** Reads every scheduled agent's standing answer for the signed-in rep. */
export function useAgentGrants() {
  return useQuery({
    queryKey: GRANTS_KEY,
    queryFn: async () => {
      const data = unwrap(await api.GET("/me/agent-grants"));
      return data.data;
    },
  });
}

/** Writes one answer. Granting mints the rep's own credential server-side;
 * withdrawing revokes it. Neither is a field this client could send. */
export function useSetAgentGrant() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const t = useT();
  return useMutation({
    mutationFn: async (granted: boolean) => {
      return unwrap(
        await api.PUT("/me/agent-grants/{spec}", {
          params: { path: { spec: MORNING_BRIEF } },
          body: { granted },
        }),
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: GRANTS_KEY });
      toast.show(t("settings.saved"));
    },
  });
}

/** Picks the morning brief's answer out of the list. */
export function morningBriefGrant(
  grants: readonly MyAgentGrant[],
): MyAgentGrant | undefined {
  return grants.find((grant) => grant.spec === MORNING_BRIEF);
}

/**
 * What stops working without this answer, said where the answer is being given.
 * `warning` and `standing`, not danger: a "no" is the rep's authority to give,
 * and the notice reports its cost rather than a fault.
 */
function GrantCostNotice() {
  const t = useT();
  return (
    <Callout
      kind="standing"
      tone="warning"
      title={t("overnightGrant.dangerTitle")}
    >
      {t("overnightGrant.danger")}
    </Callout>
  );
}

/**
 * The onboarding question: a preselected Switch row for the caller's SettingList.
 *
 * PRESELECTED: the features it feeds are the ones the product opens on, so an
 * off default would ship a morning brief that stays empty for reasons nobody is
 * told. Turning it off is warned about, never blocked.
 *
 * It does not write. The answer rides with the step's continue (`checked` /
 * `onChange`), so a rep who abandons onboarding here has granted nothing.
 */
export function OvernightGrantChoice({
  checked,
  onChange,
  failed = false,
}: Readonly<{
  checked: boolean;
  onChange: (next: boolean) => void;
  /** The answer could not be recorded when the step completed. Shown rather
   * than swallowed, so the rep does not leave believing they had answered. */
  failed?: boolean;
}>) {
  const t = useT();
  return (
    <>
      <SettingRow
        label={t("overnightGrant.label")}
        description={t("overnightGrant.choiceHelp")}
        control={(control) => (
          <Switch
            describedBy={control["aria-describedby"]}
            testId="overnight-grant-choice"
            label={t("overnightGrant.label")}
            labelHidden
            checked={checked}
            onChange={onChange}
          />
        )}
      />
      {!checked && <GrantCostNotice />}
      {failed && (
        <Callout
          kind="outcome"
          tone="danger"
          title={t("overnightGrant.saveFailedTitle")}
        >
          {t("overnightGrant.saveFailed")}
        </Callout>
      )}
    </>
  );
}

/**
 * The settings card: the same question, answered again later.
 *
 * It writes on flip rather than behind a Save button: it changes only whether an
 * agent works for the one rep flipping it, and it is reversible in the same click.
 */
export function OvernightGrantCard() {
  const t = useT();
  const query = useAgentGrants();
  const save = useSetAgentGrant();
  // While a flip is in flight the switch shows what the rep just chose, so it
  // does not snap back to the stored answer for the width of a round trip.
  const [optimistic, setOptimistic] = useState<boolean | null>(null);
  const stored =
    query.data === undefined
      ? undefined
      : morningBriefGrant(query.data)?.state === "granted";
  const shownOff = (optimistic ?? stored) === false;

  return (
    <Panel
      title={t("overnightGrant.title")}
      titleAction={shownOff ? <Badge>{t("overnightGrant.off")}</Badge> : null}
    >
      <QueryGate query={query} pendingLabel={t("overnightGrant.title")}>
        {(grants) => {
          const grant = morningBriefGrant(grants);
          const granted = grant?.state === "granted";
          const shown = optimistic ?? granted;
          // Lapsed and outgrown differ from a decline (the rep already agreed);
          // each names its cause, since either degrades the 2am run silently.
          const lapsed = granted && !grant?.credential_usable;
          const outgrown =
            granted &&
            grant?.credential_usable === true &&
            !grant?.credential_funds_agent;
          return (
            <>
              <SettingList bleed="settings">
                <SettingRow
                  label={t("overnightGrant.label")}
                  description={t("overnightGrant.help")}
                  control={(control) => (
                    <Switch
                      describedBy={control["aria-describedby"]}
                      testId="overnight-grant-toggle"
                      label={t("overnightGrant.label")}
                      labelHidden
                      checked={shown}
                      pending={save.isPending}
                      onChange={(next) => {
                        setOptimistic(next);
                        save.mutate(next, {
                          onSettled: () => setOptimistic(null),
                        });
                      }}
                    />
                  )}
                />
              </SettingList>
              {/* A quiet sentence, not a warning: off is a settled answer on
                  this page, and reading it back as a fault every visit nags. */}
              {!shown && (
                <PanelBody>
                  <p className="t-sub">{t("overnightGrant.offNote")}</p>
                </PanelBody>
              )}
              <GrantNotices
                showRenewal={lapsed && shown}
                showScopeRenewal={outgrown && shown}
                error={
                  save.isError ? problemMessageOf(save.error, t) : undefined
                }
              />
            </>
          );
        }}
      </QueryGate>
    </Panel>
  );
}

/** The card's possible notices, kept out of the card so its own body stays the
 * question rather than the answers to it. */
function GrantNotices({
  showRenewal,
  showScopeRenewal,
  error,
}: Readonly<{
  showRenewal: boolean;
  showScopeRenewal: boolean;
  error?: ReactNode;
}>) {
  const t = useT();
  if (!showRenewal && !showScopeRenewal && error === undefined) {
    return null;
  }
  return (
    <PanelBody>
      <div className="settings-panel-commit">
        {showRenewal && (
          <Callout
            kind="event"
            tone="warning"
            title={t("overnightGrant.renewTitle")}
          >
            {t("overnightGrant.renew")}
          </Callout>
        )}
        {showScopeRenewal && (
          <Callout
            kind="event"
            tone="warning"
            title={t("overnightGrant.renewScopeTitle")}
          >
            {t("overnightGrant.renewScope")}
          </Callout>
        )}
        {error !== undefined && (
          <Callout
            kind="outcome"
            tone="danger"
            title={t("overnightGrant.writeFailedTitle")}
          >
            {error}
          </Callout>
        )}
      </div>
    </PanelBody>
  );
}
