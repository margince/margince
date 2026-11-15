import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { de } from "../src/i18n/de";
import { customerPayments } from "../src/testing/customerpayments.fixtures";
import { meFixture } from "../src/testing/mefixture";
import { mockApi, servePrincipal } from "./seed";

for (const colorScheme of ["light", "dark"] as const) {
  for (const width of [1280, 390]) {
    test(`customer payments: invoice drawer fits ${width}px in ${colorScheme} and meets accessibility rules`, async ({
      page,
    }) => {
      test.skip(
        Boolean(process.env.BASE_URL),
        "The financial edge fixture needs the isolated mock API.",
      );
      await page.setViewportSize({ width, height: 900 });
      await page.emulateMedia({ colorScheme, reducedMotion: "reduce" });
      await mockApi(page);
      await servePrincipal(
        page,
        meFixture({
          allow: {
            company: ["read"],
            finance: ["read"],
            deal: ["read"],
            contact: ["read"],
            activity: ["read"],
          },
        }),
      );
      await page.route(
        (url) => url.pathname === "/v1/finance/customer-payments",
        (route) => route.fulfill({ json: customerPayments() }),
      );
      await page.goto("/#/home");
      await expect(
        page.getByRole("heading", { name: de["payments.title"] }),
      ).toBeVisible();
      await expect(
        page.getByRole("heading", { name: "Ledger accounts" }),
      ).toHaveCount(0);
      await expect(page.getByRole("link", { name: "Acme" })).toHaveAttribute(
        "href",
        /\/finance$/,
      );
      await page
        .getByRole("button", { name: de["payments.viewInvoices"] })
        .click();
      const drawer = page.getByRole("dialog", {
        name: de["payments.viewInvoices"],
      });
      await expect(drawer.getByText("INV-001")).toBeVisible();
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
      expect(
        await drawer.evaluate(
          (element) => element.scrollWidth <= element.clientWidth,
        ),
      ).toBe(true);
      const result = await new AxeBuilder({ page })
        .include('[role="dialog"]')
        .withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
        .analyze();
      expect(result.violations).toEqual([]);
    });
  }
}
