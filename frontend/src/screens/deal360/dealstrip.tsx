// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The deal's four readings, in the band under the header.
//
// A deal page is read two ways. Somebody working THIS deal reads down it;
// somebody working thirty before a forecast call scans for the one that needs
// them. The second read had nothing to land on — the page opened with a stage
// bar, and every fact about the deal was a paragraph or a card further down.
//
// Each card answers one question a rep actually asks, and each is a fact the
// server already sends. Two clauses that would have read well here are absent
// on purpose: "awaiting reply N days" needs a send timestamp the Offer schema
// does not carry, and "they replied twice, we replied once" needs deal-scoped
// direction counts nothing computes. A card that reads well and cannot be
// checked is the thing this page exists to stop.

import type { components } from "../../api/schema";
import "./deal360.css";
import { useRecordZone } from "../../app/recordzone";
import { StatCard } from "../../design-system/atoms";
import { StatStrip } from "../../design-system/statstrip";
import {
  calendarDaysBetween,
  formatDayMonth,
  formatMoneyOrAbsent,
  formatNumber,
  relativeDays,
} from "../../format/format";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

type Deal = components["schemas"]["Deal"];
type Offer = components["schemas"]["Offer"];
type DealCoverage = components["schemas"]["DealCoverage"];

// The forecast words, in the reader's language. An unmapped value renders as
// itself: the category is a wire enum, and a newer server naming a fifth is
// still telling this reader something.
const FORECAST_LABELS: Record<string, MessageKey> = {
  commit: "deal.forecast.commit",
  best_case: "deal.forecast.bestCase",
  pipeline: "deal.forecast.pipeline",
  omitted: "deal.forecast.omitted",
};

function forecastLabel(value: string, t: (key: MessageKey) => string): string {
  return Object.hasOwn(FORECAST_LABELS, value)
    ? t(FORECAST_LABELS[value])
    : value.replaceAll("_", " ");
}

/**
 * The four readings. Fixed at four for the reason the company strip is fixed
 * at five: a row that sometimes drew a fifth would fold at a different width
 * from one click away.
 */
export function DealStrip({
  deal,
  offers,
  coverage,
  coverageWithheld,
}: Readonly<{
  deal: Deal;
  // The deal's offers, newest revision first. Undefined while the read is in
  // flight or when overlay mode serves none.
  offers?: readonly Offer[];
  coverage?: DealCoverage;
  // Withheld is not empty. A caller without the relationship grant is served
  // no seats at all, and a card drawn from that would report "nobody is on
  // this deal" over a check that never ran.
  coverageWithheld: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  return (
    <section className="d360-readings" aria-label={t("deal.strip.title")}>
      <StatStrip testId="deal-strip">
        <MoneyStat deal={deal} offers={offers} locale={locale} t={t} />
        <CloseStat deal={deal} locale={locale} zone={zone} t={t} />
        <PeopleStat
          coverage={coverage}
          withheld={coverageWithheld}
          locale={locale}
          t={t}
        />
        <MomentumStat deal={deal} locale={locale} t={t} />
      </StatStrip>
    </section>
  );
}

// What the deal is worth, and what state the paper is in.
//
// The offer's own state is the qualifier, never a date it was sent: `Offer`
// carries created_at, updated_at and accepted_at, and none of them is the send.
// `updated_at` moves whenever a line item is touched or the renderer stamps the
// pdf, so "sent N days ago" read off it would be wrong on exactly the offers
// somebody is still working.
function MoneyStat({
  deal,
  offers,
  locale,
  t,
}: Readonly<{
  deal: Deal;
  offers?: readonly Offer[];
  locale: Locale;
  t: Translator;
}>) {
  const newest = offers?.[0];
  const detail = newest
    ? t("deal.strip.money.offer", {
        // The offer's identifier, not a quantity: grouped, revision 1234 would
        // read "1.234" and name no document.
        number: String(newest.offer_number),
        status: t(`commercial.offer.${newest.status}` as MessageKey),
      })
    : t("deal.strip.money.noOffer");
  return (
    <StatCard
      label={t("deal.strip.money")}
      value={formatMoneyOrAbsent(deal.amount_minor, deal.currency, locale)}
      detail={detail}
      numeric
    />
  );
}

// When this is expected to close, and how much that date is worth believing.
//
// `close_date_provisional` is the reason this card exists. It means the nightly
// run replaced a date that had aged into the past and no human has confirmed
// the replacement — and until now the page rendered that date identically to
// one somebody agreed with the buyer. A forecast built on machine guesses that
// read as confirmed is the quiet kind of wrong.
function CloseStat({
  deal,
  locale,
  zone,
  t,
}: Readonly<{
  deal: Deal;
  locale: Locale;
  zone: string;
  t: Translator;
}>) {
  if (!deal.expected_close_date) {
    return (
      <StatCard
        label={t("deal.strip.close")}
        value={t("deal.strip.close.none")}
        detail={t("deal.strip.close.noneDetail")}
      />
    );
  }
  const days = calendarDaysBetween(
    new Date(),
    new Date(deal.expected_close_date),
  );
  const parts: string[] = [
    days < 0
      ? t("deal.strip.close.overdue", {
          days: formatNumber(Math.abs(days), locale),
        })
      : t("deal.strip.close.inDays", { days: formatNumber(days, locale) }),
  ];
  if (deal.close_date_provisional) {
    parts.push(t("deal.strip.close.provisional"));
  }
  if (deal.forecast_category) {
    parts.push(forecastLabel(deal.forecast_category, t));
  }
  if (deal.wait_until) {
    parts.push(
      t("deal.strip.close.waiting", {
        date: formatDayMonth(deal.wait_until, locale, zone),
      }),
    );
  }
  return (
    <StatCard
      label={t("deal.strip.close")}
      value={formatDayMonth(deal.expected_close_date, locale, zone)}
      detail={parts.join(" · ")}
      // A date nobody confirmed is a warning about the FIGURE, not about the
      // deal: the tone marks the number, and `alert` (which tints the whole
      // tile) stays for a slot that is itself bad news.
      tone={deal.close_date_provisional || days < 0 ? "warn" : undefined}
    />
  );
}

// How many of the people on this deal are actually talking to us.
function PeopleStat({
  coverage,
  withheld,
  locale,
  t,
}: Readonly<{
  coverage?: DealCoverage;
  withheld: boolean;
  locale: Locale;
  t: Translator;
}>) {
  if (withheld) {
    return (
      <StatCard
        label={t("deal.strip.people")}
        value={t("deal.strip.withheld")}
        detail={t("deal.strip.withheldDetail")}
      />
    );
  }
  const seats = coverage?.stakeholders ?? [];
  const engaged = seats.filter((seat) => seat.engaged).length;
  if (seats.length === 0) {
    return (
      <StatCard
        label={t("deal.strip.people")}
        value={t("deal.strip.people.none")}
        detail={t("deal.strip.people.noneDetail")}
        tone="warn"
      />
    );
  }
  const champion = seats.some((seat) => seat.role === "champion");
  const detail = champion
    ? t("deal.strip.people.champion")
    : t("deal.strip.people.noChampion");
  return (
    <StatCard
      label={t("deal.strip.people")}
      value={t("deal.strip.people.count", {
        engaged: formatNumber(engaged, locale),
        total: formatNumber(seats.length, locale),
      })}
      detail={detail}
      tone={engaged <= 1 || !champion ? "warn" : undefined}
      numeric
    />
  );
}

// Whether anything is happening.
function MomentumStat({
  deal,
  locale,
  t,
}: Readonly<{
  deal: Deal;
  locale: Locale;
  t: Translator;
}>) {
  const parts: string[] = [t("deal.strip.momentum.detail")];
  if (deal.stalled) {
    parts.push(t("deal.stalled"));
  }
  return (
    <StatCard
      label={t("deal.strip.momentum")}
      value={relativeDays(deal.last_activity_at, t, locale)}
      detail={parts.join(" · ")}
      tone={deal.stalled ? "danger" : undefined}
      dot={deal.stalled}
    />
  );
}
