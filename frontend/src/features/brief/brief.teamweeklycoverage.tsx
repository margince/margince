// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Gauge, ShieldCheck } from "lucide-react";
import { Badge, StatCard } from "../../design-system/atoms";
import { PanelBody, PanelRow } from "../../design-system/panel";
import { StatStrip } from "../../design-system/statstrip";
import { formatNumber, formatPercent } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { labelFor } from "../../lib/labelfor";
import { ReportCard } from "./brief.reportcard";
import type { TeamWeeklyReview } from "./teamweekly.queries";

type Coverage = NonNullable<TeamWeeklyReview["coverage"]>[number];

/** How well the team kept its records over the week, frozen with it. */
export function TeamQuality({
  review,
}: Readonly<{ review: TeamWeeklyReview }>) {
  const t = useT();
  const { locale } = useLocale();
  const quality = review.quality;
  if (quality === undefined) return null;
  // Absent is "not measured", never zero: an older snapshot did not count.
  const figure = (value: number | undefined) =>
    value === undefined
      ? t("teamweekly.quality.notMeasured")
      : formatNumber(value, locale);
  const cards: [MessageKey, number | undefined][] = [
    ["teamweekly.quality.accepted", quality.proposals_accepted],
    ["teamweekly.quality.rejected", quality.proposals_rejected],
    ["teamweekly.quality.reversals", quality.auto_change_reversals],
  ];
  return (
    <ReportCard icon={ShieldCheck} title={t("teamweekly.quality.title")}>
      <PanelBody>
        <StatStrip testId="teamweekly-quality">
          {cards.map(([label, value]) => (
            <StatCard
              key={label}
              narrow="row"
              label={t(label)}
              value={figure(value)}
              detail={t("teamweekly.quality.basis")}
            />
          ))}
        </StatStrip>
      </PanelBody>
    </ReportCard>
  );
}

/**
 * Whether the team's open deals covered what the month and the quarter need,
 * always read against the basis it names: the product has no target.
 */
export function TeamCoverage({
  review,
}: Readonly<{ review: TeamWeeklyReview }>) {
  const t = useT();
  const coverage = review.coverage ?? [];
  if (coverage.length === 0) return null;
  return (
    <ReportCard
      icon={Gauge}
      title={t("teamweekly.coverage.title")}
      aside={
        review.replan_recommended ? (
          <Badge tone="warning">{t("teamweekly.coverage.replan")}</Badge>
        ) : undefined
      }
    >
      {coverage.map((horizon) => (
        <PanelRow key={horizon.period_kind}>
          <CoverageRow horizon={horizon} />
        </PanelRow>
      ))}
    </ReportCard>
  );
}

const HORIZON_LABEL: Readonly<Record<Coverage["period_kind"], MessageKey>> = {
  month: "teamweekly.coverage.month",
  quarter: "teamweekly.coverage.quarter",
};

const ABSENT_LABEL: Readonly<
  Record<NonNullable<Coverage["absent"]>, MessageKey>
> = {
  insufficient_basis: "forecast.pipelineAbsent.insufficient_basis",
  insufficient_history: "forecast.pipelineAbsent.insufficient_history",
  not_authorized: "teamweekly.coverage.notAuthorized",
};

const BASIS_WORDS: Readonly<
  Record<NonNullable<Coverage["basis"]>, MessageKey>
> = {
  manager_call: "teamweekly.coverage.basis.manager_call",
  historical_median: "teamweekly.coverage.basis.historical_median",
};

const BASIS_WHY: Readonly<Record<NonNullable<Coverage["basis"]>, MessageKey>> =
  {
    manager_call: "forecast.pipelineBasisWhy.manager_call",
    historical_median: "forecast.pipelineBasisWhy.historical_median",
  };

function CoverageRow({ horizon }: Readonly<{ horizon: Coverage }>) {
  const t = useT();
  const { locale } = useLocale();
  const name = t(labelFor(HORIZON_LABEL, horizon.period_kind));
  if (horizon.basis === undefined || horizon.coverage_bp === undefined) {
    return (
      <span className="teamweekly-row">
        <strong>{name}</strong>
        <span className="t-sub">
          {t(labelFor(ABSENT_LABEL, horizon.absent ?? "insufficient_basis"))}
        </span>
      </span>
    );
  }
  const basis = t(labelFor(BASIS_WORDS, horizon.basis));
  return (
    <span className="teamweekly-row">
      <strong>{name}</strong>
      <Badge tone={horizon.replan_recommended ? "warning" : "success"}>
        {horizon.replan_recommended
          ? t("teamweekly.coverage.atRisk", { basis })
          : t("teamweekly.coverage.holds", { basis })}
      </Badge>
      <span>
        {t("teamweekly.coverage.percent", {
          percent: formatPercent(horizon.coverage_bp / 10_000, locale),
        })}
      </span>
      <span className="t-sub">{t(labelFor(BASIS_WHY, horizon.basis))}</span>
    </span>
  );
}
