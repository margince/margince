// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Panel } from "../../design-system/panel";
import { isoWeekOf } from "../../format/calendarday";
import {
  formatDayLong,
  formatDayRange,
  formatNumber,
} from "../../format/format";
import { useNow } from "../../format/now";
import { startOfDayInZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { useUrlParams } from "../../lib/urlstate";
import { weeklyNumericStatus } from "./brief.numeric";
import {
  useWeeklyReview,
  useWeeklyReviewIndex,
  useWeeklyWins,
} from "./brief.queries";
import { HeadlineFigures } from "./brief.weekly.figures";

/**
 * Beside the report: the week in figures, the weeks to open, what the week
 * is up against, and when the next report comes.
 */
export function WeeklyRail({
  constraints,
}: Readonly<{ constraints?: ReactNode }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const [params] = useUrlParams();
  const asked = params.get("week") ?? undefined;
  const review = useWeeklyReview(asked).data ?? null;
  const weeks = useWeeklyReviewIndex().data;
  // The next report follows the newest week, whichever week is shown. Until the
  // index loads, only the default view is known to show the newest.
  const newest =
    weeks?.[0] ?? (asked === undefined ? review?.local_week_start : undefined);
  const next = newest ? startOfDayInZone(newest, recordZone, 14) : undefined;
  // A stalled run leaves the newest week behind; a date already past says
  // nothing true about when the next report comes.
  const now = useNow(HOUR_MS);
  // A week with no report still lists the weeks that have one, since the
  // stepper has nowhere to go from it.
  if (review === null) return <WeeksPanel weeks={weeks ?? []} shown={asked} />;
  const numeric = weeklyNumericStatus(review.numeric_summary);
  return (
    <>
      <Panel className="brief-weekly-summary">
        <HeadlineFigures review={review} figures={numeric.figures} />
      </Panel>
      <WeeksPanel weeks={weeks ?? []} shown={review.local_week_start} />
      {constraints}
      {next && Date.parse(next) >= now && (
        <p className="t-caption brief-report-closing">
          {t("brief.report.next", {
            date: formatDayLong(next, locale, recordZone),
          })}
        </p>
      )}
    </>
  );
}

// How many of the newest weeks the side column lists and draws; the stepper
// reaches every older one.
const LISTED_WEEKS = 4;
const CHARTED_WEEKS = 8;
const HOUR_MS = 3_600_000;

/**
 * The newest weeks with a report: what each won as bars, then the weeks as a
 * list with the one shown marked. A press opens a week.
 */
function WeeksPanel({
  weeks,
  shown,
}: Readonly<{ weeks: readonly string[]; shown: string | undefined }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const [params, setParams] = useUrlParams();
  const wins = new Map(
    (useWeeklyWins().data ?? []).map((entry) => [entry.week, entry.deals_won]),
  );
  if (weeks.length < 2) return null;
  return (
    <Panel
      flatHead
      title={t("brief.report.weeks")}
      titleAction={
        wins.size > 0 ? (
          <span className="t-caption">{t("brief.report.wonPerWeek")}</span>
        ) : undefined
      }
    >
      {wins.size > 0 && (
        <WinsChart
          weeks={weeks.slice(0, CHARTED_WEEKS).reverse()}
          wins={wins}
          shown={shown}
        />
      )}
      <ul className="brief-weekly-weeks">
        {weeks.slice(0, LISTED_WEEKS).map((week) => {
          const won = wins.get(week);
          return (
            <li key={week}>
              <button
                type="button"
                className="brief-weekly-week"
                aria-current={week === shown ? "true" : undefined}
                onClick={() => setParams(new Map([...params, ["week", week]]))}
              >
                <span className="brief-weekly-week-name">
                  {t("brief.report.col.week", {
                    week: formatNumber(isoWeekOf(week), locale),
                  })}
                  <span className="t-caption">
                    {formatDayRange(
                      week,
                      startOfDayInZone(week, zone, 6),
                      locale,
                      zone,
                    )}
                  </span>
                </span>
                {won !== undefined && (
                  <span className="t-caption t-num">
                    {plural("brief.report.wonCount", won, {
                      count: formatNumber(won, locale),
                    })}
                  </span>
                )}
              </button>
            </li>
          );
        })}
      </ul>
    </Panel>
  );
}

/** Deals won per week, oldest on the left. It charts more weeks than the
 *  list names, so it carries every charted count as its own label. */
function WinsChart({
  weeks,
  wins,
  shown,
}: Readonly<{
  weeks: readonly string[];
  wins: ReadonlyMap<string, number>;
  shown: string | undefined;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const most = Math.max(1, ...weeks.map((week) => wins.get(week) ?? 0));
  const label = weeks
    .map((week) => {
      const won = wins.get(week) ?? 0;
      return `${t("brief.report.col.week", {
        week: formatNumber(isoWeekOf(week), locale),
      })}: ${plural("brief.report.wonCount", won, {
        count: formatNumber(won, locale),
      })}`;
    })
    .join(", ");
  return (
    <div className="brief-weekly-wins" role="img" aria-label={label}>
      {weeks.map((week) => {
        const won = wins.get(week) ?? 0;
        return (
          <div
            key={week}
            className="brief-weekly-wins-bar"
            data-shown={week === shown}
          >
            {/* A week with no win draws no bar, so it never reads as one. */}
            <span
              style={{
                blockSize:
                  won === 0 ? 0 : `${Math.max(4, (won / most) * 100)}%`,
              }}
            />
            <small className="t-num">
              {formatNumber(isoWeekOf(week), locale)}
            </small>
          </div>
        );
      })}
    </div>
  );
}
