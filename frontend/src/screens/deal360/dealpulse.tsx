// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The sentence at the top of a deal: whose move it is, and since when.
//
// The company page opens with one — "It's your move. Akeneo wrote last on
// 5 August — 16 days ago." — and it is the most useful line on that page,
// because it answers the question a reader arrives with before they have read
// anything. The deal page opened with a stage bar, which answers a question
// nobody asks.
//
// WHOSE MOVE IS DECIDED BY THE SERVER, not here. `DealStatusCard.reply_to` is
// the inbound message nobody has answered, picked by the same rule that decides
// whether the email box offers a reply or a fresh mail (`unansweredInbound` in
// compose/dealstatus). A second reading of "is somebody waiting on us" spelled
// in the frontend would disagree with the button beside it the first time
// either changed.

import type { components } from "../../api/schema";
import { useRecordZone } from "../../app/recordzone";
import {
  calendarDaysBetween,
  formatDayMonth,
  formatNumber,
} from "../../format/format";
import { useLocale, useT } from "../../i18n";

type DealStatusCard = components["schemas"]["DealStatusCard"];
type Activity = components["schemas"]["Activity"];

/**
 * DealPulse renders the sentence, or nothing.
 *
 * Nothing is a real answer here. While the card is loading we know neither
 * whose move it is nor when they wrote, and a headline that guessed would be
 * the loudest wrong thing on the page. The stat strip below carries the facts
 * that do not depend on it.
 */
export function DealPulse({
  card,
  timeline,
}: Readonly<{
  card?: DealStatusCard;
  // The rows the page already holds. Used only to name the message behind
  // `reply_to`; never to decide whose move it is.
  timeline: readonly Activity[];
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  if (!card) {
    return null;
  }
  const waiting = waitingSince(card, timeline);
  if (card.reply_to && !waiting) {
    // It IS our move — reply_to says so — but the page cannot name the day.
    // The timeline holds one page and the reader may have filtered it, so the
    // row behind the id can be off-screen. Saying whose move it is without a
    // date is true; inventing the date would not be.
    return (
      <p className="d360-pulse">
        <span className="d360-pulse-lead">{t("deal.pulse.yourMove")}</span>{" "}
        <span className="d360-pulse-rest">{t("deal.pulse.wroteUnknown")}</span>
      </p>
    );
  }
  if (!waiting) {
    // Nobody is owed an answer. That is worth saying — a deal where the ball
    // is with the buyer is a different deal from one nobody has read.
    return (
      <p className="d360-pulse">
        <span className="d360-pulse-lead">{t("deal.pulse.theirMove")}</span>{" "}
        <span className="d360-pulse-rest">{t("deal.pulse.theirMoveWhy")}</span>
      </p>
    );
  }
  const days = calendarDaysBetween(new Date(waiting.at), new Date());
  return (
    <p className="d360-pulse">
      <span className="d360-pulse-lead">{t("deal.pulse.yourMove")}</span>{" "}
      <span className="d360-pulse-rest">
        {t("deal.pulse.wroteOn", {
          date: formatDayMonth(waiting.at, locale, zone),
          days: formatNumber(days, locale),
        })}
      </span>
    </p>
  );
}

// When the unanswered message arrived.
//
// Two sources, in order of how much they can be trusted. `next.evidence`
// carries the timestamp directly and is built from the SAME rule that produced
// `reply_to`, so when the move is a draft it is exact. Otherwise the timeline
// is searched by id — which can miss, because the page holds one page of rows
// and the reader may have filtered them. A miss answers null rather than a
// guess: the sentence then says somebody is waiting without naming a date,
// which is true, where a fabricated date would not be.
function waitingSince(
  card: DealStatusCard,
  timeline: readonly Activity[],
): { at: string } | null {
  if (!card.reply_to) {
    return null;
  }
  const fromMove = card.next?.evidence?.find(
    (row) => row.activity_id === card.reply_to && row.occurred_at,
  );
  if (fromMove?.occurred_at) {
    return { at: fromMove.occurred_at };
  }
  const row = timeline.find((activity) => activity.id === card.reply_to);
  return row ? { at: row.occurred_at } : null;
}
