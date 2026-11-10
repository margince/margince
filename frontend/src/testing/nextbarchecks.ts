// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, waitFor, within } from "storybook/test";

/** One word of 128 characters, as a mail subject with no space in it. */
export const UNBROKEN_SUBJECT = "Angebotsrevision".repeat(8);

const TEXT = ".next-bar-title, .next-bar-sub, .next-bar-none, .next-bar-detail";

function overlaps(a: DOMRect, b: DOMRect): boolean {
  return (
    a.left < b.right - 0.5 &&
    b.left < a.right - 0.5 &&
    a.top < b.bottom - 0.5 &&
    b.top < a.bottom - 0.5
  );
}

/**
 * The bar showing `words` keeps every line of its text inside its body, and
 * its verbs inside the bar and clear of that text.
 */
export async function expectNextBarHoldsItsText(
  canvasElement: HTMLElement,
  words: string,
) {
  const [shown] = await within(canvasElement).findAllByText(
    (_, element) =>
      element?.matches(".next-bar-title, .next-bar-sub") === true &&
      (element.textContent ?? "").includes(words),
  );
  const row = await waitFor(() => {
    const found = shown?.closest(".next-bar-row");
    if (!found) throw new Error("the text is not inside a Next bar");
    return found;
  });
  const bar = row.getBoundingClientRect();
  const body = row.querySelector(".next-bar-body")?.getBoundingClientRect();
  const verbs = row.querySelector(".next-bar-verbs")?.getBoundingClientRect();
  if (!body) throw new Error("the Next bar drew no body");
  for (const line of row.querySelectorAll(TEXT)) {
    const at = line.getBoundingClientRect();
    await expect(at.right).toBeLessThanOrEqual(body.right + 0.5);
    if (verbs) await expect(overlaps(at, verbs)).toBe(false);
  }
  if (verbs) await expect(verbs.right).toBeLessThanOrEqual(bar.right + 0.5);
}
