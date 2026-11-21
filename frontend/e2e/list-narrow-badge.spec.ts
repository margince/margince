// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, test } from "@playwright/test";
import { loadLocale } from "../src/i18n/catalogs";
import { de } from "../src/i18n/de";
import { viewMenuName } from "../src/testing/listview.names";
import { mockApi } from "./seed";

test.beforeAll(async () => {
  await loadLocale("de");
});

// Only a browser lays out a cell. Dragged to its minimum, the identity column
// must show every badge whole or ending in an ellipsis inside the cell.
for (const route of ["companies", "deals"]) {
  test(`${route}: a badge in the narrowest identity column stays inside its cell`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await mockApi(page);
    await page.goto(`/#/${route}`);
    if (route === "deals") {
      await page.getByRole("button", { name: viewMenuName("de") }).click();
      await page
        .getByRole("dialog", { name: viewMenuName("de") })
        .getByRole("button", { name: de["deals.viewTable"], exact: true })
        .click();
    }
    const grip = page.locator(".lt-table thead th.lt-identity .lt-grip");
    await expect(grip).toBeAttached();
    const box = await grip.boundingBox();
    if (!box) {
      throw new Error("the identity header has no grip to drag");
    }
    const y = box.y + box.height / 2;
    await page.mouse.move(box.x + box.width / 2, y);
    await page.mouse.down();
    await page.mouse.move(box.x - 600, y, { steps: 8 });
    await page.mouse.up();

    const header = await page
      .locator(".lt-table thead th.lt-identity")
      .boundingBox();
    // The drag stops at the minimum: narrower is the width that cut badges.
    expect(header?.width).toBeCloseTo(96, 0);

    const verdicts = await page
      .locator(".lt-table tbody td.lt-identity")
      .evaluateAll((cells) =>
        cells.flatMap((cell) =>
          [...cell.querySelectorAll(".badge")].map((badge) => {
            const label = badge.querySelector(".badge-label") ?? badge;
            const range = document.createRange();
            range.selectNodeContents(label);
            const cut = label.scrollWidth > label.clientWidth + 0.5;
            const shown = cut ? label : range;
            return {
              text: badge.textContent,
              badgePast:
                badge.getBoundingClientRect().right -
                cell.getBoundingClientRect().right,
              labelPast:
                shown.getBoundingClientRect().right -
                badge.getBoundingClientRect().right,
              ellipsis:
                !cut || getComputedStyle(label).textOverflow === "ellipsis",
            };
          }),
        ),
      );
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
