// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type WorklistItem = components["schemas"]["WorklistItem"];

/** Whether the reader attends this meeting and has not confirmed they are ready. */
export function isUnprepared(item: WorklistItem): boolean {
  return (
    item.source === "meeting" && item.preparation?.state === "needs_review"
  );
}
