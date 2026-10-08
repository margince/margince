// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The strip's Relationship card, and the reading words it shares with the
// other strip cards.

import type { components } from "../api/schema";
import { StatCard } from "../design-system/atoms";
import { FactList } from "../design-system/factlist";
import { calendarDaysBetween, formatNumber } from "../format/format";
import type { Locale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  HEALTH_DIMENSION_LABEL,
  HEALTH_RATING_LABEL,
  useHealthReason,
} from "./companylookups";

type Health = NonNullable<components["schemas"]["Company360"]["health"]>;
type Translate = ReturnType<typeof useT>;

// A reading the caller's grants withheld, in the word every stat card in the
// product uses. `record.notShown` stays on the contact record's readings row
// and rail: retargeting it would restyle two surfaces nobody looked at here.
export const WITHHELD_READING: MessageKey = "reading.restricted";

// A reading nobody has judged. It is NOT the withheld word — "you may not see
// this" and "there is no verdict yet" are opposite facts about who is missing
// what, and confusing them sends the reader to ask for a grant that would show
// them nothing. Its own key rather than the lifecycle label it matches today.
export const UNASSESSED_READING: MessageKey = "co.strip.notAssessed";

// How long ago an instant fell, against the read's own `as_of` and never the
// reader's clock. `undefined` is TODAY, decided here and nowhere else: zero
// days is today, and a NEGATIVE span is skew between a timestamp and the read
// instant, which no slot may print.
export function daysAgo(at: string, asOf: string): number | undefined {
  const days = calendarDaysBetween(new Date(at), new Date(asOf));
  return days > 0 ? days : undefined;
}

// How long nothing has come back, in the ONE spelling this row has: the
// unanswered slot and the quiet one make the same claim.
function noReply(days: number, locale: Locale, t: Translate): string {
  return t("co.strip.unansweredDetail", { days: formatNumber(days, locale) });
}

// They have never written, which is three different accounts and only one is
// bad news. With nothing sent either, the account has not been worked — a row
// that lit up for every untouched account would say the same of the ones being
// ignored. With something sent we are talking into silence, and a letter
// posted TODAY is not yet unanswered news: the word stands, but there is no
// span to state and nothing to warn about until a day has passed. And with the
// outbound date refused, the row says so rather than guessing which it is.
function silenceReading(
  touchWithheld: boolean,
  lastOutboundAt: string | undefined,
  asOf: string | undefined,
  locale: Locale,
  t: Translate,
): Readonly<{ value: string; detail?: string; tone?: "warning" }> {
  if (touchWithheld) {
    // "No exchange" here would report a refusal as a fact about the account,
    // the one thing this row may never do, and what the health section still
    // supports has no word left in this catalogue.
    return { value: t(WITHHELD_READING) };
  }
  if (!lastOutboundAt || !asOf) {
    return { value: t("co.strip.noInboundEver") };
  }
  const days = daysAgo(lastOutboundAt, asOf);
  return days === undefined
    ? { value: t("co.strip.unanswered") }
    : {
        value: t("co.strip.unanswered"),
        tone: "warning",
        detail: noReply(days, locale, t),
      };
}

// Health as a STATUS with its reason, never a 0-100 verdict (§4.2). The card
// below the fold decomposes it; this says which way it points and why.
//
// A LIVE relationship is reported by the balance of the exchange rather than
// by its recency: one where they write and we do not answer, and one where we
// write into silence, are equally recent and opposite problems. A silent one
// has no balance worth stating — nothing came back, and for how long.
export function HealthStat({
  health,
  touchWithheld,
  lastOutboundAt,
  asOf,
  locale,
  withheld,
  onOpen,
  t,
}: Readonly<{
  health?: Health;
  // Whether the section the outbound date lives in was refused: without it, a
  // missing date is not evidence that nothing was ever sent.
  touchWithheld: boolean;
  // The last word WE sent, which the reading itself does not carry.
  lastOutboundAt?: string;
  // The instant the 360 was read at, which every age on this row measures from.
  asOf?: string;
  locale: Locale;
  withheld: boolean;
  // Handed to every shape this reading takes: a door on only one would make
  // the way out look like a property of the figure.
  onOpen?: () => void;
  t: ReturnType<typeof useT>;
}>) {
  const healthReason = useHealthReason();
  const dimension = health?.relationship;
  const slot = {
    label: t("co.strip.health"),
    narrow: "row",
    basis: dimension ? (
      <FactList
        facts={[
          {
            key: "relationship",
            term: t(HEALTH_DIMENSION_LABEL.relationship),
            value: t(HEALTH_RATING_LABEL[dimension.rating]),
            note: healthReason(dimension),
          },
        ]}
      />
    ) : undefined,
  } as const;
  if (!health) {
    // No health section at all. Withheld says so; anything else has simply not
    // been assessed. Neither is "they have never written", a claim about the
    // account this read has no basis for.
    return (
      <StatCard
        onOpen={onOpen}
        {...slot}
        value={t(withheld ? WITHHELD_READING : UNASSESSED_READING)}
      />
    );
  }
  const days = health.days_since_last_inbound;
  const share = health.reply_balance;
  // The rating reads meetings as well as mail. Where it says the account is in
  // touch, a headline read off the inbox alone would contradict the verdict
  // under it, so the headline takes the rating's reason instead.
  const silentInbox = days == null || days > HEALTH_QUIET_DAYS;
  if (dimension && dimension.rating !== "at_risk" && silentInbox) {
    return (
      <StatCard
        onOpen={onOpen}
        value={t("co.strip.healthActive")}
        detail={healthReason(dimension)}
        {...slot}
      />
    );
  }
  if (days == null && !dimension && !lastOutboundAt && !touchWithheld) {
    // Unrated, and no mail either way: the account may well have meetings and
    // calls, so "no inbound messages" would contradict the brief beside it,
    // which reads this same missing rating as not assessed.
    return (
      <StatCard onOpen={onOpen} {...slot} value={t(UNASSESSED_READING)} />
    );
  }
  if (days == null) {
    const silence = silenceReading(
      touchWithheld,
      lastOutboundAt,
      asOf,
      locale,
      t,
    );
    return (
      <StatCard
        onOpen={onOpen}
        value={silence.value}
        tone={silence.tone}
        detail={silence.detail}
        {...slot}
      />
    );
  }
  if (days > HEALTH_QUIET_DAYS) {
    // A share of the exchange here would describe a conversation that has
    // stopped; what a reader acts on is that nothing has come back.
    return (
      <StatCard
        onOpen={onOpen}
        value={t("co.strip.healthQuiet")}
        tone="warning"
        detail={dimension ? healthReason(dimension) : noReply(days, locale, t)}
        {...slot}
      />
    );
  }
  // A live relationship: say who is carrying it. Below a third of the
  // exchange coming from them is us talking to ourselves, whatever the dates
  // say; above two thirds they are asking more than we are answering.
  if (share == null) {
    return (
      <StatCard onOpen={onOpen} value={t("co.strip.healthActive")} {...slot} />
    );
  }
  const oneSided = share < 0.34 || share > 0.66;
  return (
    <StatCard
      onOpen={onOpen}
      value={
        oneSided ? t("co.strip.healthOneSided") : t("co.strip.healthBalanced")
      }
      tone={oneSided ? "warning" : undefined}
      detail={t("co.strip.replyShare", {
        percent: formatNumber(Math.round(share * 100), locale),
      })}
      {...slot}
    />
  );
}

// The threshold that separates a live conversation from a quiet one. It names
// a number the strip states rather than one the reader must infer from a date,
// and it is deliberately the same span the dormant engagement state uses.
const HEALTH_QUIET_DAYS = 30;
