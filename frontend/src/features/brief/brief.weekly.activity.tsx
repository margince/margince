// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { PanelBody } from "../../design-system/panel";
import { dateTileParts } from "../../format/datetile";
import { formatDayLong, formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCompanyZone } from "../../lib/companyzone";
import type { WeeklyReview } from "./brief.queries";

type DayActivity = NonNullable<WeeklyReview["daily_activity"]>[number];

// Drawn bottom to top, so the rarest kind sits on the baseline.
const KINDS = [
  { field: "meetings_held", label: "brief.weekly.activity.meetings" },
  { field: "calls_logged", label: "brief.weekly.activity.calls" },
  { field: "emails_sent", label: "brief.weekly.activity.emails" },
] as const satisfies readonly {
  field: keyof Omit<DayActivity, "day">;
  label: MessageKey;
}[];

const WORKDAYS = 5;

function dayTotal(day: DayActivity): number {
  return day.emails_sent + day.calls_logged + day.meetings_held;
}

export function weekActivityTotal(days: readonly DayActivity[]): number {
  return days.reduce((sum, day) => sum + dayTotal(day), 0);
}

/** How many things the rep did in the week, for the card's head. */
export function ActivityTotal({
  days,
}: Readonly<{ days: readonly DayActivity[] }>) {
  const plural = usePlural();
  const { locale } = useLocale();
  const total = weekActivityTotal(days);
  return (
    <span className="t-caption t-num">
      {plural("brief.weekly.activity.total", total, {
        count: formatNumber(total, locale),
      })}
    </span>
  );
}

/**
 * The rep's own work per day as stacked columns, with a key that totals each
 * kind. A weekend day shows only when something happened on it.
 */
export function WeekActivity({
  days,
}: Readonly<{ days: readonly DayActivity[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useCompanyZone();
  const shown = days.filter(
    (day, index) => index < WORKDAYS || dayTotal(day) > 0,
  );
  const most = Math.max(1, ...shown.map(dayTotal));
  const busiest = shown.reduce<(typeof shown)[number] | undefined>(
    (top, day) =>
      top === undefined || dayTotal(day) > dayTotal(top) ? day : top,
    undefined,
  );
  return (
    <PanelBody>
      <div className="brief-weekly-activity">
        <ol className="brief-weekly-activity-days">
          {shown.map((day) => {
            const total = dayTotal(day);
            const name = dateTileParts(day.day, locale, zone).weekday;
            return (
              <li
                key={day.day}
                aria-label={`${formatDayLong(day.day, locale, zone)}: ${KINDS.map(
                  (kind) =>
                    `${t(kind.label)} ${formatNumber(day[kind.field], locale)}`,
                ).join(", ")}`}
              >
                <span
                  className="brief-weekly-activity-stack"
                  style={{ blockSize: `${(total / most) * 100}%` }}
                >
                  {KINDS.map(
                    (kind) =>
                      day[kind.field] > 0 && (
                        <i
                          key={kind.field}
                          data-kind={kind.field}
                          style={{ flexGrow: day[kind.field] }}
                        />
                      ),
                  )}
                </span>
                <small aria-hidden="true">{name}</small>
              </li>
            );
          })}
        </ol>
        <ul className="brief-weekly-activity-key">
          {[...KINDS].reverse().map((kind) => (
            <li key={kind.field}>
              <i data-kind={kind.field} aria-hidden="true" />
              {t(kind.label)}
              <b className="t-num">
                {formatNumber(
                  days.reduce((sum, day) => sum + day[kind.field], 0),
                  locale,
                )}
              </b>
            </li>
          ))}
        </ul>
      </div>
      {busiest !== undefined && dayTotal(busiest) > 0 && (
        <p className="t-caption">
          {t("brief.weekly.activity.busiest", {
            day: formatDayLong(busiest.day, locale, zone),
          })}
        </p>
      )}
    </PanelBody>
  );
}
