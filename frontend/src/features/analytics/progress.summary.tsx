// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "./analytics.css";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { WrittenBy } from "../../components/citations.list";
import { AiPending } from "../../design-system/aipending";
import { Button, StatCard } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody } from "../../design-system/panel";
import { StatStrip } from "../../design-system/statstrip";
import { formatAnalyticsMoney, formatDateTime } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { analyticsSectionHref } from "../../lib/analytics.address";
import { unwrap } from "../../lib/problem";
import { OutlookProse, readableAssessment } from "./analytics.assessment";
import { scopeKey } from "./analytics.context";
import { landingCaveatLine } from "./analytics.forecast.landing";
import type { ProgressAsk, SalesProgress } from "./progress.queries";
import { ProgressDelta } from "./progress.result";

// "analytics" heads the key, so turning individual performance off drops the
// summary with the figures it was written from. The figures it rests on are
// in the key, so a reload of the page that changes none of them asks nothing.
function useProgressSummary(ask: ProgressAsk, progress: SalesProgress) {
  const [refresh, setRefresh] = useState(0);
  const { locale } = useLocale();
  const query = useQuery({
    queryKey: [
      "analytics",
      "progress-summary",
      ask,
      progress.closed.amount_minor,
      progress.goal.amount_minor,
      progress.planned.amount_minor,
      locale,
      refresh,
    ],
    queryFn: async ({ signal }) => {
      const result = await api.POST("/analytics/progress/summary", {
        body: {
          period: ask.period,
          year: ask.year,
          month: ask.period === "month" ? ask.month : undefined,
          scope_kind: ask.scope?.kind,
          scope_id: ask.scope?.id,
          refresh: refresh > 0,
        },
        signal,
      });
      // absent-404: a server from before this route has no summary.
      if (result.response.status === 404) return null;
      return unwrap(result);
    },
    retry: false,
    // The last summary stays while its own reading is asked again, and never
    // stands in for another period, team member or team.
    placeholderData: (previous, last) =>
      JSON.stringify(last?.queryKey[2]) === JSON.stringify(ask)
        ? previous
        : undefined,
    staleTime: 60_000,
  });
  return { query, refresh: () => setRefresh((n) => n + 1) };
}

/**
 * A few written sentences on where the reading stands against its goal and
 * what is still open. Without a model, or with nothing to say, it draws
 * nothing and the page's figures stand alone.
 */
export function ProgressSummary({
  ask,
  progress,
}: Readonly<{ ask: ProgressAsk; progress: SalesProgress }>) {
  const t = useT();
  const { locale } = useLocale();
  const { query, refresh } = useProgressSummary(ask, progress);
  const summary = query.data;
  const written =
    summary?.generated_by === "model" &&
    summary.sentences.length > 0 &&
    readableAssessment(summary, locale);
  if (summary && written)
    return (
      <Panel
        title={t("progress.summary")}
        tone="ai"
        flatHead
        className="analytics-outlook"
        titleAction={
          <Button variant="ghost" onClick={refresh} pending={query.isFetching}>
            {t("progress.summary.refresh")}
          </Button>
        }
        footer={<WrittenBy by={summary.generated_by} />}
      >
        <PanelBody>
          <OutlookProse
            assessment={summary}
            more={t("progress.summary.more")}
          />
        </PanelBody>
      </Panel>
    );
  if (!query.isFetching && !query.isError) return null;
  return (
    <Panel title={t("progress.summary")} flatHead>
      <PanelBody>
        {query.isFetching ? (
          <AiPending label={t("progress.summary.writing")} lines={3} />
        ) : (
          <ErrorLine
            error={query.error}
            actions={
              <Button variant="ghost" onClick={refresh}>
                {t("analytics.assessment.retry")}
              </Button>
            }
          />
        )}
      </PanelBody>
    </Panel>
  );
}

type Scenarios = NonNullable<SalesProgress["scenarios"]>;

/**
 * Where a month may finish against its goal: what is won, the forecast
 * landing, and every open deal closing. The server states each case and its
 * difference to the goal; this only formats them. A year has no forecast to
 * read them from, and says so.
 */
export function ProgressScenarios({
  progress,
}: Readonly<{ progress: SalesProgress }>) {
  const t = useT();
  const { locale } = useLocale();
  const { scenarios, currency } = progress;
  if (progress.period.kind === "year")
    return (
      <p className="t-caption">{t("progress.scenarios.yearUnavailable")}</p>
    );
  if (!scenarios) return null;
  // The forecast page reads the current month only, and lists the deals each
  // case is made of.
  const href =
    progress.state === "current"
      ? analyticsSectionHref(
          "forecast",
          new Map([
            ["scope", scopeKey(progress.scope)],
            ["pipeline", "all"],
            ["period", "this_month"],
          ]),
        )
      : undefined;
  const card = (label: string, scenario: Scenarios["worst"], why: string) => (
    <StatCard
      label={label}
      value={formatAnalyticsMoney(scenario.amount_minor, currency, locale)}
      detail={
        <>
          <span>{why}</span>
          <ProgressDelta
            delta={scenario.delta_minor}
            currency={currency}
            open={false}
          />
        </>
      }
      href={href}
      opens="forecast"
    />
  );
  return (
    <StatStrip
      label={t("progress.scenarios")}
      floor={<ScenarioFloor scenarios={scenarios} progress={progress} />}
    >
      {card(
        t("progress.scenario.worst"),
        scenarios.worst,
        t("progress.scenario.worstWhy"),
      )}
      {card(
        t("progress.scenario.expected"),
        scenarios.expected,
        t(`forecast.basis.${scenarios.landing.measure}`),
      )}
      {card(
        t("progress.scenario.best"),
        scenarios.best,
        t("progress.scenario.bestWhy"),
      )}
    </StatStrip>
  );
}

// When the deals were read, and what the cases leave out.
function ScenarioFloor({
  scenarios,
  progress,
}: Readonly<{ scenarios: Scenarios; progress: SalesProgress }>) {
  const t = useT();
  const { locale } = useLocale();
  const missing = scenarios.eligible_count - scenarios.priced_count;
  const caveat = landingCaveatLine(
    scenarios.landing,
    (amount) => formatAnalyticsMoney(amount, progress.currency, locale),
    t,
  );
  return (
    <p className="t-caption">
      {t("progress.scenarios.asOf", {
        at: formatDateTime(scenarios.as_of, locale, progress.timezone),
      })}
      {caveat && ` · ${caveat}`}
      {(missing > 0 || scenarios.fx_missing_count > 0) &&
        ` · ${t("progress.scenarios.unpriced")}`}
      {scenarios.scope_limited && ` · ${t("progress.scenarios.limited")}`}
    </p>
  );
}
