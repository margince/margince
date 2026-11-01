import { expect, type Locator, type Page, test } from "@playwright/test";
import { de } from "../src/i18n/de";
import { meFixture } from "../src/testing/mefixture";
import { reportingStoryEvaluation } from "../src/testing/reporting.fixtures";
import { mockApi } from "./seed";

/**
 * Sales review's first screen at 1366×768, measured in a browser. Only a
 * real layout shows whether the question, the lead, three readings, the
 * summary and the first action are on screen before any scroll.
 * German, because the app renders German; names come from the catalog.
 */

const VIEWPORT = { width: 1366, height: 768 };

// A quarter with a target it is short of, built from the story evaluation.
const QUARTER = {
  ...reportingStoryEvaluation,
  metrics: [
    ...reportingStoryEvaluation.metrics,
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

test("the question, the lead, three readings, the summary and the first action fit 1366×768", async ({
  page,
}) => {
  await openSalesReview(page);

  const header = page.getByRole("group", { name: de["analytics.filters"] });
  await inFirstScreen(
    "the period control",
    header.getByRole("combobox").first(),
  );
  await inFirstScreen(
    "the active pipeline chip",
    header.getByText(/^Pipeline: /),
  );

  const hero = page.locator("section.panel", {
    has: page.locator(".analytics-hero-body"),
  });
  await inFirstScreen("the lead figure", hero.locator(".analytics-hero-value"));
  await inFirstScreen(
    "the target progress",
    hero.locator(".analytics-hero-aside"),
  );

  const strip = page.getByRole("region", {
    name: de["analytics.overview.readings"],
  });
  const readings = strip.locator("section.stat-card");
  await expect(readings).toHaveCount(3);
  for (let at = 0; at < 3; at++)
    await inFirstScreen(`reading ${at + 1}`, readings.nth(at));

  const summary = page.getByRole("region", {
    name: de["analytics.overview.summary"],
  });
  await inFirstScreen("the summary's title", summary.locator(".panel-title"));
  await inFirstScreen(
    "the summary's first sentence",
    summary.locator(".analytics-decision li").first(),
  );

  const actions = page.getByRole("region", { name: de["analytics.attention"] });
  await inFirstScreen(
    "the first action",
    actions.locator(".analytics-attention-row").first(),
  );

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

test("a quarter with nothing won keeps its explanation and actions, with no blank chart", async ({
  page,
}) => {
  await openSalesReview(page, {
    ...QUARTER,
    charts: QUARTER.charts.map((chart) =>
      chart.kind === "bookings_trend"
        ? {
            ...chart,
            points: chart.points.map((point) => ({
              ...point,
              value: 0,
              comparison: null,
            })),
          }
        : chart,
    ),
  });
  const hero = page.locator("section.panel", {
    has: page.locator(".analytics-hero-body"),
  });
  await expect(hero.locator(".analytics-hero-value")).toBeVisible();
  await expect(hero.locator(".report-chart")).toHaveCount(0);
  await inFirstScreen(
    "the first action",
    page
      .getByRole("region", { name: de["analytics.attention"] })
      .locator(".analytics-attention-row")
      .first(),
  );
});
