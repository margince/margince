// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components, operations } from "../api/schema";
import { useCan } from "../app/capability";
import { EmptyState, SegmentedControl } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { Heading } from "../design-system/heading";
import { formatDecimal, formatMoney, formatNumber } from "../format/format";
import {
  type Locale,
  type PluralTranslator,
  useLocale,
  usePlural,
  useT,
} from "../i18n";
import { callsHrefFor } from "./aicalls";
import { throwProblem } from "./common";
import "./ai-settings.css";

// What a provider, a tier or a host actually did, read from the call record
// health and the trace already read. Every figure here is one GET
// /ai/call-stats; the server owns what counts as failed, so a tier the health
// dot shows failing is failing here too.

type StatsQuery = NonNullable<
  operations["getAiCallStats"]["parameters"]["query"]
>;
type StatsRow = components["schemas"]["AiCallStatsRow"];
export type Window = NonNullable<StatsQuery["window"]>;
type Group = NonNullable<StatsQuery["group"]>;

export const WINDOWS: readonly Window[] = ["24h", "7d", "30d"];

export function useCallStats(query: StatsQuery, enabled: boolean) {
  return useQuery({
    queryKey: ["ai-call-stats", query],
    enabled,
    queryFn: async () => {
      const { data, error } = await api.GET("/ai/call-stats", {
        params: { query },
      });
      if (error) throwProblem(error);
      return data;
    },
  });
}

export function useTaskFlow(task: string, window: Window, enabled: boolean) {
  return useQuery({
    queryKey: ["ai-task-flow", task, window],
    enabled,
    queryFn: async () => {
      const { data, error } = await api.GET("/ai/call-stats/flow", {
        params: { query: { task, window } },
      });
      if (error) throwProblem(error);
      return data;
    },
  });
}

/** A latency in seconds, one decimal under ten. */
export function formatSeconds(ms: number, locale: Locale): string {
  const seconds = ms / 1000;
  return `${formatDecimal(seconds, locale, seconds < 10 ? 1 : 0)} s`;
}

/** A cost the server reports in micro-USD, in cents' precision. */
export function formatMicroUsd(micro: number, locale: Locale): string {
  return formatMoney(Math.round(micro / 10_000), "USD", locale);
}

export function useWindowLabels(): Record<Window, string> {
  const t = useT();
  return {
    "24h": t("aiFigures.window.24h"),
    "7d": t("aiFigures.window.7d"),
    "30d": t("aiFigures.window.30d"),
  };
}

/** The seven-day line under a provider on the Providers card. */
export function ProviderCallsLine({
  provider,
}: Readonly<{ provider: string }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const canSee = useCan("ai_diagnostics", "read");
  const stats = useCallStats({ window: "7d", group: "provider" }, canSee);
  if (!canSee || !stats.data) return null;
  const row = stats.data.rows.find((r) => r.key === provider);
  if (!row)
    return <span className="t-caption">{t("aiFigures.line.none")}</span>;
  return (
    <span className="t-caption">
      {t("aiFigures.line.prefix", {
        sentence: callsSentence(row, t, plural, locale),
      })}
    </span>
  );
}

export function callsSentence(
  row: StatsRow,
  t: ReturnType<typeof useT>,
  plural: PluralTranslator,
  locale: Locale,
): string {
  const count = (n: number) => ({ count: formatNumber(n, locale) });
  const calls = plural("aiFigures.line.calls", row.calls, count(row.calls));
  if (!row.failed) return `${calls} · ${t("aiFigures.line.noneFailed")}`;
  const failed = plural("aiFigures.line.failed", row.failed, count(row.failed));
  const timeouts = row.timeouts
    ? ` ${plural("aiFigures.line.timeouts", row.timeouts, count(row.timeouts))}`
    : "";
  return `${calls} · ${failed}${timeouts}`;
}

/** The seven-day line inside a tier's health popover. */
export function TierCallsLine({
  tier,
  sort,
}: Readonly<{ tier: string; sort?: string }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const canSee = useCan("ai_diagnostics", "read");
  const stats = useCallStats({ window: "7d", group: "tier" }, canSee);
  if (!canSee || !stats.data) return null;
  const row = stats.data.rows.find((r) => r.key === tier);
  const parts = row
    ? [
        plural("aiFigures.line.calls", row.calls, {
          count: formatNumber(row.calls, locale),
        }),
        t("aiFigures.line.p50", { latency: formatSeconds(row.p50_ms, locale) }),
        ...(row.timeouts
          ? [
              plural("aiFigures.line.timeoutCount", row.timeouts, {
                count: formatNumber(row.timeouts, locale),
              }),
            ]
          : []),
      ]
    : [t("aiFigures.line.none")];
  if (sort) parts.push(t("aiFigures.line.sort", { sort }));
  return (
    <span className="t-caption">
      {t("aiFigures.line.week", { figures: parts.join(" · ") })}
    </span>
  );
}

type Grouping = "host" | "model" | "tier";

const GROUP_OF: Record<Grouping, Group> = {
  host: "served_provider",
  model: "model",
  tier: "tier",
};

/** A provider sheet's Recent calls: by host on a broker, by model or tier. */
export function ProviderRecentCalls({
  provider,
  broker,
}: Readonly<{ provider: string; broker: boolean }>) {
  const t = useT();
  const canSee = useCan("ai_diagnostics", "read");
  const groupings: Grouping[] = broker
    ? ["host", "model", "tier"]
    : ["model", "tier"];
  const [chosen, setChosen] = useState<Grouping>(groupings[0]);
  const by = groupings.includes(chosen) ? chosen : groupings[0];
  const [window, setWindow] = useState<Window>("7d");
  const windows = useWindowLabels();
  const stats = useCallStats({ window, group: GROUP_OF[by], provider }, canSee);
  if (!canSee) return null;
  const labels: Record<Grouping, string> = {
    host: t("aiFigures.by.host"),
    model: t("aiFigures.by.model"),
    tier: t("aiFigures.by.tier"),
  };
  return (
    <section className="ai-sheet-section ai-figures">
      <div className="ai-figures-head">
        <Heading size="small" className="t-h3">
          {t("aiFigures.recentCalls")}
        </Heading>
        <span className="ai-figures-dials">
          <SegmentedControl
            label={t("aiFigures.groupBy")}
            options={groupings}
            value={by}
            labels={labels}
            onChange={setChosen}
          />
          <SegmentedControl
            label={t("aiFigures.window")}
            options={WINDOWS}
            value={window}
            labels={windows}
            onChange={setWindow}
          />
        </span>
      </div>
      <p className="t-caption">
        {broker ? t("aiFigures.intro.broker") : t("aiFigures.intro")}
      </p>
      <FiguresTable
        rows={stats.data?.rows}
        failed={stats.isError}
        keyHeader={labels[by]}
        hrefFor={(row) =>
          callsHrefFor({
            provider,
            ...(by === "host"
              ? { served_provider: row.key }
              : by === "model"
                ? { model: row.key }
                : { tier: row.key }),
          })
        }
      />
      <p className="t-caption">{t("aiFigures.openRow")}</p>
    </section>
  );
}

/** The binding dialog's Recent calls: each host that served the tier this week. */
export function TierRecentCalls({ tier }: Readonly<{ tier: string }>) {
  const t = useT();
  const canSee = useCan("ai_diagnostics", "read");
  const stats = useCallStats(
    { window: "7d", group: "served_provider", tier },
    canSee,
  );
  if (!canSee) return null;
  return (
    <section className="ai-figures">
      <div className="ai-figures-head">
        <Heading size="small" className="t-h3">
          {t("aiFigures.recentCalls")}
        </Heading>
        <span className="t-caption">
          {t("aiFigures.lastWeek")} ·{" "}
          <a className="link-button" href={callsHrefFor({ tier })}>
            {t("aiTasks.viewCalls")}
          </a>
        </span>
      </div>
      <FiguresTable
        rows={stats.data?.rows}
        failed={stats.isError}
        keyHeader={t("aiFigures.by.host")}
        hrefFor={(row) => callsHrefFor({ tier, served_provider: row.key })}
      />
    </section>
  );
}

function FiguresTable({
  rows,
  failed,
  keyHeader,
  hrefFor,
}: Readonly<{
  rows: StatsRow[] | undefined;
  failed: boolean;
  keyHeader: string;
  hrefFor: (row: StatsRow) => string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  if (failed) return <EmptyState>{t("aiFigures.unread")}</EmptyState>;
  if (!rows) return <EmptyState>{t("aiFigures.pending")}</EmptyState>;
  if (rows.length === 0) return <EmptyState>{t("aiFigures.empty")}</EmptyState>;
  const number = (value: number) => formatNumber(value, locale);
  return (
    <DataTable
      label={t("aiFigures.recentCalls")}
      rows={rows}
      rowKey={(row) => row.key}
      columns={[
        {
          key: "key",
          header: keyHeader,
          render: (row) => (
            <a href={hrefFor(row)}>{row.key || t("aiFigures.noHost")}</a>
          ),
        },
        {
          key: "calls",
          header: t("aiFigures.col.calls"),
          align: "end",
          render: (row) => number(row.calls),
        },
        {
          key: "failed",
          header: t("aiFigures.col.failed"),
          align: "end",
          render: (row) => (
            <span className={row.failed ? "ai-figures-bad" : undefined}>
              {number(row.failed)}
            </span>
          ),
        },
        {
          key: "timeouts",
          header: t("aiFigures.col.timeouts"),
          align: "end",
          render: (row) => (
            <span className={row.timeouts ? "ai-figures-bad" : undefined}>
              {number(row.timeouts)}
            </span>
          ),
        },
        {
          key: "p50",
          header: t("aiFigures.col.p50"),
          align: "end",
          render: (row) => formatSeconds(row.p50_ms, locale),
        },
        {
          key: "p95",
          header: t("aiFigures.col.p95"),
          align: "end",
          render: (row) => formatSeconds(row.p95_ms, locale),
        },
        {
          key: "cost",
          header: t("aiFigures.col.cost"),
          align: "end",
          render: (row) =>
            row.unpriced && !row.cost_microusd
              ? "—"
              : formatMicroUsd(row.cost_microusd, locale),
        },
      ]}
    />
  );
}
