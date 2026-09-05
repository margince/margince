// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "@composition/schema";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { TriangleAlert } from "lucide-react";
import { type ReactNode, useState } from "react";
import { api } from "../api/client";
import { Checkbox } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import "./overnight-grant.css";
import { problemMessageOf, QueryGate, throwProblem } from "./common";

// The rep's standing answer: may an agent work as them overnight?
//
// ONE QUESTION, ASKED IN TWO PLACES. It is put next to the mailbox connect in
// onboarding, because that is the moment a rep is already deciding what
// Margince may see, and again in Settings beside the same connectors, because
// an answer given once has to be findable later. Both surfaces read and write
// through this file so the two cannot drift into saying different things about
// the same authority.
//
// The two shapes differ on purpose, and the design system says why: a Switch IS
// the action — flipping it writes — while a Checkbox states an intent that
// something later submits. Onboarding's box is an intent carried by the connect
// step, so it is a Checkbox. Settings writes on flip, so it is a Switch.

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
const OVERNIGHT_CHOICE_KEY = "ob.connect.overnightChoice";

/** Remembers the rep's answer across the OAuth round trip. */
export function rememberOvernightChoice(wanted: boolean): void {
  try {
    sessionStorage.setItem(OVERNIGHT_CHOICE_KEY, wanted ? "yes" : "no");
  } catch {
    // Storage can be unavailable (private browsing, disabled). The connect
    // still works; the answer falls back to the preselected default, which is
    // the same answer the rep would have been shown had they never touched it.
  }
}

/** The remembered answer, or undefined when this tab has none. Undefined is
 * distinct from `false`: it means "not answered here", which is what makes the
 * preselected default correct rather than an override of an opt-out. */
export function rememberedOvernightChoice(): boolean | undefined {
  try {
    const stored = sessionStorage.getItem(OVERNIGHT_CHOICE_KEY);
    return stored === null ? undefined : stored === "yes";
  } catch {
    return undefined;
  }
}

/** Drops the mark once the step has acted on it, so a later visit to
 * onboarding in the same tab starts from the default rather than from an
 * answer given to a question that has already been settled. */
export function forgetOvernightChoice(): void {
  try {
    sessionStorage.removeItem(OVERNIGHT_CHOICE_KEY);
  } catch {
    // Nothing to clear if storage was never available.
  }
}

/** Reads every scheduled agent's standing answer for the signed-in rep. */
export function useAgentGrants() {
  return useQuery({
    queryKey: GRANTS_KEY,
    queryFn: async () => {
      const { data, error } = await api.GET("/me/agent-grants");
      if (error) {
        throwProblem(error);
      }
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
      const { data, error } = await api.PUT("/me/agent-grants/{spec}", {
        params: { path: { spec: MORNING_BRIEF } },
        body: { granted },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
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
 * What stops working without this answer, in red.
 *
 * It is the SAME words on both surfaces, because the cost is the same in both
 * places and a rep who reads one and then the other must not be told two
 * different things. `live="alert"` so it is announced when it appears rather
 * than only seen.
 */
export function OvernightGrantDanger() {
  const t = useT();
  return (
    <Callout tone="danger" live="alert" icon={TriangleAlert}>
      {t("overnightGrant.danger")}
    </Callout>
  );
}

/**
 * The onboarding question: a preselected checkbox beside the mailbox connect.
 *
 * PRESELECTED, and it stays that way unless the rep clears it — the features it
 * feeds are the ones the product opens on, so an unticked default would ship a
 * new installation whose morning brief is permanently empty for reasons nobody
 * is told. Clearing it is allowed and warned about, never blocked: it is the
 * rep's authority, and a question that cannot be answered "no" is not a
 * question.
 *
 * It does not write. The answer rides with the mailbox connect the step is
 * already performing — see `checked` / `onChange` — so a rep who abandons
 * onboarding here has granted nothing.
 */
export function OvernightGrantChoice({
  checked,
  onChange,
  failed = false,
}: Readonly<{
  checked: boolean;
  onChange: (next: boolean) => void;
  /** The answer could not be recorded when the step completed. Shown rather
   * than swallowed: a rep who ticked the box and hit a transient failure would
   * otherwise leave onboarding believing they had answered. */
  failed?: boolean;
}>) {
  const t = useT();
  return (
    <div className="overnight-grant-choice">
      <Checkbox
        checked={checked}
        onChange={(event) => onChange(event.currentTarget.checked)}
        label={t("overnightGrant.label")}
        data-testid="overnight-grant-choice"
      />
      <p className="overnight-grant-help t-sub">{t("overnightGrant.help")}</p>
      {!checked && <OvernightGrantDanger />}
      {failed && (
        <Callout tone="warn" live="alert">
          {t("overnightGrant.saveFailed")}
        </Callout>
      )}
    </div>
  );
}

/**
 * The settings card: the same question, answered again later.
 *
 * It writes on flip rather than behind a Save button, which is the opposite of
 * the mail-sharing card beside it — and deliberately. Mail sharing changes what
 * a whole team can see, so it earns a deliberate commit; this changes only
 * whether an agent works for the one rep flipping it, and it is reversible in
 * the same click.
 */
export function OvernightGrantCard() {
  const t = useT();
  const query = useAgentGrants();
  const save = useSetAgentGrant();
  // While a flip is in flight the switch shows what the rep just chose, so it
  // does not snap back to the stored answer for the width of a round trip.
  const [optimistic, setOptimistic] = useState<boolean | null>(null);

  return (
    <Panel title={t("overnightGrant.title")}>
      <PanelBody>
        <p className="settings-panel-sub">{t("overnightGrant.sub")}</p>
        <QueryGate query={query} pendingLabel={t("overnightGrant.title")}>
          {(grants) => {
            const grant = morningBriefGrant(grants);
            const granted = grant?.state === "granted";
            const shown = optimistic ?? granted;
            // The rep agreed and their credential no longer does the job.
            // Reported as its own state rather than as a decline, because they
            // already answered — asking again would be putting a settled
            // question back to them.
            //
            // TWO CAUSES, and the rep is owed which one. The passport lapsed —
            // revoked or expired — or the agent has since gained a tool the
            // passport was never minted to fund, in which case it is perfectly
            // live authority for a job this agent no longer does. Neither
            // fails the run: it degrades, silently, at 2am.
            const lapsed = granted && !grant?.credential_usable;
            const outgrown =
              granted &&
              grant?.credential_usable === true &&
              !grant?.credential_funds_agent;
            return (
              <>
                <SettingList>
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
                <GrantNotices
                  showDanger={!shown}
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
      </PanelBody>
    </Panel>
  );
}

/** The card's three possible notices, kept out of the card so its own body
 * stays the question rather than the answers to it. */
function GrantNotices({
  showDanger,
  showRenewal,
  showScopeRenewal,
  error,
}: Readonly<{
  showDanger: boolean;
  showRenewal: boolean;
  showScopeRenewal: boolean;
  error?: ReactNode;
}>) {
  const t = useT();
  if (!showDanger && !showRenewal && !showScopeRenewal && error === undefined) {
    return null;
  }
  return (
    <div className="settings-panel-commit">
      {showDanger && <OvernightGrantDanger />}
      {showRenewal && (
        <Callout tone="warn" live="status">
          {t("overnightGrant.renew")}
        </Callout>
      )}
      {showScopeRenewal && (
        <Callout tone="warn" live="status">
          {t("overnightGrant.renewScope")}
        </Callout>
      )}
      {error !== undefined && (
        <Callout tone="danger" live="alert">
          {error}
        </Callout>
      )}
    </div>
  );
}
