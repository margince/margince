// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../design-system/atoms";
import { useLocale, useT } from "../i18n";
import { omittedFactorsText } from "./brief.factors";
import type { MorningBrief } from "./brief.queries";
import { sourceUnavailableText } from "./worklist.copy";
import type { Worklist } from "./worklist.queries";

// The strip's footnotes: what the page could not read, under the figures they
// qualify.
//
// Three absences share the list because they answer one question, and they
// part over the RETRY. A failed source can be asked for again; a source or a
// ranking factor a grant withheld cannot, and a button beside either would
// promise a reader something pressing it will never give them.
//
// The same split decides what is ANNOUNCED, the distinction `Callout` draws
// between an event and a standing fact. A failed source is an event: it can
// arrive on a refetch and it brings a retry, so it is spoken, and the live
// region holds exactly those rows and their button. A withheld source or
// factor is true at first paint and true tomorrow, so it sits OUTSIDE the
// region: speaking it would read "part of your day is hidden" aloud on every
// mount for as long as the grant stands — the effect the attention feed
// suppresses for the same reason (unseen.go). A warning nobody can act on is
// how a reader learns to ignore the ones they can.
//
// The retry therefore sits WITH the failed rows rather than under the whole
// block, and that is the honest place for it: it repairs one of the three
// absences, and a button trailing all of them reads as a remedy for a grant
// it cannot touch. The wrapper is a `div` so app.css does not put paragraph
// spacing between the two lists the way it would between two `ul`s.
export function BriefCoverage({
  day,
  run,
  onRetry,
}: Readonly<{
  day: Worklist;
  run?: MorningBrief | null;
  onRetry?: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const missing = day.sources_unavailable;
  const failed = missing.filter((entry) => entry.reason === "failed");
  const withheld = missing.filter((entry) => entry.reason !== "failed");
  const withheldFactors = omittedFactorsText(
    run?.factors_omitted ?? [],
    t,
    locale,
  );
  if (missing.length === 0 && withheldFactors === null) return null;
  return (
    <div className="brief-coverage">
      {failed.length > 0 && (
        <div role="status">
          <ul>
            {failed.map((entry) => (
              <li key={entry.source}>{sourceUnavailableText(entry, t)}</li>
            ))}
          </ul>
          {onRetry && (
            <Button variant="ghost" onClick={onRetry}>
              {t("brief.coverage.retry")}
            </Button>
          )}
        </div>
      )}
      {(withheld.length > 0 || withheldFactors !== null) && (
        <ul>
          {withheld.map((entry) => (
            <li key={entry.source}>{sourceUnavailableText(entry, t)}</li>
          ))}
          {withheldFactors !== null && <li>{withheldFactors}</li>}
        </ul>
      )}
    </div>
  );
}
