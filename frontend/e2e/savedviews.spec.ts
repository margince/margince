// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, type Page, test } from "@playwright/test";
import { de } from "../src/i18n/de";
import { mockApi } from "./seed";

// A saved view renamed in the library, then again on its own page. The seed
// remembers each write and holds the next to If-Match, so a rename that only
// looked done, or a client that sent a superseded version, fails here.

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

async function rename(page: Page, from: string, to: string) {
  await page
    .getByRole("button", {
      name: de["filters.library.rowMore"].replace("{name}", from),
    })
    .click();
  await page
    .getByRole("button", { name: de["views.rename"], exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("textbox", { name: de["views.name"] }).fill(to);
  await dialog
    .getByRole("button", { name: de["views.rename"], exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
}

test("a renamed view opens under its new name, and the next rename holds the version it left", async ({
  page,
}) => {
  const patches: { ifMatch: string | undefined; status: number }[] = [];
  page.on("response", (response) => {
    if (response.request().method() === "PATCH") {
      patches.push({
        ifMatch: response.request().headers()["if-match"],
        status: response.status(),
      });
    }
  });
  await page.goto("/#/filters");
  await rename(page, "Fleet companies", "Flottenkunden");
  await expect(page.getByRole("link", { name: "Flottenkunden" })).toBeVisible();

  await page.goto("/#/filters/companies/v-fleet");
  await expect(
    page.getByRole("heading", { level: 1, name: "Flottenkunden" }),
  ).toBeVisible();
  await rename(page, "Flottenkunden", "Flottenkunden Süd");
  await expect(
    page.getByRole("heading", { level: 1, name: "Flottenkunden Süd" }),
  ).toBeVisible();

  expect(patches).toEqual([
    { ifMatch: "1", status: 200 },
    { ifMatch: "2", status: 200 },
  ]);
});
