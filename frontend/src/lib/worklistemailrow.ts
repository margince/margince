// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type WorklistItem = components["schemas"]["WorklistItem"];

/**
 * Whether the message names the row. A task keeps its own name, the promise,
 * even when it carries the email it was read out of.
 */
export function emailNamesRow(item: WorklistItem): boolean {
  return item.email_summary != null && item.source !== "task";
}
