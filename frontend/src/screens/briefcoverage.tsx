// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatNumber } from "../format/format";
import { type Locale, type Translator, useLocale, useT } from "../i18n";
import { sourceName, sourceUnavailableText } from "./worklist.copy";
import type { Worklist } from "./worklist.queries";

// What the page is NOT showing, per source.
//
// `reach` and `sources_unavailable` answer "missing from WHERE", which a rep has
// to have answered before they can trust a short day: a queue that is short
// because one source was withheld is a different day from one that is short
// because there is nothing to do.
//
// ONE META LINE, DIRECTLY UNDER THE FIGURES IT QUALIFIES. It was a Disclosure
// standing ABOVE the readings strip — two presses and a reflow for one
// sentence, met before the figures it was about. Under the strip it reads as
// the strip's own footnote, which is what it is.
//
// It renders NOTHING on the ordinary morning. A line saying "every source
// answered" every day would teach a reader to stop reading it, and then it
// would not be read on the morning it mattered.

/** How much of one bounded source the page is carrying. */
function boundedText(
  row: NonNullable<Worklist["reach"]>[number],
  t: Translator,
  locale: Locale,
): string {
  // WHAT THE PAGE HAS, and that more exists — never a shortfall between two
  // figures. `considered` is itself a floor where the source was bounded, so
  // reporting both read "8 shown of at least 8 read": a sentence that claims
  // something is held back and then accounts for all of it. One clause says the
  // true thing in every case, so there is no branch here to get wrong.
  return t("brief.coverage.bounded", {
    source: sourceName(row.source, t),
    shown: formatNumber(row.shown, locale),
  });
}

export function BriefCoverage({ day }: Readonly<{ day: Worklist }>) {
  const t = useT();
  const { locale } = useLocale();
  const missing = day.sources_unavailable ?? [];
  // A source that stopped at its bound has MORE than it showed. One that read
  // everything it had does not, and listing it would bury the ones that matter.
  const bounded = (day.reach ?? []).filter((row) => row.more_available);
  if (missing.length === 0 && bounded.length === 0) {
    return null;
  }
  // The REFUSALS lead: a source the reader may not see at all is a fact about
  // their standing, and no amount of reading this page will reveal it. A
  // bounded source is a fact about this page, and it follows.
  const said = missing.map((source) => sourceUnavailableText(source, t));
  if (bounded.length > 0) {
    said.push(
      t("brief.coverage.line", {
        sources: bounded.map((row) => boundedText(row, t, locale)).join(" · "),
      }),
    );
  }
  return <p className="brief-coverage t-caption">{said.join(" · ")}</p>;
}
