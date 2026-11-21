// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { isLiveRun } from "../../lib/backfill-run";

type BackfillStatus = components["schemas"]["BackfillStatus"];

/**
 * The line that explains why a live import's count is lower than it was, or
 * null when the count has only gone up. A restarted scan reads from the
 * newest message again, so its counts begin at zero. A retried page keeps the
 * committed count and reads the unfinished page again. The server reports
 * which one happened, so no surface guesses it from a number that fell.
 */
export function runCourseKey(run: BackfillStatus): MessageKey | null {
  if (!isLiveRun(run.state)) {
    return null;
  }
  if ((run.window_restarts ?? 0) > 0) {
    return "backfill.restartedNote";
  }
  return (run.retry_attempts ?? 0) > 0 ? "backfill.retryNote" : null;
}

/** The one rendering of that line, shared by every surface that draws an
 *  import: the Settings card, the Connections row and the onboarding step. */
export function RunCourseNote({ run }: Readonly<{ run: BackfillStatus }>) {
  const t = useT();
  const key = runCourseKey(run);
  return key === null ? null : (
    <p className="t-caption" role="status">
      {t(key)}
    </p>
  );
}
