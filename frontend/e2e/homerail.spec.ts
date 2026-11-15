import { expect, type Page, test } from "@playwright/test";
import { mockApi } from "./seed";

/**
 * Home's rail is a pane that stays in view and scrolls itself once it is taller
 * than the window. Its cards keep their own height inside it: a card squeezed
 * to fit the window hides its last rows behind its own rounded edge.
 */

async function openHome(page: Page, height: number) {
  await page.setViewportSize({ width: 1280, height });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await mockApi(page);
  await page.goto("/#/home");
  await expect(page.locator(".brief-rail > *").first()).toBeVisible();
}

const cutCards = (page: Page) =>
  page
    .locator(".brief-rail")
    .evaluate((rail) =>
      [...rail.children]
        .filter((card) => card.scrollHeight > card.clientHeight + 1)
        .map((card) => (card.textContent ?? "").trim().slice(0, 40)),
    );

test("a window shorter than the rail scrolls the rail and cuts no card", async ({
  page,
}) => {
  await openHome(page, 360);
  expect(await cutCards(page)).toEqual([]);
  const rail = page.locator(".brief-rail");
  expect(await rail.evaluate((el) => el.scrollHeight > el.clientHeight)).toBe(
    true,
  );
});
