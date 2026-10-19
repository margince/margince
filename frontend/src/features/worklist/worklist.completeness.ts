// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The figures under the filter pills: how much of a cut the page holds, in the
// contract's units. Cards and items are different units, so only items are
// ever compared with the day's `considered`.

import { floorFigure } from "../../format/figure";
import { formatNumber } from "../../format/format";
import { type Locale, translatePlural } from "../../i18n";
import { type Represented, represented } from "../../lib/worklistunits";
import type { T } from "./worklist.copy";
import { countsUnder } from "./worklist.narrowing";
import type {
  Worklist,
  WorklistFilter,
  WorklistItem,
} from "./worklist.queries";

type WorklistCount = Worklist["counts"][number];

// A pill draws a count only when it is exact. A category read to its bound has
// a floor, and `FilterPills` draws no number for an absent count.
export function pillCount(
  day: Worklist,
  filter: WorklistFilter,
): number | undefined {
  if (filter === "all") {
    return day.counts.some((count) => count.more_available)
      ? undefined
      : day.counts.reduce((total, count) => total + count.considered, 0);
  }
  const found = day.counts.find((count) => count.category === filter);
  if (!found || found.more_available) {
    return undefined;
  }
  return found.considered;
}

/**
 * What the cut on screen is not showing, or null when it shows all of it.
 *
 * `loaded` is every card on screen across the pages read so far. Without it the
 * line reads the first page's own `shown`, which is already in items.
 */
export function completenessText(
  day: Worklist,
  filter: WorklistFilter,
  t: T,
  locale: Locale,
  loaded?: readonly WorklistItem[],
): string | null {
  // Null is a narrowing these figures cannot describe. See countsUnder.
  const counted = countsUnder(day, filter);
  if (counted === null) return null;
  const onScreen: Represented = loaded
    ? represented(loaded)
    : {
        cards: day.queue.length,
        items: counted.reduce((total, count) => total + count.shown, 0),
        floor: false,
      };
  const considered = counted.reduce(
    (total, count) => total + count.considered,
    0,
  );
  const limited = counted.filter((count) => count.more_available);
  const unread = unreadSources(day, filter);
  const floor = limited.length > 0 || onScreen.floor;
  if (onScreen.items >= considered && !floor && unread === 0) {
    return null;
  }
  const shown = formatNumber(onScreen.items, locale);
  // No fraction over a floor: "200 of 200 shown" beside a limit contradicts itself.
  const parts = [
    floor
      ? translatePlural(locale, "worklist.completeness.floor", onScreen.items, {
          shown,
        })
      : t("worklist.completeness", {
          shown,
          considered: formatNumber(considered, locale),
        }),
  ];
  if (onScreen.cards !== onScreen.items) {
    parts.push(
      translatePlural(locale, "worklist.completeness.cards", onScreen.cards, {
        cards: formatNumber(onScreen.cards, locale),
      }),
    );
  }
  if (limited.length > 0) {
    parts.push(
      t("worklist.completeness.bounded", {
        kinds: limited.map((count) => limitedKind(count, t, locale)).join(", "),
      }),
    );
  }
  if (unread > 0) {
    parts.push(
      translatePlural(locale, "worklist.completeness.unread", unread, {
        count: formatNumber(unread, locale),
      }),
    );
  }
  return parts.join(" · ");
}

function limitedKind(count: WorklistCount, t: T, locale: Locale): string {
  const figure = floorFigure(
    formatNumber(count.considered, locale),
    true,
    count.considered,
  );
  return `${t(`worklist.filter.${count.category}` as const)} (${figure})`;
}

// A source that never answered may hold rows of this cut, so the line must not
// fall silent over it. A side read that puts no row on the queue cannot.
function unreadSources(day: Worklist, filter: WorklistFilter): number {
  const sources = new Set(
    day.sources_unavailable
      .filter((entry) => entry.contributes_rows !== false)
      .filter(
        (entry) =>
          filter === "all" ||
          entry.category === undefined ||
          entry.category === filter,
      )
      .map((entry) => entry.source),
  );
  return sources.size;
}
