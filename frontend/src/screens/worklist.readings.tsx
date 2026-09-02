// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The day's four outcome readings, in the band above the queue.
//
// The filter pills below already say how many items of each kind there are.
// These say what those items MEAN: a rep scanning their morning asks "what is
// at stake" before "what is there", and the pills cannot answer the first —
// "eleven deals at risk" and "€380k drifting" are different news, and the
// second is what decides whether the pill gets opened.
//
// Every figure is the server's. The strip formats and never computes: revenue
// at risk is a sum over deal amounts, and a browser doing that arithmetic
// becomes a second author of what a deal is worth, in a tree that already
// keeps a gate over exactly that split.

import { StatCard } from "../design-system/atoms";
import { StatStrip } from "../design-system/statstrip";
import { formatMoneyCompact, formatNumber } from "../format/format";
import { type Locale, type Translator, useLocale, useT } from "../i18n";
import type { Worklist } from "./worklist.queries";
import "./worklist.css";

/**
 * The four readings. Fixed at four for the reason the deal strip is fixed at
 * four: a row that sometimes drew a fifth would fold at a different width from
 * one click away.
 */
export function WorklistReadings({ day }: Readonly<{ day: Worklist }>) {
  const t = useT();
  const { locale } = useLocale();
  const readings = day.readings;
  // A source read to its bound makes every figure a floor, so the caveat sits
  // under the whole strip rather than in one slot. The four are read across as
  // one statement, and a caveat on one of them invites the reading where the
  // other three are exact.
  return (
    <section
      className="worklist-readings"
      aria-label={t("worklist.readings.label")}
    >
      <StatStrip testId="worklist-readings">
        <RevenueStat readings={readings} locale={locale} t={t} />
        <CountStat
          label={t("worklist.readings.replies")}
          detail={t("worklist.readings.replies.detail")}
          count={readings.buyer_replies}
          locale={locale}
        />
        <CountStat
          label={t("worklist.readings.prospecting")}
          detail={t("worklist.readings.prospecting.detail")}
          count={readings.prospecting}
          locale={locale}
        />
        <CountStat
          label={t("worklist.readings.review")}
          detail={t("worklist.readings.review.detail")}
          count={readings.review}
          locale={locale}
        />
      </StatStrip>
      {readings.more_available && (
        <p className="t-meta worklist-readings-floor">
          {t("worklist.readings.truncated")}
        </p>
      )}
    </section>
  );
}

// What the drifting deals are worth.
//
// Two absences reach here and they are NOT the same, so they do not render
// alike. A null amount means nothing at risk could be priced — nobody can tell
// what is drifting, which is news. A null currency means the amounts never went
// through the conversion seam, so the sum is raw minor units in no one
// currency: a figure whose units nobody knows is not money, and drawing it as
// money is the error the seam exists to prevent. Both fall to the same honest
// caption rather than to a number the reader would trust.
function RevenueStat({
  readings,
  locale,
  t,
}: Readonly<{
  readings: Worklist["readings"];
  locale: Locale;
  t: Translator;
}>) {
  const minor = readings.revenue_at_risk_minor;
  const currency = readings.revenue_currency;
  if (minor == null || !currency) {
    return (
      <StatCard
        label={t("worklist.readings.revenue")}
        value="—"
        detail={t("worklist.readings.revenue.unpriced")}
      />
    );
  }
  return (
    <StatCard
      label={t("worklist.readings.revenue")}
      value={formatMoneyCompact(minor, currency, locale)}
      detail={t("worklist.readings.revenue.detail")}
      // The reading is bad news whenever there is any of it: money drifting is
      // the thing this strip exists to surface, and a rep who sees it in the
      // page's ordinary tone reads it as a status rather than as work.
      tone={minor > 0 ? "warn" : undefined}
      numeric
    />
  );
}

// One tally, in the reader's own number formatting.
function CountStat({
  label,
  detail,
  count,
  locale,
}: Readonly<{
  label: string;
  detail: string;
  count: number;
  locale: Locale;
}>) {
  return (
    <StatCard
      label={label}
      value={formatNumber(count, locale)}
      detail={detail}
      numeric
    />
  );
}
