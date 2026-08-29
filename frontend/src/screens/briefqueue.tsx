// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import type { BriefItemLabels } from "../design-system/briefitem";
import { BriefItemCard } from "../design-system/briefitem";
import {
  formatDate,
  formatDateTime,
  formatMoneyOrAbsent,
  formatNumber,
} from "../format/format";
import { viewerZone } from "../format/timezone";
import { translatePlural, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf } from "./common";
import type { Deal, MorningBriefItem, useBriefItemMark } from "./home.queries";

// One brief item, drawn and wired — the whole of what a screen needs to show a
// queue entry and let a reader answer it.
//
// It lives here rather than in Home because two surfaces draw this queue now:
// Home reads it as the morning's narrative, and the Worklist works through it.
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
    rank: t("home.brief.rank"),
    composite: t("home.brief.composite"),
    factors: {
      winnability: t("home.factorWinnability"),
      revenue: t("home.factorRevenue"),
      timing: t("home.factorTiming"),
      momentum: t("home.factorMomentum"),
      warmth: t("home.factorWarmth"),
    },
    evidence: translatePlural(locale, "home.evidence", evidenceCount, {
      count: formatNumber(evidenceCount, locale),
    }),
    evidenceNone: t("home.evidenceNone"),
    openDeal: t("home.openDeal"),
    act: t("home.act"),
    dismiss: t("home.dismiss"),
    snooze: t("home.snooze"),
    acted: t("home.actedState"),
    dismissed: t("home.dismissedState"),
    snoozed: t("home.snoozedState"),
    resurfaces: t("home.brief.resurfaces"),
    previouslyDismissed:
      dismissedOn === undefined || recordZone === undefined
        ? ""
        : t("home.brief.previouslyDismissed", {
            day: formatDate(dismissedOn, locale, recordZone),
          }),
    returnedWith: t("home.brief.returnedWith"),
  };
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
          : t("home.brief.revenueBasis", { amount: revenueBasis })
      }
      dealName={deals.find((deal) => deal.id === item.deal_id)?.name}
      amount={amountOf(item.deal_id, deals, locale)}
      formatPercent={(fraction) =>
        t("home.pct", {
          pct: formatNumber(Math.round(fraction * 100), locale),
        })
      }
      formatInstant={(utcIso) => formatDateTime(utcIso, locale, viewerZone())}
      pending={
        mark.isPending && mark.variables?.itemId === item.id
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
