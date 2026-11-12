// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatAnalyticsMoney, monthName } from "../../format/format";
import { useLocale, useT } from "../../i18n";

type Months = readonly (number | null)[];

/** The twelve monthly goals as bars, drawn only for an uneven year. */
export function GoalBars({ months }: Readonly<{ months: Months }>) {
  const { locale } = useLocale();
  const top = Math.max(...months.map((month) => month ?? 0), 1);
  // Twelve equal bars say nothing the amounts below do not.
  if (months.every((month) => month === months[0])) return null;
  return (
    <div className="goal-bars" aria-hidden="true">
      {months.map((month, index) => (
        <div key={monthName(index + 1, "en")} className="goal-bars-month">
          <div className="goal-bars-plot">
            <span
              className="goal-bars-bar"
              data-unset={month === null || month === 0 || undefined}
              style={{ height: `${((month ?? 0) / top) * 100}%` }}
            />
          </div>
          <span className="goal-bars-name">
            <span className="month-short">
              {monthName(index + 1, locale, "short")}
            </span>
            <span className="month-narrow">
              {monthName(index + 1, locale, "narrow")}
            </span>
          </span>
        </div>
      ))}
    </div>
  );
}

/** The twelve monthly goals as text, six to a row, and the year's sum. */
export function GoalMonths({
  months,
  currency,
}: Readonly<{ months: Months; currency: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const money = (minor: number | null) =>
    minor === null
      ? t("goals.notSet")
      : formatAnalyticsMoney(minor, currency, locale);
  const total = months.every((month) => month !== null)
    ? months.reduce<number>((sum, month) => sum + (month ?? 0), 0)
    : null;
  return (
    <dl className="goal-months" aria-label={t("goals.months")}>
      {months.map((month, index) => (
        <div key={monthName(index + 1, "en")}>
          <dt>{monthName(index + 1, locale)}</dt>
          <dd className="t-num">{money(month)}</dd>
        </div>
      ))}
      <div className="goal-months-total">
        <dt>{t("goals.yearTotal")}</dt>
        <dd className="t-num">{money(total)}</dd>
      </div>
    </dl>
  );
}
