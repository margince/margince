// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { DataTable } from "../../design-system/datatable";
import { Heading } from "../../design-system/heading";
import { Panel } from "../../design-system/panel";
import { isoWeekOf } from "../../format/calendarday";
import { formatNumber } from "../../format/format";
import { type Translator, useLocale, useT } from "../../i18n";
import type { WeeklyReview } from "./brief.queries";

import "./brief.weekly.css";

// How well the week went, beside what happened in it: one table, read a row at
// a time, under the week's figures.
//
// Each block is drawn only when the server sent it. An absent block is not a
// block of zeros: a rep who carried no leads did not score zero on them.

type Scorecard = NonNullable<WeeklyReview["scorecard"]>;
type LeadBlock = NonNullable<Scorecard["lead"]>;
type DealBlock = NonNullable<Scorecard["deal"]>;

type ScoreRow = Readonly<{
  key: string;
  label: string;
  value: string;
  basis: string;
}>;

export function ScorecardPanel({
  scorecard,
  weekStart,
}: Readonly<{ scorecard: Scorecard | undefined; weekStart: string }>) {
  const t = useT();
  const { locale } = useLocale();
  // A review written before scorecards existed carries none, and draws none.
  if (!scorecard) return null;
  const { lead, deal } = scorecard;
  if (!lead && !deal) return null;
  const n = (value: number) => formatNumber(value, locale);
  const rows = [
    ...(lead ? leadRows(lead, t, n) : []),
    ...(deal ? dealRows(deal, t, n) : []),
  ];
  return (
    <>
      <Heading size="small" as="h3">
        {t("brief.weekly.scorecard.title")}
      </Heading>
      <Panel>
        <DataTable
          bleed
          label={t("brief.weekly.scorecard.title")}
          rowKey={(row: ScoreRow) => row.key}
          rows={rows}
          columns={[
            {
              key: "label",
              header: t("brief.report.col.figure"),
              render: (row) => row.label,
            },
            {
              key: "value",
              header: t("brief.report.col.week", {
                week: n(isoWeekOf(weekStart)),
              }),
              align: "end",
              render: (row) => row.value,
            },
            {
              key: "basis",
              header: t("brief.weekly.scorecard.basis"),
              grow: true,
              render: (row) => <span className="t-caption">{row.basis}</span>,
            },
          ]}
        />
      </Panel>
    </>
  );
}

function leadRows(
  block: LeadBlock,
  t: Translator,
  n: (value: number) => string,
): ScoreRow[] {
  return [
    {
      key: "advanced",
      label: t("brief.weekly.scorecard.advanced"),
      value: n(block.advanced),
      basis: t("brief.weekly.scorecard.advancedBasis"),
    },
    {
      // The breach is the figure, so the week's one actionable number leads.
      key: "breached",
      label: t("brief.weekly.scorecard.answeredInTarget"),
      value: n(block.breached),
      basis: responseBasis(block, t, n),
    },
    {
      key: "meetings",
      label: t("brief.weekly.scorecard.meetingsHeld"),
      value: n(block.meetings_held),
      basis: t("brief.weekly.scorecard.meetingsBasis", {
        booked: n(block.meetings_booked),
        noShow: n(block.meetings_no_show),
      }),
    },
    // Only when non-zero: it says the counts above are a floor.
    ...(block.meetings_partial_history > 0
      ? [
          {
            key: "partial",
            label: t("brief.weekly.scorecard.partialHistory"),
            value: n(block.meetings_partial_history),
            basis: t("brief.weekly.scorecard.partialHistoryBasis"),
          },
        ]
      : []),
  ];
}

/**
 * What the week's response figures rest on. Zero breaches never means every
 * lead was answered: a lead nobody answered and nobody stamped also breaches
 * nothing. `answered_in_target` counts responses recorded by the close, late
 * ones included, so this line names recorded responses and never a target met.
 *
 * `arrived` is the cohort both figures are read from. Missing, it is a week
 * frozen before the cohort was recorded, and the line says so rather than
 * guessing how many went unanswered.
 */
function responseBasis(
  block: LeadBlock,
  t: Translator,
  n: (value: number) => string,
): string {
  const arrived = block.arrived;
  if (arrived === undefined || arrived === null) {
    return t("brief.weekly.scorecard.respondedNoCohort", {
      count: n(block.answered_in_target),
    });
  }
  if (arrived === 0) return t("brief.weekly.scorecard.noArrivals");
  // A late answer is in both counts, so only responses are subtracted.
  const unanswered = Math.max(arrived - block.answered_in_target, 0);
  return unanswered === 0
    ? t("brief.weekly.scorecard.allResponded", { total: n(arrived) })
    : t("brief.weekly.scorecard.unanswered", {
        count: n(unanswered),
        total: n(arrived),
      });
}

function dealRows(
  block: DealBlock,
  t: Translator,
  n: (value: number) => string,
): ScoreRow[] {
  // A week with no open deal has no denominator, and "0 of 0" states a
  // coverage nobody could have. All three shares say so the same way.
  const ofOpen =
    block.open === 0
      ? t("brief.weekly.scorecard.noOpen")
      : t("brief.weekly.scorecard.ofOpen", { total: n(block.open) });
  return [
    {
      key: "advances",
      label: t("brief.weekly.scorecard.advances"),
      value: n(block.advances),
      basis: t("brief.weekly.scorecard.regressionsDetail", {
        count: n(block.regressions),
      }),
    },
    // A median of nothing is not zero days, so the row is left out.
    ...(block.median_days_in_stage != null
      ? [
          {
            key: "median",
            label: t("brief.weekly.scorecard.medianDaysInStage"),
            value: n(block.median_days_in_stage),
            basis: t("brief.weekly.scorecard.medianBasis"),
          },
        ]
      : []),
    {
      key: "nextStep",
      label: t("brief.weekly.scorecard.withNextStep"),
      value: n(block.with_next_step),
      basis: ofOpen,
    },
    {
      key: "multi",
      label: t("brief.weekly.scorecard.multiThreaded"),
      value: n(block.multi_threaded),
      basis:
        block.open === 0
          ? t("brief.weekly.scorecard.noOpen")
          : t("brief.weekly.scorecard.multiThreadedBasis", {
              total: n(block.open),
            }),
    },
    {
      key: "closeDate",
      label: t("brief.weekly.scorecard.closeDateSound"),
      value: n(block.close_date_sound),
      basis: ofOpen,
    },
    {
      key: "forecast",
      label: t("brief.weekly.scorecard.forecastMoves"),
      value: n(block.forecast_up),
      basis: t("brief.weekly.scorecard.forecastMovesBasis", {
        down: n(block.forecast_down),
      }),
    },
    // Only when non-zero: a caveat the reader needs, never a reassurance.
    ...(block.unreconstructible != null && block.unreconstructible > 0
      ? [
          {
            key: "unreconstructible",
            label: t("brief.weekly.scorecard.unreconstructible"),
            value: n(block.unreconstructible),
            basis: t("brief.weekly.scorecard.unreconstructibleBasis"),
          },
        ]
      : []),
  ];
}
