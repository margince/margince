// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, type Page, test } from "@playwright/test";
import { loadLocale } from "../src/i18n/catalogs";
import { de } from "../src/i18n/de";
import { viewMenuName } from "../src/testing/listview.names";
import { brandt } from "./mockapi/fixtures";
import { page as listPage } from "./mockapi/server";
import { mockApi } from "./seed";

test.beforeAll(async () => {
  await loadLocale("de");
});

async function showDealsAsTable(page: Page) {
  await page.getByRole("button", { name: viewMenuName("de") }).click();
  await page
    .getByRole("dialog", { name: viewMenuName("de") })
    .getByRole("button", { name: de["deals.viewTable"], exact: true })
    .click();
}

// Only a browser lays out a cell. Dragged to its minimum, a column must show
// every badge whole or ending in an ellipsis inside the cell.
const cases = [
  { route: "companies", header: de["company.name"] },
  { route: "deals", header: de["deals.lastSignal"] },
];

for (const { route, header } of cases) {
  test(`${route}: a badge in the narrowest "${header}" column stays inside its cell`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await mockApi(page);
    if (route === "companies") {
      await page.route(/\/v1\/companies(\?.*)?$/, (r) =>
        r.fulfill({
          json: listPage([{ ...brandt, archived_at: "2026-09-13T08:00:00Z" }]),
        }),
      );
    }
    await page.goto(`/#/${route}`);
    if (route === "deals") {
      await showDealsAsTable(page);
    }
    const th = page
      .locator(".lt-table thead th")
      .filter({ hasText: header })
      .first();
    const grip = th.locator(".lt-grip");
    await expect(grip).toBeAttached();
    const box = await grip.boundingBox();
    if (!box) {
      throw new Error("the header has no grip to drag");
    }
    const y = box.y + box.height / 2;
    await page.mouse.move(box.x + box.width / 2, y);
    await page.mouse.down();
    await page.mouse.move(box.x - 600, y, { steps: 8 });
    await page.mouse.up();

    // No narrower than the minimum, the width below which badges were cut.
    expect((await th.boundingBox())?.width).toBeGreaterThanOrEqual(95.5);

    const verdicts = await page.locator(".lt-table").evaluate((table, name) => {
      const heads = [...table.querySelectorAll("thead th")];
      const index = heads.findIndex((h) => h.textContent?.includes(name));
      return [...table.querySelectorAll("tbody tr")].flatMap((row) => {
        const cell = row.children[index];
        return [...(cell?.querySelectorAll(".badge") ?? [])].map((badge) => {
          const label = badge.querySelector(".badge-label") ?? badge;
          const range = document.createRange();
          range.selectNodeContents(label);
          const cut = label.scrollWidth > label.clientWidth + 0.5;
          const shown = cut ? label : range;
          return {
            text: badge.textContent,
            badgePast:
              badge.getBoundingClientRect().right -
              (cell as Element).getBoundingClientRect().right,
            labelPast:
              shown.getBoundingClientRect().right -
              badge.getBoundingClientRect().right,
            ellipsis:
              !cut || getComputedStyle(label).textOverflow === "ellipsis",
          };
        });
      });
    }, header);
    expect(verdicts.length).toBeGreaterThan(0);
    for (const verdict of verdicts) {
      expect(
        verdict.badgePast,
        `${verdict.text} inside the cell`,
      ).toBeLessThanOrEqual(0.5);
      expect(
        verdict.labelPast,
        `${verdict.text} label inside the badge`,
      ).toBeLessThanOrEqual(0.5);
      expect(verdict.ellipsis, `${verdict.text} ends in an ellipsis`).toBe(
        true,
      );
    }
  });
}
