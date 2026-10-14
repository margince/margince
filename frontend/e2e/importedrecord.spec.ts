import { expect, test } from "@playwright/test";
import { de } from "../src/i18n/de";
import { copy } from "./copy";
import { mockApi } from "./seed";

/**
 * A record the CSV importer created says so on its page. The administrator who
 * ran the import holds captured_by on every such row, so without the import
 * label the page would tell them they typed it. Read on the real page, where
 * the strip, the member lookup and the badge have to agree.
 */

const IMPORTED = copy(de["trust.importedFrom"], {
  uploadName: "pipedrive-export",
});

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

for (const [record, route] of [
  ["company", "/#/companies/o-imported"],
  ["contact", "/#/contacts/p-imported"],
] as const) {
  test(`an imported ${record} names its upload, not the administrator`, async ({
    page,
  }) => {
    await page.goto(route);

    await expect(page.getByText(IMPORTED).first()).toBeVisible();
    await expect(page.getByText(/mirror:/)).toHaveCount(0);
  });
}
