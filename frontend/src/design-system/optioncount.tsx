// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { floorFigure } from "../format/figure";
import { formatNumber } from "../format/format";
import { useLocale } from "../i18n";

/**
 * The figure beside an option's name, in a strip that counts what is behind
 * each one.
 *
 * One component because the count is four decisions, not a number: tabular
 * figures so a column of them lines up, the reader's own number format, the
 * CHIP it is drawn as, and the SEPARATOR — which is the one that was missing.
 * Both strips rendered `{label}{count}` as adjacent nodes, so the accessible
 * name a screen reader speaks was "Contacts2", "Deals0", "Tasks0". The comma is
 * visually hidden because the gap between them is already drawn in CSS; what it
 * fixes is the spoken name, where there was nothing between the two at all.
 *
 * The chip is the component's own (`.optioncount`, atoms.css) and takes no
 * class from its caller: every host was styling the same figure itself, which
 * is how one count came to be drawn two ways on two strips of one record. It
 * carries no ink of its own either — it inherits the host's, so the figure goes
 * quiet beside a quiet label and dark beside a pressed one.
 *
 * It renders INSIDE the option's button, which is what puts the figure in that
 * option's accessible name rather than leaving it to a sighted reader alone.
 */
export function OptionCount({
  count,
  floor = false,
}: Readonly<{ count: number; floor?: boolean }>) {
  const { locale } = useLocale();
  return (
    <>
      <span className="sr-only">, </span>
      <span className="optioncount t-num">
        {floorFigure(formatNumber(count, locale), floor, count)}
      </span>
    </>
  );
}
