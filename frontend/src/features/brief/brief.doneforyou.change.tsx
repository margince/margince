// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ReceiptDetail } from "../../components/receiptdetail";
import { ReceiptReview } from "../../components/receiptreview";
import { sameCalendarDay } from "../../format/calendarday";
import {
  formatDateTime,
  formatDayMonth,
  formatTimeOfDay,
} from "../../format/format";
import { type Locale, useLocale, useT } from "../../i18n";
import type { Receipt } from "../../lib/handled";
import { receiptHeadline, receiptReason } from "../../lib/receiptcopy";
import { useViewerZone } from "../../lib/viewerzone";

// The automatic change that closes a deal; the one whose reason is expected.
const CLOSE_LOST = "deal_close_lost";

/** One change: what moved and when, why, the evidence, and its verbs. */
export function ChangeRow({
  receipt,
  asOf,
  onSettled,
}: Readonly<{
  receipt: Receipt;
  asOf: string | undefined;
  onSettled: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  // A closure is the deal under its linked name, so it says only what
  // happened to it. A closure with no reason the reader may read says so.
  const closed = receipt.kind === CLOSE_LOST;
  const reason =
    receiptReason(receipt, t, locale) ??
    (closed && receipt.headline ? t("brief.done.noReason") : null);
  return (
    <li className="brief-done-change">
      <p className="brief-done-what">
        {closed && receipt.headline
          ? t("brief.done.markedLost")
          : receiptHeadline(receipt, t, locale, zone)}
      </p>
      <time
        className="t-caption brief-done-when"
        dateTime={receipt.occurred_at}
        title={formatDateTime(receipt.occurred_at, locale, zone)}
      >
        {changeWhen(receipt.occurred_at, asOf, locale, zone)}
      </time>
      {reason && <p className="brief-done-reason">{reason}</p>}
      {receipt.customer_quote && (
        <figure className="brief-done-quote">
          <figcaption className="t-caption">
            {t("brief.done.customerWrote")}
          </figcaption>
          <blockquote>{receipt.customer_quote}</blockquote>
        </figure>
      )}
      <ReceiptDetail receipt={receipt} />
      <div className="brief-done-controls">
        <ReceiptReview receipt={receipt} onDone={onSettled} />
      </div>
    </li>
  );
}

// The hour for a change from today, the day for an older one: the list spans
// as many days as changes have waited.
function changeWhen(
  iso: string,
  asOf: string | undefined,
  locale: Locale,
  zone: string,
): string {
  return asOf !== undefined &&
    sameCalendarDay(new Date(iso), new Date(asOf), zone)
    ? formatTimeOfDay(iso, locale, zone)
    : formatDayMonth(iso, locale, zone);
}
