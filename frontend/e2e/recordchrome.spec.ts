import { expect, type Page, test } from "@playwright/test";
import { mockApi } from "./seed";

/**
 * The record's own chrome, measured rather than described.
 *
 * Two invariants live here, and both were stated in prose beside the code that
 * was supposed to hold them while the product drew something else:
 *
 * 1. An icon-only button is SQUARE. `.btn-icon` said so in a comment and set a
 *    width, and a width is half a square — a flex row stretched the button to
 *    the height of the labelled verbs beside it and left the width alone, so the
 *    ellipsis on every record header was 32×40. The unit gate asserted the class
 *    was present, which jsdom can see, and a rectangle is exactly what it
 *    cannot.
 * 2. The context column stands BESIDE the work column or UNDER it, never
 *    both-and-neither. Below the fold it is a panel at the foot of the page; on
 *    a phone the sidebar leaves the grid entirely, and a rule written for the
 *    tablet was still claiming a 252px rail track at 390px — which put the
 *    context in a second column beside a 138px record.
 *
 * Geometry, therefore, and in a browser: both defects were invisible to every
 * unit test in the tree and obvious in a screenshot.
 */

const RECORDS = [
  { name: "contact", route: "/#/contacts/p-anna" },
  { name: "lead", route: "/#/leads/l-1" },
  { name: "company", route: "/#/companies/o-brandt" },
];

async function openRecord(page: Page, route: string) {
  await mockApi(page);
  await page.goto(route);
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
}

type Box = { name: string; width: number; height: number };

async function iconButtons(page: Page): Promise<Box[]> {
  return page.locator(".btn-icon").evaluateAll((buttons) =>
    buttons.map((button) => {
      const box = button.getBoundingClientRect();
      return {
        name:
          button.getAttribute("aria-label") ??
          button.textContent?.trim() ??
          "(unnamed)",
        width: box.width,
        height: box.height,
      };
    }),
  );
}

test.describe("a record's icon-only buttons", () => {
  for (const record of RECORDS) {
    // Both rungs are in scope: a header's verbs are 40px and a table row's are
    // 32, and the defect was a button taking one figure from each.
    test(`are square on a ${record.name}`, async ({ page }) => {
      await page.setViewportSize({ width: 1440, height: 900 });
      await openRecord(page, record.route);

      const buttons = await iconButtons(page);
      expect(
        buttons.length,
        "no icon-only button was found, so this measured nothing",
      ).toBeGreaterThan(0);
      const oblong = buttons.filter(
        (button) => Math.abs(button.width - button.height) > 0.5,
      );
      expect(
        oblong,
        "an icon-only button is drawn as a rectangle: it has a width from its own size rung and a height from the row it sits in",
      ).toEqual([]);
    });
  }
});

test.describe("the record's context column", () => {
  // Beside the record above the fold, and one-at-a-time below it. 900px and
  // 390px are both here because the rule that broke the phone was written for
  // the tablet: a fix swept at only the failing width is a fix nobody can trust
  // at the other one.
  const BESIDE = 1440;
  const SWAPPED = [900, 390];

  test(`stands beside the work column at ${BESIDE}px`, async ({ page }) => {
    await page.setViewportSize({ width: BESIDE, height: 900 });
    await openRecord(page, "/#/companies/o-brandt");

    const aside = await page.locator(".pageaside").boundingBox();
    const main = await page.locator("main.main").boundingBox();
    if (!aside || !main) {
      throw new Error(
        "the column and the work column are visible but one has no box",
      );
    }
    expect(
      aside.x,
      "the context column is not beside the work column",
    ).toBeGreaterThanOrEqual(main.x + main.width - 1);
  });

  for (const width of SWAPPED) {
    test(`shows the record OR its context at ${width}px, never both`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 844 });
      await openRecord(page, "/#/companies/o-brandt");

      const main = page.locator("main.main");
      const aside = page.locator(".pageaside");
      // Arriving shows the record. The remembered preference belongs to a desk
      // with room for both columns; below the fold, opening on the context would
      // hide the very thing the reader asked for.
      await expect(
        main,
        "the record is not on screen on arrival",
      ).toBeVisible();
      await expect(
        aside,
        "the context is on screen on arrival, in place of the record that was opened",
      ).toBeHidden();

      // The switch stands with the record's verbs on the pages that keep it
      // there, and at the end of the tab row on the pages that have taken the
      // design's glance; either way it is the one pressed control with a
      // glyph and no word.
      const switchToContext = page
        .locator(".record-actions, .recordtabs-trailing")
        .locator("button[aria-label]")
        .filter({ has: page.locator("svg") })
        .and(page.locator("[aria-pressed]"));
      await switchToContext.click();

      await expect(
        aside,
        "the switch did not bring the context on screen",
      ).toBeVisible();
      await expect(
        main,
        "the record is still drawn behind the context, so both share a screen that fits one",
      ).toBeHidden();

      // And it takes the whole column rather than a 288px stack in the middle of
      // it: the fixed width the open column's fold needs is not a width this
      // state has any use for.
      const box = await aside.boundingBox();
      const body = await page.locator(".pageaside-body").boundingBox();
      if (!box || !body) {
        throw new Error("the context column is visible but has no box");
      }
      expect(
        body.width,
        "the context's cards keep the narrow column's width in a region several times wider",
      ).toBeGreaterThan(box.width * 0.8);

      // Back to the record, from the panel's own head.
      await page.locator(".pageaside-fold").click();
      await expect(main, "the record did not come back").toBeVisible();
      await expect(aside, "the context is still on screen").toBeHidden();
    });
  }
});
