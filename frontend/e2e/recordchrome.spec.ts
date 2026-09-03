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
 * 2. The details pane stands BESIDE the work column under the tab row, or
 *    UNDER the work column, never both-and-neither. Below the fold it is a
 *    pane at the foot of the record; on a phone the sidebar leaves the grid
 *    entirely, and a rule written for the tablet was still claiming a 252px
 *    rail track at 390px — which put the context in a second column beside a
 *    138px record.
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

test.describe("the record's details pane", () => {
  // Beside the record above the fold, and under it below. 900px and 390px are
  // both here because the rule that broke the phone was written for the
  // tablet: a fix swept at only the failing width is a fix nobody can trust
  // at the other one.
  const BESIDE = 1440;
  const STACKED = [900, 390];

  // The switch at the end of the tab row: the one pressed control with a glyph
  // and no word.
  function detailsSwitch(page: Page) {
    return page
      .locator(".recordtabs-trailing")
      .locator("button[aria-label]")
      .filter({ has: page.locator("svg") })
      .and(page.locator("[aria-pressed]"));
  }

  test(`opens beside the work column, under the tab row, at ${BESIDE}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: BESIDE, height: 900 });
    await openRecord(page, "/#/companies/o-brandt");

    const pane = page.locator(".record-aside");
    // Closed on arrival: the pane is where a reader goes for the attributes,
    // not what they open a record to see.
    await expect(pane, "the pane is open on arrival").toBeHidden();
    await detailsSwitch(page).click();
    await expect(pane, "the switch did not open the pane").toBeVisible();

    const paneBox = await pane.boundingBox();
    const work = await page.locator(".page-zones-main").boundingBox();
    const tabs = await page.locator(".record-tabs").boundingBox();
    if (!paneBox || !work || !tabs) {
      throw new Error(
        "the pane, the work column and the tab row are visible but one has no box",
      );
    }
    expect(
      paneBox.x,
      "the pane is not beside the work column",
    ).toBeGreaterThanOrEqual(work.x + work.width - 1);
    expect(
      paneBox.y,
      "the pane does not open under the tab row",
    ).toBeGreaterThanOrEqual(tabs.y + tabs.height - 1);
  });

  for (const width of STACKED) {
    test(`stacks the pane under the work column at ${width}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 844 });
      await openRecord(page, "/#/companies/o-brandt");

      const pane = page.locator(".record-aside");
      await expect(pane, "the pane is open on arrival").toBeHidden();
      await detailsSwitch(page).click();
      await expect(pane, "the switch did not open the pane").toBeVisible();

      const paneBox = await pane.boundingBox();
      const work = await page.locator(".page-zones-main").boundingBox();
      if (!paneBox || !work) {
        throw new Error(
          "the pane and the work column are visible but one has no box",
        );
      }
      // Under the work rather than squeezed beside it: two columns leave
      // neither readable at this width.
      expect(
        paneBox.y,
        "the pane stands beside the work column on a screen that fits one",
      ).toBeGreaterThanOrEqual(work.y + work.height - 1);
      // And it takes the record's whole width rather than a 300px stack at
      // the edge of it.
      expect(
        paneBox.width,
        "the pane keeps the narrow column's width in a region several times wider",
      ).toBeGreaterThan(work.width * 0.8);

      // And away again, from the same switch.
      await detailsSwitch(page).click();
      await expect(pane, "the pane is still on screen").toBeHidden();
    });
  }
});
