// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Badge, EmptyState } from "../../design-system/atoms";
import {
  DataTable,
  type DataTableColumn,
  type DataTableDetail,
} from "../../design-system/datatable";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { Meter } from "../../design-system/readings";
import { formatDateTime, formatNumber } from "../../format/format";
import { UTC_ZONE } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import {
  AGENT_COUNTERS,
  type AgentCounter,
  type AgentCounterUsage,
  type AgentUsageState,
  tightestCounter,
  usedUp,
} from "../../lib/agentusage";
import { useCan } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";
import "../../screens/settings-agents.css";

type CounterSet = Readonly<Record<AgentCounter, AgentCounterUsage>>;
type PassportSummary = components["schemas"]["PassportSummary"];
type UserUsage = components["schemas"]["AgentUserUsage"];

const STATE_TONE = {
  ok: "success",
  near: "warning",
  stopped: "danger",
} as const satisfies Record<AgentUsageState, "success" | "warning" | "danger">;

export function AgentUsageBadge({
  state,
}: Readonly<{ state: AgentUsageState }>) {
  const t = useT();
  return (
    <Badge tone={STATE_TONE[state]}>{t(`agentUsage.state.${state}`)}</Badge>
  );
}

// One allowance as a labelled bar: what was used of what may be. A table cell
// whose column already names the allowance leaves the name out.
function CounterMeter({
  counter,
  usage,
  named = true,
}: Readonly<{
  counter: AgentCounter;
  usage: AgentCounterUsage;
  named?: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const label = t(`agentUsage.counter.${counter}`);
  return (
    <div className="agent-usage-meter">
      <span className="agent-usage-figures">
        {named && <span>{label}</span>}
        <span className="t-caption">
          {t("agentUsage.ofLimit", {
            used: formatNumber(usage.used, locale),
            limit: formatNumber(usage.limit, locale),
          })}
        </span>
      </span>
      <Meter
        dense
        value={usage.used}
        max={usage.limit}
        label={label}
        tone={usedUp(usage) ? "danger" : undefined}
      />
    </div>
  );
}

// All three allowances, for a row opened to show them.
export function AgentUsageMeters({ usage }: Readonly<{ usage: CounterSet }>) {
  return (
    <div className="agent-usage-meters">
      {AGENT_COUNTERS.map((counter) => (
        <CounterMeter key={counter} counter={counter} usage={usage[counter]} />
      ))}
    </div>
  );
}

// The allowance closest to running out and the state, for a table cell.
export function AgentUsageCell({
  usage,
  state,
}: Readonly<{ usage: CounterSet; state: AgentUsageState }>) {
  const counter = tightestCounter(usage);
  return (
    <div className="agent-usage-cell">
      <CounterMeter counter={counter} usage={usage[counter]} />
      <AgentUsageBadge state={state} />
    </div>
  );
}

// This window's usage under a passport's name: the allowance closest to its
// limit and the state. A row with no usage is a revoked or expired passport, or
// one whose counters could not be read. It sits in the name's cell because a
// passport row has no width left for a column of its own.
export function PassportUsageLine({
  passport,
}: Readonly<{ passport: PassportSummary }>) {
  return passport.usage ? (
    <AgentUsageCell usage={passport.usage} state={passport.usage.state} />
  ) : null;
}

// Opens a passport row onto all three allowances.
export function usePassportUsageDetail<Row extends PassportSummary>(
  nameOf: (row: Row) => string,
): DataTableDetail<Row> {
  const t = useT();
  return {
    header: t("settings.passportColWindow"),
    toggleLabel: (row) =>
      t("settings.passportUsageToggle", { name: nameOf(row) }),
    has: (row) => row.usage != null,
    render: (row) =>
      row.usage ? <AgentUsageMeters usage={row.usage} /> : null,
  };
}

// When every allowance starts again, in UTC as the AI allowance states it.
export function useResetLine(): (resetsAt: string) => string {
  const t = useT();
  const { locale } = useLocale();
  return (resetsAt) =>
    t("agentUsage.resets", {
      date: formatDateTime(resetsAt, locale, UTC_ZONE),
    });
}

function useAgentUsageByUser(enabled: boolean) {
  return useQuery({
    queryKey: ["agent-usage"],
    enabled,
    queryFn: async () => unwrap(await api.GET("/ai/agent-usage")),
  });
}

// How much each user's agents have used in the running window. It shows how
// much and whose, never which passports or what they read.
export function AgentUsageByUserCard() {
  const t = useT();
  const me = useMe();
  const canSee = useCan("ai_diagnostics", "read");
  const query = useAgentUsageByUser(canSee);
  const resetLine = useResetLine();
  if (!canSee) {
    return (
      <Panel title={t("agentUsage.title")}>
        <PanelBody>
          <PanelIntro>{t("agentUsage.sub")}</PanelIntro>
          <QueryGate query={me} pendingLabel={t("agentUsage.title")}>
            {() => <EmptyState>{t("agentUsage.withheld")}</EmptyState>}
          </QueryGate>
        </PanelBody>
      </Panel>
    );
  }
  return (
    <Panel title={t("agentUsage.title")}>
      <QueryGate query={query} pendingLabel={t("agentUsage.title")}>
        {(page) => (
          <>
            <PanelBody>
              <PanelIntro>{t("agentUsage.sub")}</PanelIntro>
              <p className="t-caption">{resetLine(page.resets_at)}</p>
            </PanelBody>
            {page.data.length === 0 ? (
              <PanelBody>
                <EmptyState>{t("agentUsage.empty")}</EmptyState>
              </PanelBody>
            ) : (
              <UserUsageTable rows={page.data} />
            )}
          </>
        )}
      </QueryGate>
    </Panel>
  );
}

function UserUsageTable({ rows }: Readonly<{ rows: readonly UserUsage[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const columns: DataTableColumn<UserUsage>[] = [
    {
      key: "user",
      header: t("agentUsage.col.user"),
      grow: true,
      fold: "title",
      render: (row) => row.display_name,
    },
    {
      key: "passports",
      header: t("agentUsage.col.passports"),
      align: "end",
      render: (row) => formatNumber(row.passport_count, locale),
    },
    {
      key: "window",
      header: t("settings.passportColWindow"),
      render: (row) => <AgentUsageCell usage={row} state={row.state} />,
    },
  ];
  return (
    <DataTable
      bleed
      fold
      label={t("agentUsage.title")}
      columns={columns}
      rows={[...rows]}
      rowKey={(row) => row.user_id}
      detail={{
        header: t("settings.passportColWindow"),
        toggleLabel: (row) =>
          t("settings.passportUsageToggle", { name: row.display_name }),
        render: (row) => <AgentUsageMeters usage={row} />,
      }}
    />
  );
}
