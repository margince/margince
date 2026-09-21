// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * A panel opened near the bottom of the viewport keeps its own controls
 * reachable.
 *
 * The three portalled panels in the design system — the overflow menu, the
 * popover and the evidence mark's receipt — are `position: fixed` and placed
 * by one hook (`design-system/anchored.ts`). Fixed is what makes this a
 * correctness question rather than a cosmetic one: a panel hanging past the
 * bottom edge cannot be scrolled back into view by the page, by the reader, or
 * by a test, so whatever it was opened for is simply gone.
 *
 * It needs a REAL browser. The rule is arithmetic over laid-out boxes, and the
 * unit environment gives every element a zero-sized rectangle — which is also
 * how the defect this guards shipped: the hook measured the panel's height
 * while its own `max-height: 0` was still on it, decided from a 26px box of
 * padding what belonged in a 143px one, and anchored the flipped panel by a
 * `top` computed from the wrong number. `anchored.test.ts` states the rule over
 * the measurements; this states it over the page.
 */

import { expect, type Locator, type Page, test } from "@playwright/test";
import { mockApi } from "./seed";
import { itemsOf } from "./waits";

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

// Below 1100px the list header folds its verbs into one overflow menu
// (design-system/listsurface.tsx), which is how this suite reaches a panel
// through the product rather than through a fixture of its own.
const WIDTH = 1024;

// How much room is left under the trigger once the viewport has been sized to
// it. Under the hook's own threshold for opening downward, so the panel has to
// flip — and far enough under that a layout reflowing by a few px on resize
// does not carry the case away.
const SNUG = 56;

function box(locator: Locator) {
  return locator.boundingBox().then((found) => {
    if (!found) {
      throw new Error("the element this test is about has no box");
    }
    return found;
  });
}

async function openContactsMenu(page: Page) {
  await page.goto("/#/contacts");
  await page.waitForLoadState("networkidle");
  await expect(page.locator("nav.rail")).toBeVisible();
  const trigger = page.getByRole("button", { name: "Weitere Aktionen" });
  await expect(trigger).toBeVisible();
  return trigger;
}

test("an overflow menu opened near the bottom edge stays inside the viewport", async ({
  page,
}) => {
  await page.setViewportSize({ width: WIDTH, height: 800 });
  let trigger = await openContactsMenu(page);

  // The viewport is sized to the TRIGGER rather than the trigger sought at a
  // viewport somebody once measured: the shell's layout is free to move this
  // button, and a test that hard-coded a height would quietly stop exercising
  // the case it exists for the day it did.
  const first = await box(trigger);
  await page.setViewportSize({
    width: WIDTH,
    height: Math.round(first.y + first.height + SNUG),
  });
  trigger = await openContactsMenu(page);

  const anchor = await box(trigger);
  const view = page.viewportSize();
  if (!view) {
    throw new Error("the page has no viewport");
  }
  // The precondition, asserted rather than assumed. If the layout ever puts
  // this trigger somewhere with room to spare below it, this test is no longer
  // about a panel near the bottom edge — and it has to say so by failing, not
  // by passing on a case it stopped covering.
  expect(
    view.height - (anchor.y + anchor.height),
    "the trigger must sit near the bottom edge for this test to mean anything",
  ).toBeLessThan(96);

  await trigger.click();
  const panel = page.locator(".overflow-menu-items:not([hidden])");
  await expect(panel).toBeVisible();

  const opened = await box(panel);
  expect(opened.y, "the panel's top edge").toBeGreaterThanOrEqual(0);
  expect(
    opened.y + opened.height,
    "the panel's bottom edge, against the viewport's",
  ).toBeLessThanOrEqual(view.height);
  // It flipped rather than merely being squeezed into the gap: with this
  // little room below, a panel still hanging downward is the defect even when
  // a cap happens to keep its box on screen.
  expect(
    opened.y + opened.height,
    "a panel with no room below its trigger opens above it",
  ).toBeLessThanOrEqual(anchor.y + 1);

  // And the controls are actually pressable — the reader's complaint was never
  // about the panel's box, it was that the button inside it could not be
  // reached. A trial click runs every actionability check a real one does,
  // including the in-viewport check that failed 254 times in a row, and
  // presses nothing.
  const items = await itemsOf(panel.getByRole("button"));
  for (const item of items) {
    await item.click({ trial: true, timeout: 5_000 });
  }
});
