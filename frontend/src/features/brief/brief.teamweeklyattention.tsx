// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { EntityRef } from "../../components/entityref";
import { Badge, StatCard } from "../../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { StatStrip } from "../../design-system/statstrip";
import { formatNumber, formatPercent } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { isEntityKind } from "../../lib/entity";
import {
  checkLabel,
  SeverityBadge,
} from "../analytics/analytics.forecast.review";
import type { TeamWeeklyReview } from "../worklist/teamweekly.queries";

import "./brief.teamweekly.css";

type Coverage = NonNullable<TeamWeeklyReview["coverage"]>[number];

/** The summary paragraph the server composed from the frozen figures. */
export function TeamSummary({
  review,
}: Readonly<{ review: TeamWeeklyReview }>) {
  if (!review.summary) {
    return null;
  }
  return (
    <p className="teamweekly-summary" data-testid="teamweekly-summary">
      {review.summary}
    </p>
  );
}

/**
 * What needs the manager now, read live over the team the week froze.
 *
 * Drawn only when the read carried the lists: an older server sends neither,
 * and an empty panel would claim nothing needs attention.
 */
export function TeamAttentionPanel({
  review,
}: Readonly<{ review: TeamWeeklyReview }>) {
  const t = useT();
  const { locale } = useLocale();
  if (review.exceptions === undefined) {
    return null;
  }
  const exceptions = review.exceptions;
  const stale = review.stale_deals ?? [];
  return (
    <Panel
      title={t("teamweekly.attention.title")}
      titleAction={<Badge tone="info">{t("teamweekly.attention.live")}</Badge>}
    >
      <PanelBody>
        <p className="t-sub">{t("teamweekly.attention.basis")}</p>
        {exceptions.length === 0 && stale.length === 0 && (
          <p>{t("teamweekly.attention.empty")}</p>
        )}
      </PanelBody>
      {exceptions.map((check) => (
        <PanelRow key={check.id}>
          <span className="teamweekly-attention-row">
            <SeverityBadge severity={check.severity} />
            <span>{t(checkLabel(check.type))}</span>
            {check.subject && isEntityKind(check.subject.type) && (
              <EntityRef
                kind={check.subject.type}
                id={check.subject.id}
                name={check.subject.label}
              />
            )}
          </span>
        </PanelRow>
      ))}
      {stale.map((deal) => (
        <PanelRow key={deal.deal_id}>
          <span className="teamweekly-attention-row">
            <EntityRef kind="deal" id={deal.deal_id} name={deal.deal_name} />
            <span className="t-sub">
              {t("teamweekly.attention.quiet", {
                days: formatNumber(deal.days_quiet, locale),
              })}
            </span>
          </span>
        </PanelRow>
      ))}
    </Panel>
  );
}

/** How well the team kept its records over the week, frozen with it. */
export function TeamQuality({
  review,
}: Readonly<{ review: TeamWeeklyReview }>) {
  const t = useT();
  const { locale } = useLocale();
  const quality = review.quality;
  if (quality === undefined) {
    return null;
  }
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
    <Panel title={t("teamweekly.quality.title")}>
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
    </Panel>
  );
}

/**
 * Whether the team's open deals covered what the month and the quarter need.
 *
 * Always read against the basis it names, never as a target: the product has
 * no target to be off.
 */
export function TeamCoveragePanel({
  review,
}: Readonly<{ review: TeamWeeklyReview }>) {
  const t = useT();
  const coverage = review.coverage ?? [];
  if (coverage.length === 0) {
    return null;
  }
  return (
    <Panel
      title={t("teamweekly.coverage.title")}
      titleAction={
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
    </Panel>
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
  const name = t(HORIZON_LABEL[horizon.period_kind]);
  if (horizon.basis === undefined || horizon.coverage_bp === undefined) {
    return (
      <span className="teamweekly-attention-row">
        <strong>{name}</strong>
        <span className="t-sub">
          {t(ABSENT_LABEL[horizon.absent ?? "insufficient_basis"])}
        </span>
      </span>
    );
  }
  const basis = t(BASIS_WORDS[horizon.basis]);
  return (
    <span className="teamweekly-attention-row">
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
      <span className="t-sub">{t(BASIS_WHY[horizon.basis])}</span>
    </span>
  );
}
