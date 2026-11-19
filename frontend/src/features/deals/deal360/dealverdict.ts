// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { StandingVerdict } from "../../../design-system/howitstands";
import { activityDay } from "../../../format/activitywhen";
import { formatNumber } from "../../../format/format";
import { formatDayMonthNearYear } from "../../../format/format.dates";
import { type Locale, type Translator, translatePlural } from "../../../i18n";
import { closeWords } from "./dealheaderfacts";
import { closeTone, type DealStandingInputs, waitHolds } from "./dealstanding";
import { daysWaiting, waitingSince } from "./dealwaiting";

type Words = Readonly<{ t: Translator; locale: Locale; zone: string }>;

/**
 * The word an open deal is headed with. A fact on the record that asks
 * something of the reader comes before the card's written call. The call can
 * be a day old, and the fact is current. A named blocker stays, since no fact
 * here says more than the blocker does.
 */
export function openDealVerdict(
  written: StandingVerdict,
  standing: string | undefined,
  inputs: DealStandingInputs,
  words: Words,
): StandingVerdict {
  if (standing === "blocked") {
    return written;
  }
  return recordedVerdict(inputs, words) ?? written;
}

function recordedVerdict(
  { deal, card, asOf }: DealStandingInputs,
  { t, locale, zone }: Words,
): StandingVerdict | undefined {
  const day = (iso: string) =>
    formatDayMonthNearYear(iso, locale, zone, new Date(asOf));
  const waiting = card ? waitingSince(card) : null;
  if (waiting) {
    const days = daysWaiting(waiting, asOf, zone);
    return {
      word: t("deal.verdict.yourMove"),
      sentence:
        days === 0
          ? t("deal.verdict.yourMoveToday")
          : translatePlural(locale, "deal.verdict.yourMoveDays", days, {
              count: formatNumber(days, locale),
            }),
      tone: "warning",
    };
  }
  if (deal.stalled) {
    return {
      word: t("deal.verdict.goneQuiet"),
      sentence: deal.last_activity_at
        ? t("deal.verdict.goneQuietSince", { date: day(deal.last_activity_at) })
        : t("deal.verdict.nothingFiled"),
      tone: "danger",
    };
  }
  const replied = card?.our_reply;
  if (replied) {
    return repliedVerdict(replied, asOf, { t, locale, zone });
  }
  if (waitHolds(deal.wait_until, zone, new Date(asOf))) {
    return {
      word: t("deal.verdict.onHold"),
      sentence: t("deal.verdict.onHoldUntil", { date: day(deal.wait_until) }),
      tone: "neutral",
    };
  }
  if (deal.wait_until) {
    return {
      word: t("deal.verdict.waitOver"),
      sentence: t("deal.verdict.waitOverSince", { date: day(deal.wait_until) }),
      tone: "warning",
    };
  }
  const closeDays = closeWords(deal, locale, zone, t).daysLeft;
  if (deal.expected_close_date && closeTone(deal, closeDays) === "danger") {
    return {
      word: t("deal.verdict.overdueToClose"),
      sentence: t("deal.verdict.closePassed", {
        date: day(deal.expected_close_date),
      }),
      tone: "warning",
    };
  }
  if (!deal.last_activity_at) {
    return {
      word: t("deal.verdict.notStarted"),
      sentence: t("deal.verdict.nothingFiled"),
      tone: "neutral",
    };
  }
  return undefined;
}

type RequestReply = NonNullable<DealStandingInputs["card"]>["our_reply"] &
  object;

// We replied after the buyer's request, and ours is still the newest message.
// A colleague's reply is the team's, so it never reads as the reader's own.
function repliedVerdict(
  replied: RequestReply,
  asOf: string,
  { t, locale, zone }: Words,
): StandingVerdict {
  const date = activityDay(
    replied.replied_at,
    replied.replied_precision,
    locale,
    zone,
    (at, locale, zone) =>
      formatDayMonthNearYear(at, locale, zone, new Date(asOf)),
  );
  const mine = replied.by_reader;
  return {
    word: t(mine ? "deal.verdict.youReplied" : "deal.verdict.teamReplied"),
    sentence: replied.still_asked
      ? t(
          mine
            ? "deal.verdict.youRepliedAsked"
            : "deal.verdict.teamRepliedAsked",
          { date, ask: replied.still_asked },
        )
      : t(mine ? "deal.verdict.youRepliedOn" : "deal.verdict.teamRepliedOn", {
          date,
        }),
    tone: "neutral",
  };
}
