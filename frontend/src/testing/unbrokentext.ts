// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect } from "storybook/test";

/** One word of 128 characters, as a mail subject with no space in it. */
export const UNBROKEN_SUBJECT = "Angebotsrevision".repeat(8);

// A line cut with an ellipsis clips its glyphs inside its own box, so only its
// edges can overflow.
function cutWithEllipsis(text: Element): boolean {
  const style = getComputedStyle(text);
  return style.textOverflow === "ellipsis" && style.overflowX !== "visible";
}

/**
 * `text` keeps its glyphs inside its own box, and its box inside `room` on
 * both inline edges, whichever way the text runs.
 */
export async function expectTextFits(text: Element, room: DOMRect) {
  const at = text.getBoundingClientRect();
  await expect(at.left).toBeGreaterThanOrEqual(room.left - 0.5);
  await expect(at.right).toBeLessThanOrEqual(room.right + 0.5);
  if (!cutWithEllipsis(text)) {
    await expect(text.scrollWidth).toBeLessThanOrEqual(text.clientWidth + 1);
  }
}
