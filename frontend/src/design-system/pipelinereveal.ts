// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useEffect, useRef, useState } from "react";
import { formatNumber, INTL_LOCALE } from "../format/format";
import { type Locale, translatePlural } from "../i18n";
import type { BoardColumn, BoardRecord } from "./pipelineboard";

// How much of a column must show inside the board to count as on screen. Half,
// so the cut stage at the board's edge counts only when its cards can be read.
const SEEN_RATIO = 0.5;

const scrollIn = { inline: "nearest", block: "nearest" } as const;

/** The columns holding a card, in board order. */
export function matchingColumns<Record extends BoardRecord>(
  columns: readonly BoardColumn<Record>[],
): BoardColumn<Record>[] {
  return columns.filter((column) => column.deals.length > 0);
}

/** "3 results in Closed Won": how many matches, and the columns they are in. */
export function matchesTitle<Record extends BoardRecord>(
  columns: readonly BoardColumn<Record>[],
  locale: Locale,
): string {
  const count = columns.reduce(
    (sum, column) => sum + (column.count ?? column.deals.length),
    0,
  );
  const names = new Intl.ListFormat(INTL_LOCALE[locale], {
    style: "long",
    type: "conjunction",
  }).format(columns.map((column) => column.label));
  return translatePlural(locale, "board.matchesIn", count, {
    count: formatNumber(count, locale),
    columns: names,
  });
}

/** Brings one column into view, leaving it where it is if it already shows. */
export function scrollColumnIn(node: HTMLElement | undefined) {
  node?.scrollIntoView?.(scrollIn);
}

/**
 * Once per search, checks whether any column holding a match shows inside the
 * board. When none does, it scrolls the first into view and answers true until
 * the search changes, so the board can say where the matches are.
 *
 * Once per search, because a later page or a refresh must not pull the board
 * away from a column the reader has since scrolled to.
 */
export function useOffScreenMatches(
  board: RefObject<HTMLElement | null>,
  sections: RefObject<Map<string, HTMLElement>>,
  revealKey: string | undefined,
  matchStages: readonly string[],
): boolean {
  const [offScreenFor, setOffScreenFor] = useState<string>();
  const checkedFor = useRef<string | undefined>(undefined);
  // A string, so a new array with the same stages is no new search.
  const stagesKey = matchStages.join("\n");
  useEffect(() => {
    if (!revealKey) {
      checkedFor.current = undefined;
      return;
    }
    if (!stagesKey || checkedFor.current === revealKey) {
      return;
    }
    const targets = stagesKey
      .split("\n")
      .map((stage) => sections.current.get(stage))
      .filter((node): node is HTMLElement => node !== undefined);
    // jsdom has no IntersectionObserver: the reveal still runs, unmeasured.
    if (typeof IntersectionObserver === "undefined") {
      checkedFor.current = revealKey;
      scrollColumnIn(targets[0]);
      return;
    }
    const observer = new IntersectionObserver(
      (entries) => {
        observer.disconnect();
        checkedFor.current = revealKey;
        const seen = entries.some(
          (entry) => entry.intersectionRatio >= SEEN_RATIO,
        );
        // Cleared as well as set: the same words searched again may land on screen.
        setOffScreenFor(seen ? undefined : revealKey);
        if (!seen) {
          scrollColumnIn(targets[0]);
        }
      },
      { root: board.current, threshold: [0, SEEN_RATIO] },
    );
    for (const target of targets) {
      observer.observe(target);
    }
    return () => observer.disconnect();
  }, [revealKey, stagesKey, board, sections]);
  return (
    revealKey !== undefined && revealKey !== "" && offScreenFor === revealKey
  );
}
