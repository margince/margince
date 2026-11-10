// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { mockApi } from "./seed";

// A list that is still loading draws placeholder rows. At phone width a row is
// a card, and its cells must still show. Otherwise the reader sees blank cards,
// and axe sees rows with no cells.
for (const width of [390, 1440]) {
  test(`the loading rows keep their cells at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 1000 });
    await mockApi(page);
    // The list request never answers, so the placeholder rows stay.
    await page.route("**/v1/contacts?**", () => undefined);
    await page.route("**/v1/contacts", () => undefined);
    await page.goto("/#/contacts");
    const loading = page.locator(".lt-loading");
    await expect(loading.first()).toBeAttached();
    await expect(
      loading.first().locator("td:not(.lt-slack)").first(),
    ).toBeVisible();
    const audit = await new AxeBuilder({ page })
      .include(".lt-table")
      .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
      .analyze();
    expect(audit.violations).toEqual([]);
  });
}
