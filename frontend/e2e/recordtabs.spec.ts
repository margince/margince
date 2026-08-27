import { expect, type Page, test } from "@playwright/test";
import { mockApi } from "./seed";

/**
 * The record tab strip is never covered by the rail beside it.
 *
 * The strip's rules span the WORK column, so they read as the seam between a
 * record's identity and its content rather than as a box drawn around the
 * tabs. The rail is a column INSIDE that same container, so a strip that
 * centres itself on the container runs underneath it: on the contact page the
 * rail's card sat over the first three tabs, and a reader met "Network" cut to
 * "twork" with the two tabs before it missing entirely. That reads as a broken
 * menu, not as a strip that continues.
 *
 * A geometry test rather than a unit one, because the defect is layout: the
 * markup, the labels and the order were all correct while the page was wrong.
 * jsdom has no box model and would have passed throughout.
 */

// Where the columns fold. Above it the rail is BESIDE the tabs and can cover
// them; below it the rail stacks underneath and the question does not arise.
const RAILED_WIDTHS = [1280, 1440, 1600];

async function openContact(page: Page) {
  await mockApi(page);
  await page.goto("/#/contacts/p-anna");
  await expect(page.locator(".record-head h1")).toBeVisible();
  await expect(page.locator(".recordtabs-tab").first()).toBeVisible();
}

test.describe("the record tab strip", () => {
  for (const width of RAILED_WIDTHS) {
    test(`is clear of the rail at ${width}px`, async ({ page }) => {
      await page.setViewportSize({ width, height: 900 });
      await openContact(page);

      const rail = await page.locator(".record-rail").boundingBox();
      const tabs = await page.locator(".recordtabs-tab").all();
      expect(tabs.length).toBeGreaterThan(0);
      if (!rail) {
        throw new Error("the rail is present but has no box — cannot place it");
      }

      for (const tab of tabs) {
        const box = await tab.boundingBox();
        const label = (await tab.textContent())?.trim();
        if (!box) {
          throw new Error(`tab "${label}" has no box`);
        }
        // Only a rail on the same row can cover a tab. Once the columns fold
        // the rail sits below, and a left-edge comparison alone would read
        // that stacking as an overlap.
        const sameRow =
          box.y < rail.y + rail.height && box.y + box.height > rail.y;
        if (sameRow) {
          expect(
            box.x,
            `tab "${label}" starts left of the rail's right edge, so the rail covers it`,
          ).toBeGreaterThanOrEqual(rail.x + rail.width);
        }
      }
    });
  }

  // The strip may scroll — it says so — but it must not OPEN scrolled, which
  // is what hid the first tabs while the last ones were in view.
  test("opens at its first tab", async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 900 });
    await openContact(page);
    const scrolled = await page
      .locator(".recordtabs-strip")
      .evaluate((el) => el.scrollLeft);
    expect(scrolled).toBe(0);
  });

  // Every tab is readable in full. A strip whose last tab is half-drawn is the
  // same defect one column over.
  test("draws every tab whole", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await openContact(page);
    const strip = await page.locator(".recordtabs-strip").boundingBox();
    if (!strip) {
      throw new Error("the strip is present but has no box");
    }
    for (const tab of await page.locator(".recordtabs-tab").all()) {
      const box = await tab.boundingBox();
      const label = (await tab.textContent())?.trim();
      if (!box) {
        throw new Error(`tab "${label}" has no box`);
      }
      expect(
        box.x,
        `tab "${label}" is cut off at the strip's left`,
      ).toBeGreaterThanOrEqual(strip.x - 1);
      expect(
        box.x + box.width,
        `tab "${label}" is cut off at the strip's right`,
      ).toBeLessThanOrEqual(strip.x + strip.width + 1);
    }
  });
});
