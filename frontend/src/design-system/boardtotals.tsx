// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  formatDate,
  formatDayMonth,
  formatMoneyCompact,
  formatMoneyOrAbsent,
  MONEY_ABSENT,
} from "../format/format";
import { formatMoneyOrWord } from "../format/moneyword";
import { useLocale, useT } from "../i18n";
import { useCompanyZone } from "../lib/companyzone";
import { RATE_SHEET, useCoverageWords } from "../lib/coveragewords";
import { hasGap, type MoneyCoverage } from "../lib/moneycoverage";
import { AppLink } from "./applink";
import type { BoardMoneyColumn } from "./pipelineboard";
import { Meter } from "./readings";
import { useDetailTooltip } from "./tooltip";

// What the total's tip says of the deals it leaves out.
function usePartialTip(coverage: MoneyCoverage | null | undefined) {
  const t = useT();
  const words = useCoverageWords();
  if (!hasGap(coverage)) return null;
  const reasons = Object.values(words(coverage)).filter(Boolean);
  return [t("coverage.partialTip"), ...reasons.map((r) => `${r}.`)].join(" ");
}

// The two reasons, worded apart. A missing rate links to the rate sheet.
function GapNote({ coverage }: Readonly<{ coverage: MoneyCoverage }>) {
  const { rate, value } = useCoverageWords()(coverage);
  return (
    <span>
      {rate && <AppLink href={RATE_SHEET}>{rate}</AppLink>}
      {rate && value && " · "}
      {value}
    </span>
  );
}

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
}: Readonly<{
  money: BoardMoneyColumn;
  scaleMinor: number;
}>) {
  const t = useT();
  // An older response names no zone for its rates; the rates day is the
  // company's, counted on its clock like every report figure.
  const companyZone = useCompanyZone();
  const { locale } = useLocale();
  const compact = (minor: number | null) =>
    formatMoneyOrWord(
      minor,
      money.currency,
      locale,
      MONEY_ABSENT,
      formatMoneyCompact,
    );
  const gap = hasGap(money.coverage);
  const partialTip = usePartialTip(money.coverage);
  const reportZone = money.timezone ?? companyZone;
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
      }),
      partialTip,
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
          {gap && money.rawMinor != null && (
            <span className="board-col-partial"> {t("coverage.partial")}</span>
          )}
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
      {(gap || rates) && (
        <span className="board-col-notes">
          {hasGap(money.coverage) && <GapNote coverage={money.coverage} />}
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
