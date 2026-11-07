// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { providerName } from "../format/connectorprovider";
import { formatPercent } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { usePendingApprovalCount } from "../lib/approvals.queries";
import { useCan, useHoldsAdminRole } from "../lib/capability";
import { type CaptureProgress, liveCapture } from "../lib/capture-progress";
import { serviceHealth } from "../lib/connector-status";
import { useConnectors } from "../lib/connectors";
import { type LicensePosture, useLicensePosture } from "../lib/license-posture";

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
   *  An exact total the server counts across every page. */
  waiting: number | undefined;
  /** Sources the agent cannot reach, one per connection. A Gmail and a
   *  Calendar grant of one account share its address, so each carries its
   *  connection id as identity and names its provider beside the address. */
  offline: readonly OfflineSource[];
  /** True when the connections read failed: nobody knows which sources are
   *  reachable, so no line may say that nothing needs attention. */
  sourcesUnknown: boolean;
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

type Connection = components["schemas"]["CaptureConnection"];

/** One unreachable connection, as the panel names it. */
export type OfflineSource = Readonly<{
  id: string;
  provider: Connection["provider"];
  status: Connection["status"];
  /** The account and provider in the reader's words; display only. */
  label: string;
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

/** A source the agent cannot read, by the health the Connections page shows. A
 *  connection its owner disconnected is not broken, and one still "connected"
 *  while its sync keeps failing is. */
export function isOffline(connection: Connection): boolean {
  const health = serviceHealth(connection);
  return health === "needs_reconnect" || health === "needs_attention";
}

/** The account address and its provider, or the provider alone when the
 *  connection carries no address. The label is display only; the id is the
 *  identity, so a missing address never merges two connections. */
export function offlineSource(
  connection: Connection,
  t: (key: MessageKey, vars?: Record<string, string>) => string,
): OfflineSource {
  const provider = providerName(connection.provider, t);
  const label = connection.account_label
    ? t("agent.fact.sourceOnProvider", {
        account: connection.account_label,
        provider,
      })
    : provider;
  return {
    id: connection.id,
    provider: connection.provider,
    status: connection.status,
    label,
  };
}

export function useSignals(): Signals {
  const t = useT();
  const { locale } = useLocale();
  const approvals = usePendingApprovalCount();
  const connectors = useConnectors();
  const agent = useAgentProfile();
  const license = useLicensePosture();

  const connections = connectors.data?.data ?? [];
  const offline = connections
    .filter(isOffline)
    .map((connection) => offlineSource(connection, t));
  const capture = liveCapture(connections);

  return {
    // Absent `data` means the read has not answered, or was refused. A 0 here
    // would be this surface inventing an all-clear.
    waiting: approvals.data?.count,
    offline,
    sourcesUnknown: connectors.isError,
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
 * The model the agent last actually ran on — the SERVED one, not the configured
 * one, because a fallback ladder makes those two differ exactly when it matters.
 *
 * A short window, not one row: the newest call is often an embedding or a
 * failure, and the panel names the newest call that answered (modelText). The
 * recap beside it is drawn from the AI-activity feed, not from this trace.
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
        params: { query: { limit: 10 } },
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
 * The server omits the figure when no line could be priced. That month, or a
 * failed read, draws no figure rather than a zero that claims it cost nothing.
 *
 * Admin-only: the role narrows `ai_diagnostics:read`, which an edited role may
 * also hold. The grant stays beside it, so an admin without it asks for no 403.
 */
export function useAiSpend(): Spend {
  const admin = useHoldsAdminRole();
  const granted = useCan("ai_diagnostics", "read");
  const allowed = admin && granted;
  // Under the "ai-usage" key, so whatever refreshes the usage view refreshes
  // this figure too.
  const summary = useQuery({
    queryKey: ["ai-usage", "agentrail-month"],
    enabled: allowed,
    // The month's spend does not move between two page opens, and this read sits
    // in the chrome, so a short staleness would put a request behind every
    // navigation.
    staleTime: 5 * 60_000,
    queryFn: async () => {
      const { data, error } = await api.GET("/ai/usage/summary");
      if (error) {
        // Chrome must not take a page down over a reading: an unreadable figure
        // is a state this surface draws, not an error it throws.
        return null;
      }
      return data;
    },
  });
  return {
    allowed,
    minor: summary.data?.cost_est_minor,
    currency: summary.data?.currency ?? "USD",
  };
}
