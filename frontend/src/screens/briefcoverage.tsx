// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../design-system/atoms";
import { useT } from "../i18n";
import { sourceUnavailableText } from "./worklist.copy";
import type { Worklist } from "./worklist.queries";

export function BriefCoverage({
  day,
  onRetry,
}: Readonly<{ day: Worklist; onRetry?: () => void }>) {
  const t = useT();
  const failed = day.sources_unavailable.filter(
    (entry) => entry.reason === "failed",
  );
  if (failed.length === 0) return null;
  return (
    <div className="brief-coverage" role="status">
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
  );
}
