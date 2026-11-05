// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, type Locator, type Page, test } from "@playwright/test";
import { mockApi } from "./seed";

// The Details editor of the Log activity drawer, measured in a real engine.
// Bold weight and a sticky toolbar are cascade and layout, which the unit
// engine does not compute.

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

async function openDetails(page: Page): Promise<Locator> {
  await page.goto("/#/leads/l-1");
  await page.getByRole("button", { name: "Aktivität erfassen" }).click();
  return page.getByRole("dialog").getByRole("textbox", { name: "Details" });
}

/** Writes `lines` paragraphs the way a paste lands, then reports them. */
async function writeLines(editor: Locator, lines: readonly string[]) {
  await editor.evaluate((node, paragraphs) => {
    node.innerHTML = paragraphs.map((line) => `<p>${line}</p>`).join("");
    node.dispatchEvent(new Event("input", { bubbles: true }));
  }, lines);
}

/** Selects the first word of the first paragraph. */
async function selectFirstWord(editor: Locator) {
  await editor.evaluate((node) => {
    const text = node.querySelector("p")?.firstChild;
    if (!(text instanceof Text)) throw new Error("no first paragraph");
    const range = document.createRange();
    range.setStart(text, 0);
    range.setEnd(text, text.data.indexOf(" "));
    node.focus();
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
  });
}

test("bold text is drawn bold, and a second press takes it off", async ({
  page,
}) => {
  const editor = await openDetails(page);
  await writeLines(editor, ["Budget ist freigegeben."]);
  await selectFirstWord(editor);
  const bold = page.getByRole("dialog").getByRole("button", { name: "Fett" });

  await bold.click();
  const word = editor.locator("b, strong");
  await expect(word).toHaveText("Budget");
  await expect(word).toHaveCSS("font-weight", "700");

  // The engine reads "already bold" from the computed weight, so only a word
  // that is drawn bold can be un-bolded by the same button.
  await bold.click();
  await expect(editor.locator("b, strong")).toHaveCount(0);
});

test("the toolbar stays in view over a long text and stays with its field", async ({
  page,
}) => {
  const editor = await openDetails(page);
  const lines = Array.from({ length: 80 }, (_, at) => `Zeile ${at + 1} hier`);
  await writeLines(editor, lines);
  const scroller = page.getByRole("dialog").locator(".drawer-body");
  const field = editor.locator("xpath=..");
  const foot = field.locator(".richtext-foot");
  const bold = foot.getByRole("button", { name: "Fett" });

  await scroller.evaluate((node) => {
    node.scrollTop = 0;
  });
  await selectFirstWord(editor);
  await expect(editor.locator("p").first()).toBeInViewport();
  await expect(bold).toBeInViewport();
  await bold.click();
  await expect(editor.locator("b")).toHaveText("Zeile");

  await scroller.evaluate((node) => {
    node.scrollTop = node.scrollHeight;
  });
  const placed = await field.evaluate((frame) => {
    const footBox = frame
      .querySelector(".richtext-foot")
      ?.getBoundingClientRect();
    const next = frame.closest(".logactivity-body-field")?.nextElementSibling;
    return {
      footBottom: footBox?.bottom ?? Number.NaN,
      frameBottom:
        frame.getBoundingClientRect().bottom -
        Number.parseFloat(getComputedStyle(frame).borderBottomWidth),
      nextTop: next?.getBoundingClientRect().top ?? Number.NaN,
    };
  });
  // Past the field, the toolbar is back under the last line, above the fields
  // that follow, and covers none of them.
  expect(placed.footBottom).toBeCloseTo(placed.frameBottom, 0);
  expect(placed.footBottom).toBeLessThanOrEqual(placed.nextTop);
});
