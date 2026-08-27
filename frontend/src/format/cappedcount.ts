// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale } from "../i18n";
import { formatNumber } from "./format";

/**
 * A count read off ONE page of a keyset-paged list.
 *
 * The list endpoints carry no total, so a screen counting the rows it received
 * knows only that there are AT LEAST that many. Printing that flat reads as a
 * total: a workspace with two hundred open pairs said "50", and the reader who
 * cleared fifty of them found the number unmoved.
 *
 * `more` is the page's own `has_more`. It is not optional and has no default,
 * because a caller that forgot it is exactly the caller this type exists for.
 */
export type CappedCount = Readonly<{ seen: number; more: boolean }>;

/** The count as a surface prints it: the figure, and "+" when the page was
 *  full. The digits go through `formatNumber` so a four-figure count is
 *  grouped the way the reader's locale groups it. */
export function cappedCountLabel(count: CappedCount, locale: Locale): string {
  const figure = formatNumber(count.seen, locale);
  return count.more ? `${figure}+` : figure;
}
