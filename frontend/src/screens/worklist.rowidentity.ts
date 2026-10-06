// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { WorklistItem } from "./worklist.queries";

// What identifies one row on this page.
//
// The SOURCE and the id together, because `id` alone is not unique across the
// queue: a task and a waiting message may carry the same underlying record's
// id, and the lanes mint ids independently. The React key has always spelled
// it this way; the selected-row state used the bare id, so two rows sharing one
// could both light up pressed while the pane resolved to whichever came first.
// One function, read by the key, the pane and the Mark done selection, so they
// cannot drift apart again.
export function rowIdentity(item: WorklistItem): string {
  return `${item.source}-${item.id}`;
}
