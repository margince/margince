// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useLocale, usePlural, useT } from "../i18n";
import {
  morningReadings,
  openLane,
  type Reading,
  readingFigure,
  readingWords,
} from "./brief.readings";
import type { Worklist } from "./worklist.queries";

/**
 * The same five readings as ONE LINE under Focus.
 *
 * Home leads with six cards; five tiles under them were a second dashboard
 * that outweighed the cards on visual mass alone. A line keeps every figure
 * and every door, and spends colour only where a figure is breaching — the
 * rule `Reading.warn` already states for the tiles.
 */
export function BriefReadingsLine({ day }: Readonly<{ day: Worklist }>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  return (
    <section
      className="brief-readings brief-readings-line"
      aria-label={t("brief.readings.label")}
      data-testid="brief-readings"
    >
      <ul className="brief-readline">
        {morningReadings(day, t, locale, plural).map(({ key, ...reading }) => (
          <LineReading key={key} {...reading} />
        ))}
      </ul>
    </section>
  );
}

function LineReading({
  label,
  count,
  figure: spelled,
  basis,
  warn,
  floor,
  lane,
  scope,
  openLabel,
  spans,
}: Reading) {
  const t = useT();
  const { locale } = useLocale();
  const marked = floor === true && count !== null && count > 0;
  const openable =
    count === null || count > 0 || floor === true || spans === true;
  const figure = readingFigure(
    spelled ?? readingWords(count, t, locale),
    marked,
  );
  // The basis rides the line only where it changes the reading: a breach, a
  // floor, or a figure that could not be read. "assigned and awaiting first
  // contact" under every "2" is a caption the tile had room for and a line
  // does not.
  const qualified = warn === true || marked || count === null;
  const words = (
    <>
      <span className="brief-readline-label">{label}</span>{" "}
      <strong className={warn ? "brief-readline-warn" : undefined}>
        {figure}
      </strong>
    </>
  );
  return (
    <li>
      {openable ? (
        <button
          type="button"
          className="brief-readline-door"
          title={openLabel}
          onClick={() => openLane(lane, scope)}
        >
          {words}
        </button>
      ) : (
        <span className="brief-readline-door">{words}</span>
      )}
      {qualified && <span className="t-caption"> · {basis}</span>}
    </li>
  );
}
