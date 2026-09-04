// What the page is NOT showing, per source.
//
// `Worklist.reach` has been on the wire since the queue shipped and was read by
// nothing: the completeness line beside it totals the counts, which answers
// "how much is missing" and not "missing from where". A rep who sees a short
// day needs the second question answered before they can trust the first — a
// queue that is short because one source was withheld is a different day from
// one that is short because there is nothing to do.

import { Disclosure } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { FactList } from "../design-system/factlist";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { sourceName, sourceUnavailableText } from "./worklist.copy";
import type { Worklist } from "./worklist.queries";

/**
 * The coverage disclosure, or nothing.
 *
 * Drawn only when there is something to disclose: a day where every source
 * answered and none was bounded has nothing to say, and a panel that said "all
 * sources complete" every morning would teach a reader to stop reading it.
 */
export function BriefCoverage({ day }: Readonly<{ day: Worklist }>) {
  const t = useT();
  const { locale } = useLocale();
  const missing = day.sources_unavailable ?? [];
  // A source that stopped at its bound has MORE than it showed. One that read
  // everything it had does not, and listing it would bury the two that matter.
  const bounded = (day.reach ?? []).filter((row) => row.more_available);
  if (missing.length === 0 && bounded.length === 0) {
    return null;
  }
  return (
    <Callout tone="info" className="brief-coverage">
      {/* The refusals first and outside the disclosure: a source the reader may
          not see at all is a fact about their day, not a detail to expand. A
          bounded source is the detail — the page did read it, and there is
          simply more behind it. */}
      {missing.map((source) => (
        <p key={source.source} className="t-caption">
          {sourceUnavailableText(source, t)}
        </p>
      ))}
      {/* The summary names what is behind it rather than restating the
          caveat. Home already carries that sentence, on the readings strip's
          own floor slot where it qualifies the figures it is about — and this
          panel renders directly above that strip, so a summary reading "some
          sources have more than this page shows" put the same fact on screen
          twice, two lines apart, in two wordings. The per-source counts below
          are what this disclosure is for and the only place they appear. */}
      {bounded.length > 0 && (
        <Disclosure summary={t("brief.coverage.summary")}>
          <FactList
            facts={bounded.map((row) => ({
              key: row.source,
              term: sourceName(row.source, t),
              // `considered` is a FLOOR where the source was bounded, so the
              // sentence says "at least" rather than printing a figure the read
              // cannot stand behind. The contract says the same thing where the
              // field is declared.
              value: t("brief.coverage.bounded", {
                shown: formatNumber(row.shown, locale),
                considered: formatNumber(row.considered, locale),
              }),
            }))}
          />
        </Disclosure>
      )}
    </Callout>
  );
}
