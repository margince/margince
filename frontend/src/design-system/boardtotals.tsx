// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  formatDate,
  formatDayMonth,
  formatMoneyCompact,
  formatMoneyOrAbsent,
  formatNumber,
  MONEY_ABSENT,
} from "../format/format";
import { formatMoneyOrWord } from "../format/moneyword";
import { translatePlural, useLocale, useT } from "../i18n";
import type { BoardMoneyColumn } from "./pipelineboard";
import { Meter } from "./readings";
import { useDetailTooltip } from "./tooltip";

/**
 * The scale every stage's bar is drawn against: the largest stage total on the
 * board. Only columns in one currency share a scale, so a board whose totals
 * name two currencies draws no bars.
 */
export function largestStageTotal(
  columns: readonly (BoardMoneyColumn | undefined)[],
): number {
  const priced = columns.filter(
    (money): money is BoardMoneyColumn =>
      money?.rawMinor != null && money.currency != null && !money.sumHidden,
  );
  const currencies = new Set(priced.map((money) => money.currency));
  if (currencies.size !== 1) return 0;
  return Math.max(0, ...priced.map((money) => money.rawMinor ?? 0));
}

/**
 * A stage's money, as it heads the column. It shows the total and the weighted
 * total, a bar against the largest stage, and one line of notes.
 *
 * Both figures are abbreviated, as on the cards under them. The exact amounts,
 * the probability and what the notes stand for are in the total's tip.
 */
export function ColumnTotals({
  money,
  scaleMinor,
  zone,
}: Readonly<{
  money: BoardMoneyColumn;
  scaleMinor: number;
  zone?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const compact = (minor: number | null) =>
    formatMoneyOrWord(
      minor,
      money.currency,
      locale,
      MONEY_ABSENT,
      formatMoneyCompact,
    );
  const unpriced = money.unpricedCount ?? 0;
  const reportZone = money.timezone ?? zone;
  const rates =
    money.asOf && reportZone ? { at: money.asOf, zone: reportZone } : null;
  const { ref, trigger, tip } = useDetailTooltip<HTMLSpanElement>(
    [
      t("board.totalsTip", {
        total: formatMoneyOrAbsent(money.rawMinor, money.currency, locale),
        weighted: formatMoneyOrAbsent(
          money.weightedMinor,
          money.currency,
          locale,
        ),
        probability: `${formatNumber(money.probabilityPct, locale)}%`,
      }),
      unpriced > 0 &&
        translatePlural(locale, "board.unpricedTip", unpriced, {
          count: formatNumber(unpriced, locale),
        }),
      rates &&
        t("board.convertedTip", {
          date: formatDate(rates.at, locale, rates.zone),
        }),
    ]
      .filter(Boolean)
      .join(" "),
  );
  const bar = money.rawMinor != null && scaleMinor > 0;
  return (
    <>
      <div className="board-col-total">
        <span className="board-col-money" ref={ref} {...trigger}>
          {compact(money.rawMinor)}
          {tip}
        </span>
        <span className="board-col-weighted">
          <span className="board-col-weighted-figure">
            {compact(money.weightedMinor)}
          </span>{" "}
          {t("board.weightedLabel")}
        </span>
      </div>
      {/* The bar repeats the two figures beside it, so it stays out of the
          accessible tree. A missing weighted figure draws no part, not a zero. */}
      {bar && (
        <div aria-hidden="true">
          {money.weightedMinor == null ? (
            <Meter
              dense
              flat
              value={money.rawMinor ?? 0}
              max={scaleMinor}
              label={t("board.weightedLabel")}
            />
          ) : (
            <Meter
              dense
              value={money.rawMinor ?? 0}
              max={scaleMinor}
              part={money.weightedMinor}
              label={t("board.weightedLabel")}
            />
          )}
        </div>
      )}
      {(unpriced > 0 || rates) && (
        <span className="board-col-notes">
          {unpriced > 0 && (
            <span>
              {translatePlural(locale, "board.unpriced", unpriced, {
                count: formatNumber(unpriced, locale),
              })}
            </span>
          )}
          {rates && (
            <span>
              {t("board.ratesOn", {
                date: formatDayMonth(rates.at, locale, rates.zone),
              })}
            </span>
          )}
        </span>
      )}
    </>
  );
}
