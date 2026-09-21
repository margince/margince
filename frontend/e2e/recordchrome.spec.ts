import { expect, type Page, test } from "@playwright/test";
import { RECORDS } from "./records";
import { type MockApiOptions, mockApi } from "./seed";

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
    return token(page, "--space-5");
  }

  /** The interval under the tab strip. On a record drawn on a sheet the strip
   *  is stuck inside the sheet and the first card under it keeps the stack's
   *  own gap, so the strip-to-card interval reads as one more card-to-card
   *  interval; every other record keeps the record step. */
  async function stripStep(page: Page): Promise<number> {
    const sheet = await page.locator(".record-sheet").count();
    return token(page, sheet > 0 ? "--space-6" : "--space-5");
  }

  async function token(page: Page, name: string): Promise<number> {
    const value = await page.evaluate(
      (name) =>
        Number.parseFloat(
          getComputedStyle(document.documentElement).getPropertyValue(name),
        ),
      name,
    );
    if (!Number.isFinite(value) || value <= 0) {
      throw new Error(`the page publishes no ${name} to measure against`);
    }
    return value;
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

  /** Opens the record, whose details pane is out on arrival, measures every
   *  interval the opening claims, and answers the band's edges, or null on a
   *  record with nothing to say about itself as a whole, which draws none. */
  async function measureOpening(
    page: Page,
    route: string,
    options?: MockApiOptions,
  ): Promise<Edge | null> {
    await page.setViewportSize({ width: WIDE, height: 900 });
    await openRecord(page, route, options);
    await expect(
      page.locator(".record-aside"),
      "the pane is not open on arrival",
    ).toBeVisible();

    const step = await recordStep(page);
    const head = await present(page, ".record-head");
    const tabs = await present(page, "[data-testid='record-tabs']");
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
      await stripStep(page),
      "the block under the strip does not sit one interval below it",
    );
    if (band) {
      isOneStep(
        columns.top - band.bottom,
        step,
        "the columns do not sit one interval under the band",
      );
    }
    return band;
  }

  for (const record of RECORDS) {
    test(`opens on one interval on a ${record.name}, band or no band`, async ({
      page,
    }) => {
      await measureOpening(page, record.route);
    });
  }

  // The band's two intervals are measured only where a band is drawn, and a
  // record draws one only when it has something to say about itself as a whole
  // — which every record in the seed, all of them writable and none closed,
  // has not. The sweep above would then report PASS having measured half of
  // what it names, with no failing assertion to notice.
  //
  // So this case DRAWS one rather than hoping to find one: the same project
  // page, seeded as a row this caller may not write, whose band carries the one
  // sentence that refuses an edit. The band is asserted present, so the case
  // cannot go quiet the way the census it replaces did.
  test("measures the band's own intervals on a record that draws one", async ({
    page,
  }) => {
    // The project's own route from the census rather than a second spelling of
    // it: `src/app/recordcensus.test.ts` holds that entry against the shell.
    const project = RECORDS.find((record) => record.screen === "projects");
    if (!project) {
      throw new Error("no project route in the record census to measure");
    }
    const band = await measureOpening(page, project.route, {
      project: "read-only",
    });
    expect(
      band,
      "the read-only project drew no band, so the band's intervals went unmeasured",
    ).not.toBeNull();
  });
});

test.describe("the record's details pane", () => {
  // Tablet panes stack below the record. Contacts use a phone drawer; other
  // records retain the stacked pane. Both widths must remain usable.
  const BESIDE = 1440;
  const STACKED = [900, 390];

  for (const record of RECORDS) {
    test(`opens beside the work column, under the tab row, on a ${record.name} at ${BESIDE}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width: BESIDE, height: 900 });
      await openRecord(page, record.route);

      const pane = page.locator(".record-aside");
      // Open on arrival: the attributes stand beside the work from the first
      // frame, and the switch at the end of the tab row is how a reader
      // folds them away and brings them back.
      await expect(pane, "the pane is not open on arrival").toBeVisible();
      await detailsSwitch(page).click();
      await expect(pane, "the switch did not close the pane").toBeHidden();
      await detailsSwitch(page).click();
      await expect(pane, "the switch did not reopen the pane").toBeVisible();

      // At rest before measuring. The tab strip is STICKY (composed.css), so
      // once the record is scrolled it pins to the top while the pane travels
      // with the content — and the two boxes stop describing the layout this
      // asserts. Clicking the switch scrolls it into view, and how far depends
      // on how tall the head above it is, which is a property of the record
      // rather than of the rule being checked here.
      //
      // The shell scrolls an inner `.scroll` container, not the window, so
      // `window.scrollTo` is a no-op here and the strip stayed pinned through
      // one.
      await page.evaluate(() => {
        for (const box of document.querySelectorAll(".scroll")) {
          box.scrollTop = 0;
        }
      });
      const paneBox = await pane.boundingBox();
      const work = await page.locator(".page-zones-main").boundingBox();
      const tabs = await page
        .locator("[data-testid='record-tabs']")
        .boundingBox();
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
      const mobileDrawer = record.name === "contact" && width === 390;
      const behavior = mobileDrawer
        ? "opens Details in a drawer"
        : "stacks the pane under the work column";
      test(`${behavior} on a ${record.name} at ${width}px`, async ({
        page,
      }) => {
        await page.setViewportSize({ width, height: 844 });
        await openRecord(page, record.route);

        const pane = page.locator(".record-aside");
        if (mobileDrawer) {
          // The phone drawer is the one shape that waits to be asked for.
          await expect(pane, "the pane is open on arrival").toBeHidden();
          await detailsSwitch(page).click();
          const dialog = page.getByRole("dialog", {
            name: "Details & Berechtigungen",
          });
          await expect(dialog).toBeVisible();
          await expect(dialog.getByTestId("contact-rail")).toBeVisible();
          await expect(pane).toHaveCount(0);
          // Where the drawer comes to REST, not where it is on the frame it
          // became visible: it arrives by sliding in from the trailing edge,
          // so measured on the first visible frame it is still most of a
          // screen to the right of where it lands. Retried rather than waited
          // out, so the assertion needs no copy of the duration.
          await expect(async () => {
            const box = await dialog.boundingBox();
            if (!box) throw new Error("the visible Details drawer has no box");
            expect(box.x).toBeGreaterThanOrEqual(0);
            expect(box.x + box.width).toBeLessThanOrEqual(width + 1);
            expect(box.width).toBeGreaterThan(width * 0.9);
          }).toPass();
          await page.keyboard.press("Escape");
          await expect(dialog).toBeHidden();
          await expect(detailsSwitch(page)).toBeFocused();
          return;
        }
        await expect(pane, "the pane is not open on arrival").toBeVisible();

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
