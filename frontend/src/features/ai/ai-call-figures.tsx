// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components, operations } from "../../api/schema";
import { AppLink } from "../../design-system/applink";
import { Badge, EmptyState, SegmentedControl } from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { Heading } from "../../design-system/heading";
import { formatDecimal, formatMoney, formatNumber } from "../../format/format";
import {
  type Locale,
  type PluralTranslator,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import { useCan } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import { callsHrefFor } from "./aicalls";
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
      return unwrap(
        await api.GET("/ai/call-stats", {
          params: { query },
        }),
      );
    },
  });
}

export function useTaskFlow(task: string, window: Window, enabled: boolean) {
  return useQuery({
    queryKey: ["ai-task-flow", task, window],
    enabled,
    queryFn: async () => {
      return unwrap(
        await api.GET("/ai/call-stats/flow", {
          params: { query: { task, window } },
        }),
      );
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

/** The seven-day line under a provider; one with no calls draws nothing. */
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
  if (!row) return null;
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

/** How many of total answered calls the provider answered with another model than the binding. */
export function switchedSentence(
  switched: number,
  total: number,
  model: string,
  plural: PluralTranslator,
  locale: Locale,
): string {
  return plural("aiFigures.switched", switched, {
    count: formatNumber(switched, locale),
    total: formatNumber(total, locale),
    model,
  });
}

/** This week's figures for one tier, from the read every tier line shares. */
function useTierWeek(tier: string): StatsRow | undefined {
  const canSee = useCan("ai_diagnostics", "read");
  const stats = useCallStats({ window: "7d", group: "tier" }, canSee);
  return canSee ? stats.data?.rows.find((r) => r.key === tier) : undefined;
}

/** A tier the provider answered with another model this week, as a badge. */
export function TierSwitchedBadge({ tier }: Readonly<{ tier: string }>) {
  const t = useT();
  const row = useTierWeek(tier);
  if (!row?.switched) return null;
  return <Badge tone="warning">{t("aiFigures.switchedBadge")}</Badge>;
}

/** Which model the provider answered a tier's calls with this week, if not the bound one. */
export function TierSwitchedLine({
  tier,
  linked,
}: Readonly<{ tier: string; linked?: boolean }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const row = useTierWeek(tier);
  if (!row?.switched || !row.switched_to) return null;
  return (
    <p className="t-caption" role="status">
      {switchedSentence(
        row.switched,
        row.calls - row.failed,
        row.switched_to,
        plural,
        locale,
      )}
      {linked && (
        <>
          {" "}
          <AppLink
            className="link-button"
            href={callsHrefFor({ tier, served_model: row.switched_to })}
          >
            {t("aiTasks.viewCalls")}
          </AppLink>
        </>
      )}
    </p>
  );
}

type Grouping = "host" | "model" | "tier";

// "By model" is the model that answered, so a provider's switch shows as its own row.
const GROUP_OF: Record<Grouping, Group> = {
  host: "served_provider",
  model: "served_model",
  tier: "tier",
};

/** A provider sheet's Recent calls: by host on a broker, by model or tier. */
export function ProviderRecentCalls({
  provider,
  broker,
}: Readonly<{ provider: string; broker: boolean }>) {
  const t = useT();
  const canSee = useCan("ai_diagnostics", "read");
  const groupings: readonly [Grouping, ...Grouping[]] = broker
    ? ["host", "model", "tier"]
    : ["model", "tier"];
  // Unchosen follows the broker as the host becomes known.
  const [chosen, setChosen] = useState<Grouping | null>(null);
  const by = chosen && groupings.includes(chosen) ? chosen : groupings[0];
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
                ? { served_model: row.key }
                : { tier: row.key }),
          })
        }
      />
      <p className="t-caption">{t("aiFigures.openRow")}</p>
    </section>
  );
}

/**
 * The binding dialog's Recent calls: each host that served the tier this week
 * on the broker, and each model elsewhere, where the vendor is the host.
 */
export function TierRecentCalls({
  tier,
  broker,
}: Readonly<{ tier: string; broker: boolean }>) {
  const t = useT();
  const canSee = useCan("ai_diagnostics", "read");
  const by: Grouping = broker ? "host" : "model";
  const stats = useCallStats(
    { window: "7d", group: GROUP_OF[by], tier },
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
          <AppLink className="link-button" href={callsHrefFor({ tier })}>
            {t("aiTasks.viewCalls")}
          </AppLink>
        </span>
      </div>
      <TierSwitchedLine tier={tier} linked />
      <FiguresTable
        rows={stats.data?.rows}
        failed={stats.isError}
        keyHeader={t(
          by === "host" ? "aiFigures.by.host" : "aiFigures.by.model",
        )}
        hrefFor={(row) =>
          callsHrefFor(
            by === "host"
              ? { tier, served_provider: row.key }
              : { tier, served_model: row.key },
          )
        }
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
          // A row no host served has no value to filter the log by, so it is
          // named and not linked: a link would open every call on the lane.
          // A row whose calls all failed had no host answer; one with answers
          // is a lane, embeddings among them, whose broker names no host.
          render: (row) =>
            row.key ? (
              <AppLink href={hrefFor(row)}>{row.key}</AppLink>
            ) : row.failed === row.calls ? (
              t("aiFigures.noHost")
            ) : (
              t("aiFigures.hostUnrecorded")
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
            // ds:ignore a failure count in the danger ink, not a message
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
            // ds:ignore a failure count in the danger ink, not a message
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
          // A cost with unpriced calls in it is a floor, and says so.
          render: (row) =>
            row.unpriced === row.calls
              ? "—"
              : row.unpriced
                ? t("aiFigures.costAtLeast", {
                    // Floored to the cent: a rounded-up subtotal is no lower bound.
                    cost: formatMicroUsd(
                      Math.floor(row.cost_microusd / 10_000) * 10_000,
                      locale,
                    ),
                  })
                : formatMicroUsd(row.cost_microusd, locale),
        },
      ]}
    />
  );
}
