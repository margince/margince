// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Inbox, type LucideIcon, Unplug } from "lucide-react";
import { AppLink } from "../design-system/applink";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { routeHash } from "../lib/router";
import { settingsHref } from "../lib/settingsrouting";
import { type AgentJob, AgentJobRow } from "./agentjobs";
import type { Signals } from "./agentrail-reads";

const CONNECTIONS_HREF = routeHash(settingsHref("connections"));

/** How many recommendations the panel names at once. */
const ADVICE_ROWS = 3;

/** A step the reader takes themselves: what stands, and the door to it. */
type Pointer = Readonly<{
  id: string;
  mark: LucideIcon;
  tone: "warn" | "plain";
  title: string;
  href: string;
  open: string;
}>;

/** What the readings ask of the reader, worst first. Unread readings ask nothing. */
function usePointers(signals: Signals): readonly Pointer[] {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const offline = signals.offline.map(
    (source): Pointer => ({
      id: `offline-${source.id}`,
      mark: Unplug,
      tone: "warn",
      title: t("agent.advice.offline", { source: source.label }),
      href: CONNECTIONS_HREF,
      open: t("agent.advice.offline.open"),
    }),
  );
  const waiting = signals.waiting ?? 0;
  if (waiting === 0) return offline;
  return [
    ...offline,
    {
      id: "approvals",
      mark: Inbox,
      tone: "plain",
      title: plural("agent.advice.approvals", waiting, {
        count: formatNumber(waiting, locale),
      }),
      href: "#/worklist",
      open: t("agent.advice.approvals.open"),
    },
  ];
}

/**
 * What the agent recommends next: repairs and decisions that are the reader's,
 * then the work it can take over. Absent when there is nothing to recommend.
 */
export function Recommended({
  signals,
  jobs,
}: Readonly<{ signals: Signals; jobs: readonly AgentJob[] }>) {
  const pointers = usePointers(signals);
  if (pointers.length === 0 && jobs.length === 0) return null;
  const shown = pointers.slice(0, ADVICE_ROWS);
  const offered = jobs.slice(0, ADVICE_ROWS - shown.length);
  return (
    <ul className="aradvices">
      {shown.map((pointer) => (
        <li className="aradvice" key={pointer.id}>
          <span
            className={
              pointer.tone === "warn"
                ? "aradvicemark aradvicewarn"
                : "aradvicemark"
            }
            aria-hidden="true"
          >
            <pointer.mark />
          </span>
          <p className="aradvicename">{pointer.title}</p>
          <AppLink className="link-button" href={pointer.href}>
            {pointer.open}
          </AppLink>
        </li>
      ))}
      {offered.map((job) => (
        <AgentJobRow key={job.id} job={job} />
      ))}
    </ul>
  );
}
