// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
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
          <LineReading key={key} {...reading} label={t(CHIP_LABEL[key])} />
        ))}
      </ul>
    </section>
  );
}

/** The chip's word for each reading: short, because five chips share a line. */
const CHIP_LABEL: Record<string, MessageKey> = {
  urgent: "brief.readings.chip.urgent",
  meetings: "brief.readings.chip.meetings",
  leads: "brief.readings.chip.leads",
  risk: "brief.readings.chip.risk",
  decisions: "brief.readings.chip.decisions",
};

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
  // A TOPIC AT ZERO SAYS NOTHING and takes a chip to say it. The spanning
  // reading — urgent — stands at zero, because "nothing urgent" is the one
  // zero a reader opens the page for; the risk reading, which spells its
  // figure in words, drops out with the others when no deal is flagged.
  if (count === 0 && (spans !== true || spelled !== undefined)) {
    return null;
  }
  const figure = readingFigure(
    spelled ?? readingWords(count, t, locale),
    marked,
  );
  const qualified = warn === true || marked || count === null;
  const words = (
    <>
      <strong className={warn ? "brief-readline-warn" : undefined}>
        {figure}
      </strong>{" "}
      <span className="brief-readline-label">{label}</span>
      {qualified && <span className="brief-readline-basis"> · {basis}</span>}
    </>
  );
  return (
    <li className="brief-readline-chip">
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
    </li>
  );
}
