import { expect, type Page, test } from "@playwright/test";
import { RECORDS } from "./records";
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
 * 2. ONE interval opens every record. The head, the tab strip, the band and
 *    the columns are four blocks on one scale, and the interval between them
 *    was a value each screen could reach: a stylesheet keyed the head's on
 *    whether the identity drew a meta row, so the strip sat a hair under the
 *    name on a lead and a full step under it on a company. A gap is a
 *    difference between two boxes and no unit test in jsdom, which lays out
 *    nothing, can subtract them.
 * 3. The details pane stands BESIDE the work column under the tab row, or
 *    UNDER the work column, never both-and-neither. Below the fold it is a
 *    pane at the foot of the record; on a phone the sidebar leaves the grid
 *    entirely, and a rule written for the tablet was still claiming a 252px
 *    rail track at 390px — which put the context in a second column beside a
 *    138px record.
 *
 * Geometry, therefore, and in a browser: both defects were invisible to every
 * unit test in the tree and obvious in a screenshot.
 *
 * Both belong to the record SHELL rather than to any one screen, so both are
 * measured on every record page in `./records`: a screen that draws its own
 * answer to either is the thing these tests exist to catch, and it can only be
 * caught on the page that draws it. That list is a census rather than a habit:
 * `src/app/recordcensus.test.ts` holds it against the set the shell caps, so a
 * record page missing from it fails there instead of going unmeasured here.
 */

async function openRecord(page: Page, route: string) {
  await mockApi(page);
  await page.goto(route);
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
}

// The switch at the end of the tab row: the one control there that SETS rather
// than does, so `aria-pressed` is what names it structurally — the word on it
// is copy this suite does not pin.
function detailsSwitch(page: Page) {
  return page.locator(".recordtabs-trailing button[aria-pressed]");
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

type Edge = { top: number; bottom: number };

/** A block's top and bottom in page coordinates, or null where it draws none —
 *  the band is absent on a record with nothing to say about itself as a whole. */
async function edge(page: Page, selector: string): Promise<Edge | null> {
  const found = page.locator(selector);
  if ((await found.count()) === 0) {
    return null;
  }
  const box = await found.boundingBox();
  if (!box) {
    throw new Error(`${selector} is in the record but has no box`);
  }
  return { top: box.y, bottom: box.y + box.height };
}

async function present(page: Page, selector: string): Promise<Edge> {
  const found = await edge(page, selector);
  if (!found) {
    throw new Error(`no ${selector} on this record`);
  }
  return found;
}

test.describe("the record's rhythm", () => {
  // Wide enough for the pane, and measured with it open: the open pane is the
  // shape the record spends its time in and the one that puts a second column
  // beside the blocks these gaps run between.
  const WIDE = 1440;

  /** The step the record opens on, read from the page's own scale rather than
   *  restated here: a suite that hard-codes 20px passes a tree that has moved
   *  the whole scale and fails one that has renamed nothing. */
  async function recordStep(page: Page): Promise<number> {
    const step = await page.evaluate(() =>
      Number.parseFloat(
        getComputedStyle(document.documentElement).getPropertyValue(
          "--space-5",
        ),
      ),
    );
    if (!Number.isFinite(step) || step <= 0) {
      throw new Error("the page publishes no --space-5 to measure against");
    }
    return step;
  }

  /** A pixel of tolerance, because a gap between two laid-out boxes is
   *  fractional and an interval that is one step is not a claim about the
   *  fourth decimal. */
  function isOneStep(gap: number, step: number, what: string) {
    expect(
      Math.abs(gap - step),
      `${what}: ${gap.toFixed(1)}px where the record's step is ${step}px`,
    ).toBeLessThanOrEqual(1);
  }

  for (const record of RECORDS) {
    test(`opens on one interval on a ${record.name}, band or no band`, async ({
      page,
    }) => {
      await page.setViewportSize({ width: WIDE, height: 900 });
      // The arrival is a 10px translate (design-system/enter.css) and
      // `getBoundingClientRect` reads transforms, so a block measured in
      // flight is up to half a step from where the layout put it — a gap this
      // suite would then report as a spacing defect that no stylesheet holds.
      // Reduced motion IS the end state by construction, so every box is at
      // rest from the first frame.
      await page.emulateMedia({ reducedMotion: "reduce" });
      await openRecord(page, record.route);
      await detailsSwitch(page).click();
      await expect(
        page.locator(".record-aside"),
        "the switch did not open the pane",
      ).toBeVisible();

      const step = await recordStep(page);
      const head = await present(page, ".record-head");
      const tabs = await present(page, ".record-tabs");
      const band = await edge(page, ".record-band");
      // The work column rather than the grid around it: a record with neither
      // rail nor aside draws no `.page-zones` at all, and this column is the
      // one block every shape puts at the columns' top edge.
      const columns = await present(page, ".page-zones-main");

      isOneStep(
        tabs.top - head.bottom,
        step,
        "the strip does not sit one interval under the identity",
      );
      // What follows the strip is the band on a record that carries one and
      // the columns on a record that does not, and the reader meets the same
      // interval either way — which is the whole invariant.
      const next = band ?? columns;
      isOneStep(
        next.top - tabs.bottom,
        step,
        "the block under the strip does not sit one interval below it",
      );
      if (band) {
        isOneStep(
          columns.top - band.bottom,
          step,
          "the columns do not sit one interval under the band",
        );
      }
    });
  }

  // The band's two intervals are measured only where a band is drawn, so a
  // sweep over records that all happen to draw none would report PASS having
  // measured half of what it names. This is the assertion that notices.
  test("at least one record in the sweep draws a band", async ({ page }) => {
    await page.setViewportSize({ width: WIDE, height: 900 });
    const withBand: string[] = [];
    for (const record of RECORDS) {
      await openRecord(page, record.route);
      if ((await page.locator(".record-band").count()) > 0) {
        withBand.push(record.name);
      }
    }
    expect(
      withBand,
      "no record page draws a band, so the band's intervals went unmeasured",
    ).not.toEqual([]);
  });
});

test.describe("the record's details pane", () => {
  // Beside the record above the fold, and under it below. 900px and 390px are
  // both here because the rule that broke the phone was written for the
  // tablet: a fix swept at only the failing width is a fix nobody can trust
  // at the other one.
  const BESIDE = 1440;
  const STACKED = [900, 390];

  for (const record of RECORDS) {
    test(`opens beside the work column, under the tab row, on a ${record.name} at ${BESIDE}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width: BESIDE, height: 900 });
      await openRecord(page, record.route);

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
      test(`stacks the pane under the work column on a ${record.name} at ${width}px`, async ({
        page,
      }) => {
        await page.setViewportSize({ width, height: 844 });
        await openRecord(page, record.route);

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
  }
});
