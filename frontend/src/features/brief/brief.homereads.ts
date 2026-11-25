// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { floorFigure } from "../../format/figure";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCan } from "../../lib/capability";
import { useMagic } from "../../lib/magic.queries";
import { usePendingMarginceChanges } from "../../lib/marginchanges";
import { pillFigure } from "../../lib/worklistcounts";
import { DEFAULT_VIEW } from "../../lib/worklistviews";
import type { Worklist } from "../worklist/worklist.queries";
import { useMorningBrief } from "./brief.queries";

// The reads Home's morning is drawn from, beside the day itself.

// The deal reads never start for a reader without `deal.read`, so their
// sections are not drawn and nothing waits for them.
export function useDealReads() {
  const readsDeals = useCan("deal", "read");
  const brief = useMorningBrief(readsDeals);
  const magic = useMagic(readsDeals);
  const pendingChanges = usePendingMarginceChanges(readsDeals);
  return {
    readsDeals,
    brief,
    magic,
    pendingChanges,
    dealReads: readsDeals ? [brief, magic] : [],
    // No run yet answers 404, which the read turns into null.
    firstDay: readsDeals && brief.isSuccess && brief.data === null,
  };
}

/** What the waiting lanes are counted since: no last brief exists before the first. */
export function waitingHeading(firstDay: boolean): MessageKey {
  return firstDay ? "magic.heading.firstBrief" : "magic.heading.brief";
}

/** The Worklist's own total, and what it counts, for the switch that opens it. */
export function useQueueFigure(day: Worklist | undefined) {
  const plural = usePlural();
  const { locale } = useLocale();
  const figure = day && pillFigure(day, "all", DEFAULT_VIEW);
  if (!figure) return undefined;
  const count = floorFigure(
    formatNumber(figure.count, locale),
    figure.floor,
    figure.count,
  );
  return {
    ...figure,
    countLabel: plural("brief.queue.count", figure.count, { count }),
  };
}
