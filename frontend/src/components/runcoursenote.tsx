// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { useT } from "../i18n";
import { runCourseKey } from "../lib/capture-progress";

type BackfillStatus = components["schemas"]["BackfillStatus"];

/** The one rendering of the line that explains why a live import's count
 *  fell, shared by every surface that draws an import. */
export function RunCourseNote({ run }: Readonly<{ run: BackfillStatus }>) {
  const t = useT();
  const key = runCourseKey(run);
  return key === null ? null : (
    <p className="t-caption" role="status">
      {t(key)}
    </p>
  );
}
