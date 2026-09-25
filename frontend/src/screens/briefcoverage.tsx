// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../design-system/atoms";
import { useT } from "../i18n";
import { omittedFactorsText } from "./brief.factors";
import type { MorningBrief } from "./brief.queries";
import { sourceUnavailableText } from "./worklist.copy";
import type { Worklist } from "./worklist.queries";

// The strip's footnotes: what the page could not read, under the figures they
// qualify.
//
// Two absences share the line because they answer one question, and they part
// over the retry. A FAILED source can be asked for again; a factor a grant
// withheld cannot, and a button beside it would promise a reader something
// pressing it will never give them.
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
  const failed = day.sources_unavailable.filter(
    (entry) => entry.reason === "failed",
  );
  const withheldFactors = omittedFactorsText(run?.factors_omitted ?? [], t);
  if (failed.length === 0 && withheldFactors === null) return null;
  return (
    <div className="brief-coverage" role="status">
      <ul>
        {failed.map((entry) => (
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
