import { expect, type Page, test } from "@playwright/test";
import { mockApi } from "./seed";

/**
 * Home's rail has no scroller of its own: a window shorter than the page
 * scrolls the page, and the rail goes with it. Its cards keep their own height:
 * a card squeezed to fit the window hides its last rows behind its own edge.
 */

async function openHome(page: Page, height: number) {
  await page.setViewportSize({ width: 1280, height });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await mockApi(page);
  await page.goto("/#/home");
  await expect(page.locator(".brief-rail .brief-overview")).toBeVisible();
}

// A card is cut when its own box is shorter than what it holds, or when it
// reaches past the rail that clips it.
const cutCards = (page: Page) =>
  page.locator(".brief-rail").evaluate((rail) => {
    const railBottom = rail.getBoundingClientRect().bottom;
    return [...rail.children]
      .filter(
        (card) =>
          card.scrollHeight > card.clientHeight + 1 ||
          card.getBoundingClientRect().bottom > railBottom + 1,
      )
      .map((card) => (card.textContent ?? "").trim().slice(0, 40));
  });

test("a window shorter than the rail scrolls the page to the rail's last card and cuts no card", async ({
  page,
}) => {
  await openHome(page, 360);
  expect(await cutCards(page)).toEqual([]);
  const scroller = page.locator(".scroll");
  expect(
    await scroller.evaluate((el) => el.scrollHeight > el.clientHeight),
  ).toBe(true);

  // The wheel, not a script, so a scroller that only code can move fails.
  await page.locator(".brief-rail").hover();
  await page.mouse.wheel(0, 5000);

  const lastCard = page.locator(".brief-rail > *").last();
  await expect
    .poll(async () => {
      const card = await lastCard.boundingBox();
      const view = await scroller.boundingBox();
      return card !== null && view !== null
        ? card.y + card.height <= view.y + view.height
        : false;
    })
    .toBe(true);
});
