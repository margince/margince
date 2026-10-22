import { expect, type Page, test } from "@playwright/test";
import { brandt } from "./mockapi/fixtures";
import { mockApi } from "./seed";
import { pageOverflow } from "./waits";

/**
 * A name nobody broke into words stays inside the page that shows it.
 *
 * A company name copied from a URL is one unbroken run. A flex or grid cell
 * sizes to its content unless told otherwise, so the run pushed its neighbours
 * off the window. Only a browser lays out the box; jsdom does not.
 */

const LONG = `https://example.test/${"a".repeat(200)}`;

async function openDealWithLongCompany(page: Page, width: number) {
  await page.setViewportSize({ width, height: 800 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await mockApi(page);
  await page.route(/\/v1\/companies\/o-brandt$/, async (route) => {
    await route.fulfill({ json: { ...brandt, display_name: LONG } });
  });
  await page.goto("/#/deals/d-fleet");
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  await expect(page.getByText(LONG).first()).toBeVisible();
}

for (const width of [1280, 390]) {
  test.describe(`a deal page at ${width}px`, () => {
    test("does not pan for a company name with no break in it", async ({
      page,
    }) => {
      await openDealWithLongCompany(page, width);
      expect(await pageOverflow(page)).toEqual([]);
    });

    test("keeps the company name inside its card", async ({ page }) => {
      await openDealWithLongCompany(page, width);
      const verdicts = await page.getByText(LONG).evaluateAll((nodes) =>
        nodes.map((node) => {
          const range = document.createRange();
          range.selectNodeContents(node);
          const card = node.closest(".panel, .record-head");
          return {
            node: `${node.tagName}.${node.className}`,
            card: card?.className ?? null,
            past: card
              ? range.getBoundingClientRect().right -
                card.getBoundingClientRect().right
              : null,
          };
        }),
      );
      expect(verdicts.length).toBeGreaterThan(0);
      for (const verdict of verdicts) {
        expect(verdict.card, `${verdict.node} has a card`).not.toBeNull();
        expect(
          verdict.past,
          `${verdict.node} past ${verdict.card}`,
        ).toBeLessThanOrEqual(0.5);
      }
    });
  });
}
