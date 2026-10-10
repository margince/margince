import { expect, type Locator, type Page, test } from "@playwright/test";
import { mockApi } from "./seed";

/**
 * Two pieces of chrome that share a corner do not hide each other.
 *
 * The notification count is pinned near the bell and the rail's chevron ends
 * the spend line. Each placement must hold for the widest realistic figure.
 * Whether one box covers another is a fact about layout that jsdom cannot see.
 */

type Rect = { left: number; top: number; right: number; bottom: number };

async function rect(target: Locator): Promise<Rect> {
  const box = await target.boundingBox();
  if (!box) throw new Error("the element has no box");
  return {
    left: box.x,
    top: box.y,
    right: box.x + box.width,
    bottom: box.y + box.height,
  };
}

function overlapArea(a: Rect, b: Rect): number {
  const width = Math.min(a.right, b.right) - Math.max(a.left, b.left);
  const height = Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top);
  return width > 0 && height > 0 ? width * height : 0;
}

const area = (r: Rect) => (r.right - r.left) * (r.bottom - r.top);

async function open(page: Page, width: number, height: number) {
  await page.setViewportSize({ width, height });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await mockApi(page);
  // Two digits wide, the widest count the badge shows before it caps.
  await page.route(/\/v1\/notices(\?.*)?$/, async (route) => {
    await route.fulfill({ json: { items: [], unread_count: 40 } });
  });
  await page.goto("/#/deals");
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
}

for (const [width, height] of [
  [1280, 720],
  [390, 844],
]) {
  test.describe(`the chrome at ${width} by ${height}`, () => {
    test("the notification count leaves most of the bell showing", async ({
      page,
    }) => {
      await open(page, width, height);
      const count = page.locator(".notifbell-count");
      await expect(count).toBeVisible();
      const bell = await rect(page.locator(".notifbell svg"));
      const covered = overlapArea(bell, await rect(count)) / area(bell);
      expect(covered).toBeLessThan(0.4);
    });
  });
}

test.describe("the rail footer at 1280 by 720", () => {
  test("the chevron does not print over the balance", async ({ page }) => {
    await open(page, 1280, 720);
    const figure = page.locator(".arspend b");
    await expect(figure).toBeVisible();
    const chevron = page.locator(".archev");
    expect(overlapArea(await rect(figure), await rect(chevron))).toBe(0);
  });
});
