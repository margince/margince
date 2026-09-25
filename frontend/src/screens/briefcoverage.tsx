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
// No live region. Two of the three are STANDING facts, true at first paint and
// true tomorrow, and announcing them would read "part of your day is hidden"
// aloud on every mount for as long as the grant stands. The attention feed
// suppresses the permanent role case for that reason (unseen.go), and a
// warning nobody can act on is how a reader learns to ignore the ones they can.
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
  const withheldFactors = omittedFactorsText(
    run?.factors_omitted ?? [],
    t,
    locale,
  );
  if (missing.length === 0 && withheldFactors === null) return null;
  return (
    <div className="brief-coverage">
      <ul>
        {missing.map((entry) => (
          <li key={entry.source}>{sourceUnavailableText(entry, t)}</li>
        ))}
        {withheldFactors !== null && <li>{withheldFactors}</li>}
      </ul>
      {onRetry && failed.length > 0 && (
        <Button variant="ghost" onClick={onRetry}>
          {t("brief.coverage.retry")}
        </Button>
      )}
    </div>
  );
}
