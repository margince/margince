import { expect, type Locator, type Page, test } from "@playwright/test";
import { de } from "../src/i18n/de";
import { meFixture } from "../src/testing/mefixture";
import {
  reportingStoryCatalog,
  reportingStoryEvaluation,
} from "../src/testing/reporting.fixtures";
import { mockApi } from "./seed";

// Compact readings precede a substantial outlook on desktop and phone.
const VIEWPORT = { width: 1366, height: 768 };

// A quarter with a target it is short of, built from the story evaluation.
const QUARTER = {
  ...reportingStoryEvaluation,
  metrics: [
    ...reportingStoryEvaluation.metrics.map((metric) =>
      metric.id === "bookings_won" ? { ...metric, value: 42_000_000 } : metric,
    ),
    {
      id: "forecast_landing" as const,
      version: "1",
      unit: "EUR",
      value: 76_000_000,
      coverage: { status: "ok" as const, withheld: false },
      evidence: { metric: "forecast_landing" as const, context_id: "interval" },
      target_comparison: {
        status: "compared" as const,
        target_metric: "bookings_won" as const,
        target: 80_000_000,
        landing: 76_000_000,
        won: 42_000_000,
        gap: 4_000_000,
        remaining: 38_000_000,
        open_pipeline: 136_000_000,
        coverage: 3.58,
      },
    },
  ],
};

const finding = (id: string, type: string, deal: string, label: string) => ({
  id,
  type,
  subject_kind: "deal",
  subject_id: deal,
  subject: { type: "deal", id: deal, label },
  severity: "high",
  affected_minor: 4_800_000,
  currency: "EUR",
  status: "open",
  first_seen_at: "2026-10-12T02:00:00Z",
  last_seen_at: "2026-10-31T02:00:00Z",
  inputs_changed: false,
  destination: { screen: "forecast_review", mode: "answer" },
});

// Three findings on two deals, as listInputChecks answers them.
const CHECKS = {
  data: [
    finding("f-1", "close_past", "d-1", "Harbourline depot retrofit"),
    finding("f-2", "no_next_step", "d-1", "Harbourline depot retrofit"),
    finding("f-3", "close_past", "d-2", "Quayside fleet tools"),
  ],
  groups: [
    {
      type: "close_past",
      findings: 2,
      deals: 2,
      other_findings: 0,
      exposure: [{ currency: "EUR", amount_minor: 9_600_000 }],
      destination: { screen: "forecast_review", mode: "answer" },
    },
    {
      type: "no_next_step",
      findings: 1,
      deals: 1,
      other_findings: 0,
      exposure: [{ currency: "EUR", amount_minor: 4_800_000 }],
      destination: { screen: "forecast_review", mode: "answer" },
    },
  ],
  totals: { findings: 3, deals: 2, other_findings: 0 },
};

async function openSalesReview(page: Page, evaluation: unknown = QUARTER) {
  await page.setViewportSize(VIEWPORT);
  await mockApi(page);
  // The mock's admin holds no forecast grant, so the checks would be withheld.
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
  await page.route("**/v1/analytics/metrics", (route) =>
    route.fulfill({
      json: {
        ...reportingStoryCatalog,
        assessment_views: ["sales", "prospecting", "team"],
      },
    }),
  );
  await page.route("**/v1/analytics/assessment", (route) =>
    route.fulfill({
      json: {
        evaluation_key: QUARTER.evaluation_key,
        generated_at: "2026-11-03T09:00:00Z",
        generated_by: "model",
        sentences: [
          {
            text: "Review confirmed commitments with their owners before changing the forecast.",
            kind: "observation",
            cells: ["metric:bookings_won"],
          },
        ],
        paragraphs: [
          [
            {
              text: "Review confirmed commitments with their owners before changing the forecast.",
              kind: "observation",
              cells: ["metric:bookings_won"],
            },
          ],
          [
            {
              text: "Open pipeline shows the opportunities available, while close dates determine which belong to this quarter. Review those opportunities alongside the target before deciding the next forecast call.",
              kind: "observation",
              cells: ["metric:bookings_won"],
            },
          ],
        ],
        cells: [],
        decisions: 0,
      },
    }),
  );
  await page.route(/\/v1\/analytics\/evaluate/, (route) =>
    route.fulfill({ json: evaluation }),
  );
  await page.route(/\/v1\/forecast\/assurance\/exceptions/, (route) =>
    route.fulfill({ json: CHECKS }),
  );
  await page.goto("/#/analytics/overview?overview=sales", {
    waitUntil: "networkidle",
  });
}

// The element lies wholly inside the viewport: below nothing, cut by no edge.
async function inFirstScreen(what: string, element: Locator) {
  await expect(element, what).toBeVisible();
  const box = await element.boundingBox();
  expect(box, `${what} has a box`).not.toBeNull();
  if (!box) return;
  expect(box.x, `${what} starts inside the left edge`).toBeGreaterThanOrEqual(
    0,
  );
  expect(
    box.x + box.width,
    `${what} ends inside the right edge`,
  ).toBeLessThanOrEqual(VIEWPORT.width);
  expect(box.y, `${what} starts below the top`).toBeGreaterThanOrEqual(0);
  expect(box.y + box.height, `${what} ends above the fold`).toBeLessThanOrEqual(
    VIEWPORT.height,
  );
}

test("four compact readings precede a readable outlook at 1366×768", async ({
  page,
}, testInfo) => {
  await openSalesReview(page);
  const strip = page.getByRole("region", {
    name: de["analytics.overview.readings"],
  });
  const readings = strip.locator("section.stat-card");
  await expect(readings).toHaveCount(4);
  for (let at = 0; at < 4; at++)
    await inFirstScreen(`reading ${at + 1}`, readings.nth(at));
  const outlook = page.getByRole("region", {
    name: de["analytics.outlook.sales"],
  });
  await inFirstScreen("outlook heading", outlook.locator(".panel-title"));
  await expect(outlook.locator(".analytics-outlook-prose p")).toHaveCount(2);
  await expect(
    page.getByRole("region", { name: "Zusammenfassung" }),
  ).toHaveCount(0);
  await expect(page.getByText("Observation", { exact: true })).toHaveCount(0);
  await page.screenshot({
    path: testInfo.outputPath("sales-review-desktop.png"),
    fullPage: true,
  });
  // Nothing pushes the page sideways, neither the document nor the column
  // that scrolls the content.
  const overflow = await page.evaluate(() => {
    const wider = (node: Element) => node.scrollWidth > node.clientWidth;
    const column = document.querySelector("main");
    return {
      document: wider(document.documentElement),
      column: column ? wider(column) : false,
    };
  });
  expect(overflow).toEqual({ document: false, column: false });
});

test("the complete outlook stays readable on a phone with keyboard evidence access", async ({
  page,
}) => {
  await openSalesReview(page);
  await page.setViewportSize({ width: 390, height: 844 });
  const outlook = page.getByRole("region", {
    name: de["analytics.outlook.sales"],
  });
  await expect(outlook.locator(".analytics-outlook-prose p")).toHaveCount(2);
  const records = outlook.getByRole("button", {
    name: de["analytics.outlook.records"],
  });
  await records.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(records).toBeFocused();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});
