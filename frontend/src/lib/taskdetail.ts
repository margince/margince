// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { markdownFirstLine } from "../design-system/markdown-text";

/**
 * What a task asks, as one plain line for a row under its title, or nothing.
 * A task the reader may not read says nothing, whatever its body field holds.
 */
export function taskDetail(
  task: Readonly<{
    body?: string | null;
    content_state?: "available" | "withheld" | null;
  }>,
): string | undefined {
  if (task.content_state === "withheld" || !task.body) {
    return undefined;
  }
  return markdownFirstLine(task.body) || undefined;
}
