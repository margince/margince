import { expect, type Locator, type Page, test } from "@playwright/test";
import {
  backlogList,
  quarterSales,
} from "../src/features/analytics/analytics.overview.scenariodata";
import { de } from "../src/i18n/de";
import { reportingFixtureFrames } from "../src/testing/reporting.fixtures";
import { mockApi } from "./seed";

/**
 * Sales review's first screen at 1366×768, measured in a browser. Only a
 * real layout shows whether the question, the lead, three readings, the
 * summary and the first action are on screen before any scroll.
 * German, because the app renders German; names come from the catalog.
 */

const VIEWPORT = { width: 1366, height: 768 };
const clock = reportingFixtureFrames.quarterCutoff.clock;

async function openSalesReview(page: Page) {
  await page.setViewportSize(VIEWPORT);
  await mockApi(page);
  await page.route(/\/v1\/analytics\/evaluate/, (route) =>
    route.fulfill({ json: quarterSales() }),
  );
  await page.route(/\/v1\/forecast\/assurance\/exceptions/, (route) =>
    route.fulfill({ json: backlogList(clock) }),
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
  await page.setViewportSize(VIEWPORT);
  await mockApi(page);
  await page.route(/\/v1\/analytics\/evaluate/, (route) =>
    route.fulfill({
      json: quarterSales({
        charts: quarterSales().charts.map((chart) =>
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
      }),
    }),
  );
  await page.route(/\/v1\/forecast\/assurance\/exceptions/, (route) =>
    route.fulfill({ json: backlogList(clock) }),
  );
  await page.goto("/#/analytics/overview?overview=sales", {
    waitUntil: "networkidle",
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
