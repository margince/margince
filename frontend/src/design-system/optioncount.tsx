// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { floorFigure } from "../format/figure";
import { formatNumber } from "../format/format";
import { useLocale } from "../i18n";

/**
 * The figure beside an option's name, in a strip that counts what is behind
 * each one.
 *
 * One component because the count is four decisions, not a number. Tabular
 * figures line up a column. The reader's own number format is used. It is
 * drawn as a chip, with a separator between label and figure. Without the
 * separator, adjacent `{label}{count}` nodes speak as "Contacts2". The comma
 * is visually hidden because CSS already draws the gap. It fixes only the
 * spoken name.
 *
 * The chip is the component's own (`.optioncount`, atoms.css) and takes no
 * class from its caller. Hosts that styled the figure themselves drew one
 * count two ways on two strips of one record. It carries no ink of its own
 * either. It inherits the host's, so the figure goes quiet beside a quiet
 * label and dark beside a pressed one.
 *
 * It renders inside the option's button. That puts the figure in the option's
 * accessible name, so a screen reader hears it too.
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
