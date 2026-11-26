import { expect, type Locator, test } from "@playwright/test";
import { de } from "../src/i18n/de";
import { worklist } from "./mockapi/work";
import { mockApi } from "./seed";

/**
 * The Worklist drawer's panel heads in a real browser. A head may hold more
 * than one action, and each must show whole. happy-dom has no layout, so only
 * here can two buttons be seen to overlap.
 */

// A decision the reader can take off their list, so Select is offered beside
// the Decisions panel's Company questions launcher.
const DECISION_ROW = {
  actions: ["open"],
  band: "now",
  because: [],
  category: "decisions",
  consequence: "task_slips",
  destination: "review",
  dispositions: ["handled"],
  id: "01a16e36-ef6a-7107-a907-b0a605203d01",
  level: 2,
  occurred_precision: "instant",
  source: "dedupe_candidate",
  title: "infoteamer.de",
};

const VIEWPORTS = [
  { name: "a laptop", size: { width: 1280, height: 800 } },
  { name: "a phone", size: { width: 390, height: 844 } },
] as const;

// Whether two controls share no pixel, so neither hides part of the other.
async function apart(first: Locator, second: Locator) {
  const one = await first.boundingBox();
  const two = await second.boundingBox();
  if (one === null || two === null) return false;
  return (
    one.x + one.width <= two.x ||
    two.x + two.width <= one.x ||
    one.y + one.height <= two.y ||
    two.y + two.height <= one.y
  );
}

for (const { name, size } of VIEWPORTS) {
  test(`Select stands beside Company questions on ${name}`, async ({
    page,
  }) => {
    await page.setViewportSize(size);
    await mockApi(page);
    await page.route("**/v1/worklist*", (route) =>
      route.fulfill({ json: { ...worklist, queue: [DECISION_ROW] } }),
    );
    await page.goto("/#/home?queue=1&queue_view=decisions", {
      waitUntil: "networkidle",
    });
    const drawer = page.getByRole("dialog");
    const launcher = drawer.getByRole("button", {
      name: de["worklist.domainReview.title"],
    });
    const select = drawer.getByRole("button", {
      name: de["worklist.bulk.select"],
      exact: true,
    });
    await expect(launcher).toBeVisible();
    await expect(select).toBeVisible();
    expect(await apart(launcher, select)).toBe(true);
  });
}
