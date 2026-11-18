// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { calendarDay } from "../format/calendarday";
import { formatDayMonthNearYear } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { BoardDeal } from "./pipelineboard";

/**
 * When the deal closes, on the card's figure line.
 *
 * Read as a calendar day in the record's zone, never as the instant midnight
 * UTC: that instant is the previous date for half the world. The tone marks a
 * date nobody confirmed or one already behind us. Words say the same for a
 * reader who does not get the colour, as the deal strip does.
 */
function DealCloses({
  day,
  provisional,
  zone,
}: Readonly<{ day: string; provisional: boolean; zone: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const overdue = day < calendarDay(new Date(), zone);
  const classes =
    provisional || overdue ? "deal-closes deal-closes-warning" : "deal-closes";
  return (
    <span className={classes}>
      {t("deal.closes", {
        date: formatDayMonthNearYear(day, locale, zone, new Date()),
      })}
      {provisional && (
        <span className="sr-only"> · {t("deal.closesProvisional")}</span>
      )}
    </span>
  );
}

// The card's close line: the caller's own words, the day a decided deal
// closed, or the day an open one is expected to.
export function DealCardClose({
  deal,
  closed,
  zone,
}: Readonly<{ deal: BoardDeal; closed?: string | null; zone: string }>) {
  const t = useT();
  const { locale } = useLocale();
  if (closed !== undefined) {
    return <span className="deal-closes">{closed ?? t("deal.undated")}</span>;
  }
  // When a won or lost deal closed: a fact, so it carries no warning tone.
  if (deal.closedAt !== undefined) {
    return (
      <span className="deal-closes">
        {deal.closedAt
          ? t("deal.closed", {
              date: formatDayMonthNearYear(
                deal.closedAt,
                locale,
                zone,
                new Date(),
              ),
            })
          : t("deal.undated")}
      </span>
    );
  }
  if (!deal.closeDate) {
    return <span className="deal-closes">{t("deal.undated")}</span>;
  }
  return (
    <DealCloses
      day={deal.closeDate}
      provisional={deal.closeDateProvisional ?? false}
      zone={zone}
    />
  );
}
