import { expect, type Page, test } from "@playwright/test";
import { mockApi } from "./seed";

/**
 * A railed record's tab strip is never covered by the rail beside it.
 *
 * The strip's rules span the WORK column, so they read as the seam between a
 * record's identity and its content rather than as a box drawn around the
 * tabs. The rail is a column INSIDE that same container, so a strip that
 * centres itself on the container runs underneath it — 138px of it, which on
 * the contact page is the first three tabs.
 *
 * A geometry test rather than a unit one, because the defect is layout: the
 * markup, the labels and the order were all correct while the page was wrong.
 * jsdom has no box model and would have passed throughout.
 *
 * BOTH railed records, not just the one the defect was reported on: the
 * invariant is that a rail never covers the strip, and two pages carry a rail.
 */

// Where the columns fold. Above it the rail is BESIDE the tabs and can cover
// them; below it the rail stacks underneath and the question does not arise.
const RAILED_WIDTHS = [1280, 1440, 1600];

// Every record whose layout places a rail to the left of the work column.
const RAILED_RECORDS = [
  { name: "contact", route: "/#/contacts/p-anna" },
  { name: "lead", route: "/#/leads/l-1" },
];

async function openRecord(page: Page, route: string) {
  await mockApi(page);
  await page.goto(route);
  await expect(page.locator(".recordtabs-tab").first()).toBeVisible();
  await expect(page.locator(".record-rail")).toBeVisible();
}

test.describe("the record tab strip", () => {
  for (const record of RAILED_RECORDS) {
    for (const width of RAILED_WIDTHS) {
      test(`is clear of the rail on a ${record.name} at ${width}px`, async ({
        page,
      }) => {
        await page.setViewportSize({ width, height: 900 });
        await openRecord(page, record.route);

        const rail = await page.locator(".record-rail").boundingBox();
        const tabs = await page.locator(".recordtabs-tab").all();
        expect(tabs.length).toBeGreaterThan(0);
        if (!rail) {
          throw new Error("the rail is visible but has no box");
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
  }

  // The strip belongs to its column, on EVERY railed record.
  //
  // Measured against the column rather than against its own children, which is
  // what makes it fail when the breakout returns: a strip and its tabs move
  // TOGETHER under the broken rule, so any assertion comparing the two passes
  // in both states. It is also the only check the lead page can fail — it
  // carries two tabs, so its strip can overhang the rail without a tab
  // landing under one, and the overhang is the defect either way.
  for (const record of RAILED_RECORDS) {
    test(`keeps the width of the column it sits in on a ${record.name}`, async ({
      page,
    }) => {
      await page.setViewportSize({ width: 1440, height: 900 });
      await openRecord(page, record.route);

      const strip = await page.locator(".recordtabs").boundingBox();
      const column = await page.locator(".page-zones-main").boundingBox();
      if (!strip || !column) {
        throw new Error(
          "the strip and its column are visible but one has no box",
        );
      }
      // One pixel of tolerance: the boxes are laid out in fractional CSS pixels.
      expect(
        strip.x,
        "the strip starts left of its column, which is where it ran under the rail",
      ).toBeGreaterThanOrEqual(column.x - 1);
      expect(
        strip.x + strip.width,
        "the strip runs past its column's right edge",
      ).toBeLessThanOrEqual(column.x + column.width + 1);
    });
  }
});
