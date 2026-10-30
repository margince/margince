import { expect, type Page, test } from "@playwright/test";
import { RECORDS } from "./records";
import { type MockApiOptions, mockApi } from "./seed";
import { pageOverflow } from "./waits";

/**
 * The record's own chrome, measured rather than described.
 *
 * Every invariant here was stated in prose beside the code that was supposed to
 * hold it while the product drew something else:
 *
 * 1. An icon-only button is SQUARE. `.btn-icon` said so in a comment and set a
 *    width, and a width is half a square — a flex row stretched the button to
 *    the height of the labelled verbs beside it and left the width alone, so the
 *    ellipsis on every record header was 32×40. The unit gate asserted the class
 *    was present, which jsdom can see, and a rectangle is exactly what it
 *    cannot.
 * 2. The record is two cards in a row. The details card stands BESIDE the
 *    sheet, on one top edge and with the tab strip clear of it, or ABOVE the
 *    sheet once the row is under 900px wide, never both-and-neither. A rule
 *    written for the wide row was still claiming a second column at phone
 *    width, which put the context beside a record a few dozen pixels wide.
 * 3. The header verbs are tiles that show their name under the glyph, so the
 *    square rule below reads every icon-only button outside them.
 * 4. A record does not PAN. A strip or card wider than the row it sits in
 *    lets every record be dragged sideways under the reader's hands.
 *
 * Geometry, therefore, and in a browser: both defects were invisible to every
 * unit test in the tree and obvious in a screenshot.
 *
 * All of these belong to the record shell rather than to any one screen, so
 * they are measured on every record page in `./records`. A screen that draws
 * its own answer to either is what these tests exist to catch. Only the page
 * that draws it can show it. That list is a census rather than a habit.
 * `src/app/recordcensus.test.ts` holds it against the set the shell caps. A
 * record page missing from it fails there instead of going unmeasured here.
 */

async function openRecord(page: Page, route: string, options?: MockApiOptions) {
  // Everything here is measured as a BOX, and the record's blocks arrive with a
  // 10px rise (design-system/enter.css). A box read in flight is up to half a
  // step from where the layout put it; a control PRESSED in flight is worse
  // than that, because a press is preceded by bringing the control into view,
  // and on the two records whose tab strip is sticky the browser answers that
  // request mid-rise by scrolling the record a third of a screen — after which
  // the strip is stuck over columns that have slid under it, and the details
  // pane reads as having opened above the tabs. Reduced motion IS the end state
  // by construction, so every box is at rest from the first frame.
  await page.emulateMedia({ reducedMotion: "reduce" });
  await mockApi(page, options);
  await page.goto(route);
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
}

type Box = { name: string; width: number; height: number };

async function iconButtons(page: Page): Promise<Box[]> {
  return page
    .locator(".btn-icon:not(.record-split-verbs *)")
    .evaluateAll((buttons) =>
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

test.describe("the record's two cards", () => {
  // Wide enough for the details card to stand beside the sheet. The row stacks
  // them once its own container is under 900px, which a 1440px window is not.
  const BESIDE = 1440;
  const STACKED = [900, 390];

  async function box(page: Page, selector: string) {
    const found = await page.locator(selector).first().boundingBox();
    if (!found) {
      throw new Error(`${selector} is in the record but has no box`);
    }
    return found;
  }

  for (const record of RECORDS) {
    test(`stand side by side, tabs clear of the details, on a ${record.name} at ${BESIDE}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width: BESIDE, height: 900 });
      await openRecord(page, record.route);
      await expect(page.locator(".record-aside")).toBeVisible();

      const details = await box(page, ".record-split-details");
      const sheet = await box(page, ".record-split-sheet");
      const tabs = await box(page, ".recordtabs");
      expect(
        sheet.x,
        "the sheet is not beside the details card",
      ).toBeGreaterThanOrEqual(details.x + details.width - 1);
      expect(
        Math.abs(sheet.y - details.y),
        "the two cards do not open on one top edge",
      ).toBeLessThanOrEqual(1);
      expect(
        tabs.x,
        "the tab strip reaches under the details card",
      ).toBeGreaterThanOrEqual(details.x + details.width - 1);
    });

    for (const width of STACKED) {
      test(`stack the details card above the sheet on a ${record.name} at ${width}px`, async ({
        page,
      }) => {
        await page.setViewportSize({ width, height: 844 });
        await openRecord(page, record.route);
        await expect(page.locator(".record-aside")).toBeVisible();

        const details = await box(page, ".record-split-details");
        const sheet = await box(page, ".record-split-sheet");
        // Above rather than squeezed beside: two columns leave neither
        // readable at this width.
        expect(
          sheet.y,
          "the sheet stands beside the details card on a screen that fits one",
        ).toBeGreaterThanOrEqual(details.y + details.height - 1);
        // And each takes the row's whole width rather than a narrow column at
        // its edge.
        expect(
          Math.abs(details.width - sheet.width),
          "the stacked cards differ in width",
        ).toBeLessThanOrEqual(1);
      });
    }
  }
});

test.describe("the record's column holds still", () => {
  // The four widths the record is drawn at: a phone, a tablet, the reading
  // column's own measure, and a desktop wider than it.
  const WIDTHS = [390, 768, 1280, 1440];

  for (const record of RECORDS) {
    for (const width of WIDTHS) {
      test(`does not pan sideways on a ${record.name} at ${width}px`, async ({
        page,
      }) => {
        await page.setViewportSize({ width, height: 844 });
        await openRecord(page, record.route);
        // The strip is the block that breaks out, so a page measured before it
        // is on screen is a page measured without the thing under test.
        await expect(page.locator(".recordtabs")).toBeVisible();

        expect(
          await pageOverflow(page),
          "the record pans sideways: the reader can drag the page off the column it is reading",
        ).toEqual([]);
      });
    }
  }
});

test.describe("a refused rename under the record's name", () => {
  // The name is drawn in heading type. The refusal beneath it took that type
  // from its wrapper: a red second headline that wrapped over five lines.
  // The admin fixture holds no contact or company update (E2E_ADMIN_GRANTS), so
  // those two draw their name as text and have no rename to refuse.
  const RENAMED = RECORDS.filter(
    (record) => record.screen !== "contacts" && record.screen !== "companies",
  );
  for (const record of RENAMED) {
    test(`reads in body type on a ${record.name}`, async ({ page }) => {
      await page.setViewportSize({ width: 1440, height: 900 });
      await openRecord(page, record.route);

      const rename = page.locator(".record-split-rename");
      await rename.locator(".inline-editable").click();
      const field = rename.getByRole("textbox");
      await field.fill("");
      await field.press("Enter");
      const refusal = rename.getByRole("alert");
      await expect(refusal).toBeVisible();

      const type = (selector: string) =>
        page.locator(selector).evaluate((element) => {
          const style = getComputedStyle(element);
          return `${style.fontSize} ${style.fontWeight}`;
        });
      expect(
        await type(".record-split-rename [role=alert]"),
        "the refusal under the name is set in the name's heading type",
      ).toBe(await type("body"));
    });
  }
});
