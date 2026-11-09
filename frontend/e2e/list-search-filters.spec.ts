// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { meFixture } from "../src/testing/mefixture";
import { E2E_ADMIN_GRANTS } from "./mockapi/fixtures";
import { mockApi, servePrincipal } from "./seed";

// The suggestions and View write the same address and request parameters.
test.use({ locale: "en-GB" });
for (const screen of ["contacts", "companies", "deals", "leads"]) {
  for (const width of [390, 1440]) {
    for (const colorScheme of ["light", "dark"] satisfies (
      | "light"
      | "dark"
    )[]) {
      test(`${screen} filter search at ${width}px in ${colorScheme}`, async ({
        page,
      }, testInfo) => {
        await page.setViewportSize({ width, height: 1000 });
        await page.emulateMedia({ colorScheme, reducedMotion: "reduce" });
        await mockApi(page);
        await servePrincipal(page, {
          ...meFixture({ allow: E2E_ADMIN_GRANTS }),
          user: {
            ...meFixture().user,
            locale: "en",
          },
        });
        const requests: URL[] = [];
        page.on("request", (request) => {
          const url = new URL(request.url());
          if (url.pathname === `/v1/${screen}`) requests.push(url);
        });
        await page.goto(`/#/${screen}`);
        const search = page.getByRole("combobox", {
          name: "Search",
          exact: true,
        });
        await search.fill("Lena");
        await expect(
          page.getByRole("option", { name: "Owner is Lena Fischer" }),
        ).toBeVisible();
        const audit = await new AxeBuilder({ page })
          .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
          .analyze();
        expect(audit.violations).toEqual([]);
        const screenshot = testInfo.outputPath("filter-suggestions.png");
        await page.screenshot({ path: screenshot });
        await testInfo.attach("filter-suggestions", {
          path: screenshot,
          contentType: "image/png",
        });
        await search.press("Escape");
        await expect(search).toHaveValue("Lena");
        await search.press("ArrowDown");
        await search.press("Enter");
        await expect(search).toHaveValue("");
        await expect(
          page.getByRole("group", { name: "Owner: Lena Fischer" }),
        ).toBeVisible();
        await expect(page).toHaveURL(/owner_id=u1/);
        await expect
          .poll(() =>
            requests.some(
              (url) =>
                url.searchParams.getAll("owner_id").length === 1 &&
                url.searchParams.get("owner_id") === "u1" &&
                !url.searchParams.get("q"),
            ),
          )
          .toBe(true);
        await page.reload();
        await expect(
          page.getByRole("group", { name: "Owner: Lena Fischer" }),
        ).toBeVisible();
      });
    }
  }
}
