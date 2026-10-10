// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Disclosure, EmptyState } from "../design-system/atoms";
import { CellStack } from "../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { PanelBody, PanelGroupHead } from "../design-system/panel";
import { formatDayMonth, formatMoney, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../i18n";
import { tierLabel, tierRank } from "./ai-decision-labels";
import { TaskName } from "./ai-task-name";

type AiUsage = components["schemas"]["AiUsage"];
type UsageTask = AiUsage["days"][number]["tasks"][number];
type UsageDay = AiUsage["days"][number];

/** One line of the spend table: a task's total, or one tier under a task run on several. */
export type SpendRow = Readonly<{
  key: string;
  task: string;
  name: string;
  summary?: string;
  // The tier a single-tier task ran on, or the tier of a subrow.
  tier?: string;
  subrow: boolean;
  calls: number;
  cached: number;
  tokensIn: number;
  tokensOut: number;
  // Absent when the server priced none of the calls behind the line.
  cost?: number;
}>;

export function aggregate(days: AiUsage["days"]): UsageTask[] {
  const rows = new Map<string, UsageTask>();
  for (const day of days) {
    for (const task of day.tasks) {
      const key = `${task.task}\u0000${task.tier}`;
      const current = rows.get(key);
      if (!current) {
        rows.set(key, { ...task });
        continue;
      }
      current.calls += task.calls;
      current.cached_hits =
        (current.cached_hits ?? 0) + (task.cached_hits ?? 0);
      current.tokens_in += task.tokens_in;
      current.tokens_out += task.tokens_out;
      current.unpriced_calls =
        (current.unpriced_calls ?? 0) + (task.unpriced_calls ?? 0);
      if (task.cost_est_minor !== undefined) {
        current.cost_est_minor =
          (current.cost_est_minor ?? 0) + task.cost_est_minor;
      }
    }
  }
  return [...rows.values()];
}

function line(task: UsageTask, subrow: boolean): SpendRow {
  return {
    key: `${task.task}\u0000${task.tier}`,
    task: task.task,
    name: task.task_display_name ?? task.task,
    summary: task.task_summary,
    tier: task.tier,
    subrow,
    calls: task.calls,
    cached: task.cached_hits ?? 0,
    tokensIn: task.tokens_in,
    tokensOut: task.tokens_out,
    cost: task.cost_est_minor,
  };
}

function sum(
  lines: readonly UsageTask[],
  pick: (line: UsageTask) => number,
): number {
  return lines.reduce((total, entry) => total + pick(entry), 0);
}

function total(tiers: readonly UsageTask[]): SpendRow {
  const first = tiers[0];
  const priced = tiers.filter((tier) => tier.cost_est_minor !== undefined);
  return {
    key: first.task,
    task: first.task,
    name: first.task_display_name ?? first.task,
    summary: first.task_summary,
    subrow: false,
    calls: sum(tiers, (tier) => tier.calls),
    cached: sum(tiers, (tier) => tier.cached_hits ?? 0),
    tokensIn: sum(tiers, (tier) => tier.tokens_in),
    tokensOut: sum(tiers, (tier) => tier.tokens_out),
    cost:
      priced.length === 0
        ? undefined
        : sum(priced, (tier) => tier.cost_est_minor ?? 0),
  };
}

/**
 * The month's lines grouped by task, the costliest first. A task run on one
 * tier is one line; one run on several is its total with a line per tier under it.
 */
export function spendRows(lines: readonly UsageTask[]): SpendRow[] {
  const byTask = new Map<string, UsageTask[]>();
  for (const entry of lines) {
    byTask.set(entry.task, [...(byTask.get(entry.task) ?? []), entry]);
  }
  const groups = [...byTask.values()].map((tiers) => {
    const ordered = [...tiers].sort(
      (a, b) => tierRank(a.tier) - tierRank(b.tier),
    );
    return ordered.length === 1
      ? [line(ordered[0], false)]
      : [total(ordered), ...ordered.map((tier) => line(tier, true))];
  });
  groups.sort(([a], [b]) => (b.cost ?? 0) - (a.cost ?? 0) || b.calls - a.calls);
  return groups.flat();
}

function spendColumns(
  showCost: boolean,
  currency: string,
  locale: Locale,
  t: Translator,
): DataTableColumn<SpendRow>[] {
  const columns: DataTableColumn<SpendRow>[] = [
    {
      key: "task",
      header: t("aiusage.col.task"),
      grow: true,
      render: (row) =>
        row.subrow ? (
          <span className="aiusage-subrow">
            <span className="sr-only">{row.name}: </span>
            {tierLabel(row.tier ?? "", t)}
          </span>
        ) : (
          <span className="aiusage-task">
            <CellStack>
              <TaskName name={row.name} summary={row.summary} />
              {row.tier !== undefined && (
                <span className="t-caption">{tierLabel(row.tier, t)}</span>
              )}
            </CellStack>
          </span>
        ),
    },
    {
      key: "calls",
      header: t("aiusage.col.calls"),
      align: "end",
      render: (row) => formatNumber(row.calls, locale),
    },
    {
      key: "tokens",
      header: t("aiusage.col.tokens"),
      align: "end",
      render: (row) =>
        `${formatNumber(row.tokensIn, locale)} / ${formatNumber(row.tokensOut, locale)}`,
    },
    {
      key: "cached",
      header: t("aiusage.col.cached"),
      align: "end",
      render: (row) => formatNumber(row.cached, locale),
    },
  ];
  if (!showCost) {
    return columns;
  }
  return [
    ...columns,
    {
      key: "cost",
      header: t("aiusage.col.cost"),
      align: "end",
      // A line the server did not price is not a line that cost nothing.
      render: (row) =>
        row.cost === undefined ? "—" : formatMoney(row.cost, currency, locale),
    },
  ];
}

export function SpendByTask({
  rows,
  showCost,
  currency,
  note,
}: Readonly<{
  rows: readonly SpendRow[];
  showCost: boolean;
  currency: string;
  note?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <>
      <PanelGroupHead title={t("aiusage.spendLabel")} level="h3" />
      {rows.length === 0 ? (
        <PanelBody>
          <EmptyState>{t("aiusage.empty")}</EmptyState>
        </PanelBody>
      ) : (
        <DataTable
          bleed
          stickyFirst
          label={t("aiusage.spendLabel")}
          columns={spendColumns(showCost, currency, locale, t)}
          rows={[...rows]}
          rowKey={(row) => row.key}
          rowTestId={(row) => `spend-${row.key.replace("\u0000", "-")}`}
        />
      )}
      {note !== undefined && (
        <PanelBody>
          <p className="t-caption">{note}</p>
        </PanelBody>
      )}
    </>
  );
}

// Diagnostic, so it waits behind its own summary.
export function CallsByDay({ days }: Readonly<{ days: readonly UsageDay[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  return (
    <PanelBody>
      <Disclosure summary={t("aiusage.days.show")}>
        <DataTable<UsageDay>
          label={t("aiusage.days.label")}
          rows={[...days]}
          rowKey={(day) => day.date}
          columns={[
            {
              key: "day",
              header: t("aiusage.days.col.day"),
              render: (day) => (
                <time dateTime={day.date}>
                  {formatDayMonth(day.date, locale, zone)}
                </time>
              ),
            },
            {
              key: "calls",
              header: t("aiusage.col.calls"),
              align: "end",
              render: (day) =>
                formatNumber(
                  day.tasks.reduce((calls, task) => calls + task.calls, 0),
                  locale,
                ),
            },
          ]}
        />
      </Disclosure>
    </PanelBody>
  );
}
