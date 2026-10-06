import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { meFixture } from "../src/app/mefixture";
import {
  reportingStoryCatalog,
  reportingStoryEvaluation,
  reportingStoryFramework,
  reportingStoryScope,
} from "../src/screens/reporting.fixtures";
import { forecastEvaluation } from "../src/screens/reporting.scenarios";
import { mockApi } from "./seed";

test.use({ locale: "en-GB" });
for (const colorScheme of ["light", "dark"] satisfies ("light" | "dark")[]) {
  for (const width of [1440, 768, 390]) {
    test(`reporting marks and monetary units at ${width}px in ${colorScheme}`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 1000 });
      // Reduced motion before the page loads: `.analytics-body` is still fading in
      // when the evidence drawer has closed, and axe would read the blend.
      await page.emulateMedia({ colorScheme, reducedMotion: "reduce" });
      await mockApi(page);
      await page.route("**/v1/me", (route) =>
        route.fulfill({
          json: meFixture({
            rowScope: "all",
            allow: {
              deal: ["read"],
              forecast: ["read"],
              pipeline: ["read"],
              report_definition: ["read", "create"],
              reporting_framework: ["read"],
              installation_settings: ["read"],
            },
          }),
        }),
      );
      await page.route("**/v1/analytics/context", (route) =>
        route.fulfill({
          json: {
            default_scope: reportingStoryScope,
            allowed_scopes: [reportingStoryScope],
            capabilities: {},
            as_of: reportingStoryEvaluation.context.evaluated_at,
            timezone: "Europe/Berlin",
            base_currency: "EUR",
          },
        }),
      );
      await page.route("**/v1/analytics/metrics", (route) =>
        route.fulfill({ json: reportingStoryCatalog }),
      );
      await page.route("**/v1/analytics/framework", (route) =>
        route.fulfill({ json: reportingStoryFramework }),
      );
      await page.route("**/v1/analytics/evaluate?*", (route) =>
        route.fulfill({ json: reportingStoryEvaluation }),
      );
      await page.route("**/v1/analytics/evidence?*", (route) =>
        route.fulfill({
          json: {
            context: reportingStoryEvaluation.context,
            metric: "bookings_won",
            truncated: false,
            rows: [
              {
                key: "receipt",
                label: "Frozen booking receipt",
                value: 21600000,
                restricted: true,
              },
            ],
          },
        }),
      );
      await page.goto("/#/analytics");
      const graph = page.getByRole("figure", {
        name: "Sales won over time",
      });
      await expect(graph).toBeVisible();
      await expect(page.locator(".stat-card-value").first()).toHaveText(
        "€216k",
      );
      const bounds = await page
        .locator(".report-chart-line")
        .evaluate((element) => {
          const parent = element.parentElement;
          return {
            width: element.getBoundingClientRect().width,
            parentWidth: parent?.getBoundingClientRect().width ?? 0,
          };
        });
      expect(bounds.width).toBeLessThanOrEqual(bounds.parentWidth);
      const widths = await page
        .locator(".report-chart-bullet-fill")
        .evaluateAll((elements) =>
          elements.map((element) => element.getBoundingClientRect().width),
        );
      expect(widths[0]).toBeGreaterThan(20);
      expect(widths[0] / widths[1]).toBeCloseTo(120 / 96, 1);
      const lastPoint = graph.locator(".report-chart-point").last();
      const mark = await lastPoint.boundingBox();
      const line = await page.locator(".report-chart-line").boundingBox();
      expect(mark).not.toBeNull();
      expect(line).not.toBeNull();
      if (!mark || !line) throw new Error("Missing chart geometry");
      expect(mark.x + mark.width / 2).toBeLessThan(line.x + line.width);
      await lastPoint.focus();
      await page.keyboard.press("Enter");
      await expect(
        page.getByText("Restricted record", { exact: true }),
      ).toBeVisible();
      await expect(
        page.getByText("Frozen booking receipt", { exact: true }),
      ).toHaveCount(0);
      await page.keyboard.press("Escape");
      await expect(page.getByRole("dialog")).toBeHidden();
      await expect(lastPoint).toBeFocused();
      const accessibility = await new AxeBuilder({ page })
        .include(".reporting-grid")
        .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
        .analyze();
      expect(accessibility.violations).toEqual([]);
      await page.route("**/v1/analytics/evaluate?*", (route) =>
        route.fulfill({ json: forecastEvaluation }),
      );
      await page.goto("/#/analytics/forecast");
      await expect(page.locator(".segbar-action")).toHaveCount(3);
      const segments = await page
        .locator(".segbar-action")
        .evaluateAll((elements) =>
          elements.map((element) => ({
            width: element.getBoundingClientRect().width,
            colour: getComputedStyle(element).backgroundColor,
          })),
        );
      expect(segments[0].width / segments[1].width).toBeCloseTo(216 / 450, 1);
      expect(new Set(segments.map((segment) => segment.colour)).size).toBe(3);
      for (const segment of segments)
        expect(segment.colour).not.toBe("rgba(0, 0, 0, 0)");
      await expect(page.locator(".waterfall-bars")).toBeVisible();
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
    });
  }
}
