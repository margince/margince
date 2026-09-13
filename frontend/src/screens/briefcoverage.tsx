// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button, Disclosure } from "../design-system/atoms";
import { useT } from "../i18n";
import { sourceName, sourceUnavailableText } from "./worklist.copy";
import type { Worklist } from "./worklist.queries";

export function BriefCoverage({
  day,
  onRetry,
}: Readonly<{ day: Worklist; onRetry?: () => void }>) {
  const t = useT();
  const missing = day.sources_unavailable;
  const bounded = (day.reach ?? []).filter((row) => row.more_available);
  if (missing.length === 0 && bounded.length === 0) return null;
  return (
    <Disclosure
      summary={t("brief.coverage.summary")}
      className="brief-coverage"
    >
      <ul>
        {missing.map((entry) => (
          <li key={entry.source}>{sourceUnavailableText(entry, t)}</li>
        ))}
        {bounded.map((entry) => (
          <li key={entry.source}>
            {t("brief.coverage.more", {
              source: sourceName(entry.source, t),
            })}
          </li>
        ))}
      </ul>
      {onRetry && missing.some((entry) => entry.reason === "failed") && (
        <Button variant="ghost" onClick={onRetry}>
          {t("brief.coverage.retry")}
        </Button>
      )}
    </Disclosure>
  );
}
