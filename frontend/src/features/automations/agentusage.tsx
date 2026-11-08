// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Badge, EmptyState } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import {
  DataTable,
  type DataTableColumn,
  type DataTableDetail,
} from "../../design-system/datatable";
import { Eyebrow } from "../../design-system/eyebrow";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { Meter } from "../../design-system/readings";
import { Select } from "../../design-system/select";
import { StatCard } from "../../design-system/statcard";
import { StatStrip } from "../../design-system/statstrip";
import { formatDateTime, formatNumber } from "../../format/format";
import { UTC_ZONE } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { useCan } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";
import {
  AGENT_COUNTERS,
  type AgentCounter,
  type AgentCounterUsage,
  type AgentUsage,
  type AgentUsageState,
  tightestCounter,
  usedUp,
} from "./agentusage.counters";
import "../settings/settings-agents.css";

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

// Which limit stopped an agent, its ceiling, and when the window lets it go
// on. The calls limit is named first, as the gate asks it first.
function StoppedNote({ usage }: Readonly<{ usage: AgentUsage }>) {
  const t = useT();
  const { locale } = useLocale();
  const counter = (["calls", "egress", "writes"] as const).find((c) =>
    usedUp(usage[c]),
  );
  if (counter === undefined) {
    return null;
  }
  return (
    <Callout tone="danger" title={t("agentUsage.state.stopped")}>
      {t(`agentUsage.stopped.${counter}`, {
        used: formatNumber(usage[counter].used, locale),
        limit: formatNumber(usage[counter].limit, locale),
        date: formatDateTime(usage.resets_at, locale, UTC_ZONE),
      })}
    </Callout>
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
      row.usage ? (
        <>
          <StoppedNote usage={row.usage} />
          <AgentUsageMeters usage={row.usage} />
        </>
      ) : null,
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
              <>
                <UserUsageTable rows={page.data} />
                <AllowanceCheck rows={page.data} />
              </>
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
      render: (row) => (
        <span className="agent-usage-user">
          <span>{row.display_name}</span>
          <span className="t-caption">{roleNames(row, t)}</span>
        </span>
      ),
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

function roleNames(row: UserUsage, t: ReturnType<typeof useT>): string {
  return row.roles.length === 0
    ? t("agentUsage.noRole")
    : row.roles.map((role) => role.role_name).join(", ");
}

// What one user's agents may do per window, count by count, and the role each
// count comes from. That is the highest of their roles, or the installation value.
function AllowanceCheck({ rows }: Readonly<{ rows: readonly UserUsage[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const [chosen, setChosen] = useState(rows[0]?.user_id ?? "");
  const row = rows.find((candidate) => candidate.user_id === chosen) ?? rows[0];
  if (row === undefined) {
    return null;
  }
  const from = (roleKey: string | null) => {
    const role = row.roles.find((r) => r.role_key === roleKey);
    return role === undefined
      ? t("agentUsage.check.fromInstallation")
      : t("agentUsage.check.fromRole", { role: role.role_name });
  };
  return (
    <PanelBody className="form-stack">
      <Eyebrow as="h3">{t("agentUsage.check.title")}</Eyebrow>
      <div className="agent-usage-check">
        <Select
          aria-label={t("agentUsage.check.label")}
          value={row.user_id}
          options={rows.map((r) => ({
            value: r.user_id,
            label: `${r.display_name} · ${roleNames(r, t)}`,
          }))}
          onChange={setChosen}
          testId="allowance-check-user"
        />
      </div>
      <StatStrip testId="allowance-check">
        {(["calls", "writes", "egress"] as const).map((counter) => (
          <StatCard
            key={counter}
            label={t(`agentUsage.counter.${counter}`)}
            value={formatNumber(row.allowance[counter].limit, locale)}
            detail={from(row.allowance[counter].role_key)}
          />
        ))}
      </StatStrip>
      <p className="t-caption">{t("agentUsage.check.note")}</p>
    </PanelBody>
  );
}
