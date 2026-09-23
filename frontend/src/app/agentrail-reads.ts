// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { formatPercent } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { usePendingApprovals } from "../screens/approvals.queries";
import { useConnectors } from "../screens/connectors";
import { useLicenseEntitlement } from "../screens/license";
import { useCan, useHoldsAdminRole } from "./capability";
import { type CaptureProgress, liveCapture } from "./capture-progress";

// WHAT THE AGENT SECTION READS. The rail's row, the state it derives and the
// panel beside it all stand on these answers and on nothing else: approvals
// waiting, which sources are unreachable, the model the last call actually ran
// on, what the month cost.
//
// Nothing here is a zero standing in for a read that has not answered. A figure
// this seat may not make, or that has not landed, is `undefined` — which the
// surfaces draw as absence rather than as an all-clear.

/** One AI occurrence, as the server reports it. */
export type AiActivityItem = components["schemas"]["AiActivityItem"];

/** One terminal attempt of the model-call trace, as `/ai/calls` reports it. */
export type AiCall = components["schemas"]["AiCallSummary"];

/**
 * What the installation's entitlement adds up to, for a surface that reports
 * rather than enforces.
 *
 * `none` and `refused` are the two a contact has to act on, and they are why the
 * Core carries this at all: an installation with no licence is not a healthy
 * agent with a footnote, it is a standing fault, and the rail used to state it
 * as a grey row at the very bottom that nobody read. `pressing` is the same
 * claim one step softer: over the seat cap, in grace, or renewal due.
 */
export type LicensePosture = "ok" | "pressing" | "refused" | "none";

/**
 * What the deployment has bound, as `/assistant/profile` reports it.
 *
 * `configured` says the bindings were CONSTRUCTED at boot — the contract is
 * explicit that it is not a health check, so nothing here may render as online,
 * running or healthy. The negative is the honest half and the one worth showing:
 * a deployment with no provider key has an agent that cannot think, and every
 * other thing this bar reports is beside the point until that is fixed.
 */
export type AiPosture =
  | "configured"
  | "unconfigured"
  | "development"
  | "unknown";

/** What the installation can actually tell us, and what it cannot. */
export type Signals = Readonly<{
  /** Approvals staged for this human; undefined until the read answers.
   *  A true total — usePendingApprovals walks every page. */
  waiting: number | undefined;
  /** Sources the agent cannot reach, named as the reader knows them. */
  offline: readonly string[];
  /** Whether this deployment has a model bound at all. */
  ai: AiPosture;
  /** Who the panel is a report BY, in the deployment's own words. */
  name: string;
  /** What the installation is entitled to; undefined when this seat may not
   *  read it, which is not the same as an installation with no licence. */
  license: LicensePosture | undefined;
  /** The licence posture in the reader's words, for the panel's pill. */
  licenseLine: string;
  /**
   * Mail being imported this moment, with the sentence that says so; null
   * while no mailbox is importing. Read off the same connections list as
   * `offline`, so the orb never reports a mailbox as both unreachable and
   * mid-import from two answers.
   */
  capture: Readonly<{ progress: CaptureProgress; line: string }> | null;
}>;

/** What the agent has cost this month, and whether this seat may know. */
export type Spend = Readonly<{
  allowed: boolean;
  minor: number | undefined;
  currency: string;
}>;

/**
 * The agent's name, as the contract declares it.
 *
 * Typed off the schema rather than written out, so a deployment that ever names
 * its agent something else fails the build here instead of leaving the panel
 * titled after a product the reader does not have. It is the fallback only: the
 * profile read below is what actually answers, and this is what stands in the
 * heading for the moment before it does.
 */
const AGENT_NAME: components["schemas"]["AssistantProfile"]["name"] =
  "Margince";

/** Who is reporting, and whether it can think at all. */
function useAgentProfile(): Readonly<{ state: AiPosture; name: string }> {
  const profile = useQuery({
    queryKey: ["assistant-profile"],
    // Anonymous, cheap and effectively static for the life of the process: the
    // same key the sign-in screen uses, so the two share one answer.
    staleTime: Number.POSITIVE_INFINITY,
    retry: false,
    queryFn: async () => {
      const { data, error } = await api.GET("/assistant/profile");
      if (error) {
        return null;
      }
      return data;
    },
  });
  return {
    state: profile.data?.state ?? "unknown",
    name: profile.data?.name ?? AGENT_NAME,
  };
}

export function useSignals(): Signals {
  const t = useT();
  const { locale } = useLocale();
  const approvals = usePendingApprovals();
  const connectors = useConnectors();
  const agent = useAgentProfile();
  const license = useLicensePosture();

  const connections = connectors.data?.data ?? [];
  const offline = connections
    .filter((connection) => connection.status !== "connected")
    .map((connection) => connection.account_label ?? connection.provider);
  const capture = liveCapture(connections);

  return {
    // Absent `data` means the read has not answered, or was refused. A 0 here
    // would be this surface inventing an all-clear.
    waiting: approvals.data ? approvals.data.data.length : undefined,
    offline,
    ai: agent.state,
    name: agent.name,
    license,
    licenseLine:
      license === "refused"
        ? t("shell.license.refused")
        : t("shell.license.none"),
    capture:
      capture === null
        ? null
        : { progress: capture, line: captureLine(capture, t, locale) },
  };
}

/**
 * The one line an import puts under the orb: what is happening, and how far
 * along where a preview gave it a denominator. Without one the sentence stops
 * at the verb rather than inventing a share.
 */
function captureLine(
  capture: CaptureProgress,
  t: (key: MessageKey) => string,
  locale: Locale,
): string {
  const said = t("shell.capture.importing");
  return capture.fraction === null
    ? said
    : `${said} · ${formatPercent(capture.fraction, locale)}`;
}

/**
 * The installation's entitlement, reduced to the posture its chrome shows.
 *
 * Absent for a seat without `license:read`, silently: a read they may not make
 * is not a fact being withheld from them, it is a fact that is none of their
 * work, and a notice about it on every screen they opened would be a permission
 * boundary drawn as a fault.
 */
export function useLicensePosture(): LicensePosture | undefined {
  const mayRead = useCan("license", "read");
  const query = useLicenseEntitlement(mayRead);
  const entitlement = query.data;
  if (!mayRead || !entitlement) {
    return undefined;
  }
  if (entitlement.state === "rejected") {
    return "refused";
  }
  if (entitlement.state !== "valid") {
    return "none";
  }
  return entitlement.over_limit ||
    entitlement.license?.in_grace === true ||
    entitlement.license?.renewal_due === true
    ? "pressing"
    : "ok";
}

/**
 * The model the agent last actually ran on — the SERVED one, not the configured
 * one, because a fallback ladder makes those two differ exactly when it matters.
 *
 * ONE row, because the runtime facts show one model and nothing else there
 * reads the trace: the recap beside them is drawn from the AI-activity feed,
 * which is the projection that knows what each occurrence was ABOUT. Asking for
 * five would be four rows nothing renders.
 *
 * Operator-only, because `/ai/calls` sits behind `ai_diagnostics:read`. A seat
 * without it is told the model is not readable rather than shown one nobody on
 * that seat could verify.
 */
export function useLastCall(): Readonly<{
  allowed: boolean;
  calls: readonly AiCall[];
}> {
  // GET /ai/calls asks for ai_diagnostics:read (ai/callread.go).
  const allowed = useCan("ai_diagnostics", "read");
  const recent = useQuery({
    queryKey: ["ai-calls", "agentrail-served-model"],
    enabled: allowed,
    staleTime: 30_000,
    queryFn: async () => {
      const { data, error } = await api.GET("/ai/calls", {
        params: { query: { limit: 1 } },
      });
      if (error) {
        // Chrome must not take a page down over telemetry: an unreadable log is
        // a state this surface draws, not an error it throws.
        return [];
      }
      return data.data;
    },
  });
  return { allowed, calls: recent.data ?? [] };
}

/**
 * What the agent has cost this month, as the server priced it.
 *
 * `cost_est_minor` is an ESTIMATE the server computes on read from its own rate
 * tables, in minor units of the budget's currency, and it is omitted for a call
 * nothing could be priced against. So the sum is over the lines that HAVE a
 * price, and a month where nothing was priced draws no figure at all rather than
 * a confident zero: the difference between "this cost nothing" and "nobody knows
 * what this cost" is the whole point of the figure.
 *
 * Admin-only. The grant alone is not the predicate: `automation:update` is what
 * the server serves the figure on, and the ops seat holds it by default while an
 * operator-edited role may hold it too. What the agent costs is the
 * administrator's figure, not every seat's that may configure automation, so
 * the role narrows the grant here — and the grant still stands beside it,
 * because an admin whose role lost it would only be asking for a 403.
 */
export function useAiSpend(): Spend {
  const admin = useHoldsAdminRole();
  // GET /ai/usage asks for ai_diagnostics:read (ai/usage.go).
  const granted = useCan("ai_diagnostics", "read");
  const allowed = admin && granted;
  const usage = useQuery({
    queryKey: ["ai-usage", "agentrail-month"],
    enabled: allowed,
    // The month's spend does not move between two page opens, and this read sits
    // in the chrome, so a short staleness would put a request behind every
    // navigation.
    staleTime: 5 * 60_000,
    queryFn: async () => {
      const { data, error } = await api.GET("/ai/usage", {
        params: { query: {} },
      });
      if (error) {
        // Chrome must not take a page down over a reading: an unreadable figure
        // is a state this surface draws, not an error it throws.
        return null;
      }
      return data;
    },
  });
  const priced = (usage.data?.days ?? [])
    .flatMap((day) => day.tasks)
    .filter((task) => task.cost_est_minor !== undefined);
  return {
    allowed,
    minor:
      priced.length === 0
        ? undefined
        : priced.reduce((total, task) => total + (task.cost_est_minor ?? 0), 0),
    currency: usage.data?.budget.currency ?? "USD",
  };
}
