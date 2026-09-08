// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import type { BriefItemLabels } from "../design-system/briefitem";
import { BriefItemCard } from "../design-system/briefitem";
import {
  formatDate,
  formatDateTime,
  formatMoney,
  formatMoneyOrAbsent,
  formatNumber,
} from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, translatePlural, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type {
  Deal,
  MorningBrief,
  MorningBriefItem,
  useBriefItemMark,
} from "./brief.queries";
import { problemMessageOf } from "./common";

// One brief item, drawn and wired — the whole of what a screen needs to show a
// queue entry and let a reader answer it.
//
// It lives here rather than in Brief because two surfaces draw this queue now:
// Brief reads it as the morning's narrative, and the Worklist works through it.
// The card itself is presentational by design, so everything AROUND it — the
// thirteen labels, the two locale formatters, the per-item pending and error
// projection, and the three mutations — is the part a second screen would
// otherwise copy. Copied, the two would drift: a snooze policy changed on one
// screen and not the other is two answers to "when does this come back".

/**
 * When a set-aside item returns: tomorrow at eight, in the reader's own zone.
 *
 * The policy is the screen's rather than the card's, and it is spelled once
 * here for the same reason the labels are. A reader who sets something aside on
 * two surfaces must get it back at one time.
 */
export function tomorrowMorning(nowMs: number): string {
  const next = new Date(nowMs);
  next.setDate(next.getDate() + 1);
  next.setHours(8, 0, 0, 0);
  return next.toISOString();
}

/** The card's thirteen strings, in the reader's language. */
export function briefLabels(
  t: (key: MessageKey, params?: Record<string, string>) => string,
  evidenceCount: number,
  locale: Parameters<typeof formatNumber>[1],
  // The day the rep dismissed this deal, when it is one that came back. The
  // CARD cannot compose this: it knows no language and no calendar, and the
  // sentence names a day.
  dismissedOn?: string,
  // The zone that date is read in. The INSTALLATION's, not the reader's: a
  // `format: date` wire value holds no instant to localize, and reading
  // 2026-08-21 in a zone behind UTC prints the day before — so a rep in
  // Vancouver would be told they dismissed it a day earlier than they did.
  recordZone?: string,
): BriefItemLabels {
  return {
    rank: t("brief.rank"),
    composite: t("brief.composite"),
    factors: {
      winnability: t("brief.factorWinnability"),
      revenue: t("brief.factorRevenue"),
      timing: t("brief.factorTiming"),
      momentum: t("brief.factorMomentum"),
      warmth: t("brief.factorWarmth"),
    },
    evidence: translatePlural(locale, "brief.evidence", evidenceCount, {
      count: formatNumber(evidenceCount, locale),
    }),
    evidenceNone: t("brief.evidenceNone"),
    openDeal: t("brief.openDeal"),
    act: t("brief.act"),
    dismiss: t("brief.dismiss"),
    snooze: t("brief.snooze"),
    acted: t("brief.actedState"),
    dismissed: t("brief.dismissedState"),
    snoozed: t("brief.snoozedState"),
    resurfaces: t("brief.resurfaces"),
    previouslyDismissed:
      dismissedOn === undefined || recordZone === undefined
        ? ""
        : t("brief.previouslyDismissed", {
            day: formatDate(dismissedOn, locale, recordZone),
          }),
    returnedWith: t("brief.returnedWith"),
  };
}

/**
 * What the revenue factor measured against, as money.
 *
 * `undefined` unless the run names BOTH the figure and its currency. A bare
 * number is not money — it reads as whatever currency the reader assumes — and
 * the note exists so a proportion can be checked, which an unnamed base cannot
 * do. A run assembled before the currency was carried names none.
 */
export function revenueBasisOf(
  brief: MorningBrief,
  locale: Locale,
): string | undefined {
  if (
    brief.revenue_norm_minor === undefined ||
    brief.revenue_norm_currency === undefined
  ) {
    return undefined;
  }
  return formatMoney(
    brief.revenue_norm_minor,
    brief.revenue_norm_currency,
    locale,
  );
}

/**
 * One queue entry, drawn and answerable.
 *
 * `mark` is the caller's rather than this component's own, so a screen showing
 * several entries runs one mutation across all of them — which is what lets the
 * card that was clicked show the pending verb while its neighbours stay live.
 */
export function BriefQueueItem({
  item,
  deals,
  nowMs,
  mark,
  revenueBasis,
}: Readonly<{
  item: MorningBriefItem;
  deals: readonly Deal[];
  nowMs: number;
  mark: ReturnType<typeof useBriefItemMark>;
  /**
   * The RUN's revenue basis, already formatted as money — every item in one
   * brief measured against the same figure, so it is the caller's to compose
   * once rather than each card's to derive.
   *
   * Absent when the run does not name a currency, which a run assembled before
   * the field existed does not. A bare number is not money: it reads as
   * whatever currency the reader assumes, and the whole point of the note is
   * that a proportion can be checked.
   */
  revenueBasis?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  return (
    <BriefItemCard
      item={item}
      labels={briefLabels(
        t,
        item.evidence_ids.length,
        locale,
        item.lineage?.dismissed_on,
        recordZone,
      )}
      revenueBasisNote={
        revenueBasis === undefined
          ? undefined
          : t("brief.revenueBasis", { amount: revenueBasis })
      }
      dealName={deals.find((deal) => deal.id === item.deal_id)?.name}
      amount={amountOf(item.deal_id, deals, locale)}
      formatPercent={(fraction) =>
        t("brief.pct", {
          pct: formatNumber(Math.round(fraction * 100), locale),
        })
      }
      formatInstant={(utcIso) => formatDateTime(utcIso, locale, viewerZone())}
      pending={
        // The CARD's three verbs only. `unsnooze` shares this mutation but has
        // no control here — a snoozed item is not on the queue to carry one, so
        // the take-back is offered from the toast that follows the snooze and
        // there is no button on this card for it to mark as busy.
        mark.isPending &&
        mark.variables?.itemId === item.id &&
        mark.variables.mark !== "unsnooze"
          ? mark.variables.mark
          : undefined
      }
      error={
        mark.isError && mark.variables?.itemId === item.id
          ? problemMessageOf(mark.error, t)
          : undefined
      }
      onOpenDeal={(dealId) => navigate({ screen: "deals", id: dealId })}
      onAct={(itemId) => mark.mutate({ itemId, mark: "act" })}
      onDismiss={(itemId) => mark.mutate({ itemId, mark: "dismiss" })}
      onSnooze={(itemId) =>
        mark.mutate({
          itemId,
          mark: "snooze",
          snoozedUntil: tomorrowMorning(nowMs),
        })
      }
    />
  );
}

/** The deal's amount, through the one helper that decides how absent money reads. */
function amountOf(
  dealId: string,
  deals: readonly Deal[],
  locale: Parameters<typeof formatMoneyOrAbsent>[2],
): string | null {
  const deal = deals.find((candidate) => candidate.id === dealId);
  if (!deal) {
    return null;
  }
  return formatMoneyOrAbsent(deal.amount_minor, deal.currency, locale);
}
