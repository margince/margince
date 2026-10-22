// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The figures under the filter pills: how much of a cut the page holds, in the
// contract's units. Cards and items are different units, so only items are
// ever compared with the day's `considered`.

import { floorFigure } from "../../format/figure";
import { formatNumber } from "../../format/format";
import { type Locale, translatePlural } from "../../i18n";
import { unreadSources } from "../../lib/worklistcounts";
import type { T } from "./worklist.copy";
import { countsUnder } from "./worklist.narrowing";
import type {
  Worklist,
  WorklistFilter,
  WorklistItem,
} from "./worklist.queries";
import { type Represented, represented } from "./worklistunits";

type WorklistCount = Worklist["counts"][number];

export { pillCount } from "../../lib/worklistcounts";

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
  if (counted === null) {
    return day.counts.some((count) => count.more_available)
      ? t("worklist.completeness.boundedSome")
      : null;
  }
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

// A bounded category reads zero when the scope dropped every row it read.
// It is named without a figure, because "(0)" beside "may have more" contradicts itself.
function limitedKind(count: WorklistCount, t: T, locale: Locale): string {
  const name = t(`worklist.filter.${count.category}` as const);
  if (count.considered === 0) return name;
  const figure = floorFigure(
    formatNumber(count.considered, locale),
    true,
    count.considered,
  );
  return `${name} (${figure})`;
}
