import { expect, type Locator, type Page, test } from "@playwright/test";
import { de } from "../src/i18n/de";
import type { MessageKey } from "../src/i18n/en";
import { signIn } from "./waits";

/**
 * The Brief, the page a rep and a lead open first.
 *
 * Behavioural tests (vitest) already prove the data flow: which rows render,
 * what a checkbox does not send, which absence draws which plate. What none of
 * them can see is that the readings strip came out as two stacked rows. Nor
 * can they see a focus panel that lost its full width, or a panel that spilled
 * sideways under a long German label. That is what this suite is for.
 *
 * Two describes, and the split is the same one company-record.spec.ts makes:
 *
 *   - page shape, which regions exist and in what order;
 *   - what a reader can reach, nothing pans, at three widths and in both
 *     themes.
 *
 * Never pixel-equality and never the drawn English strings. The app renders
 * German, so a copy change must not fail a layout suite. Assertions are
 * structural, or floors ("at least this prominent").
 *
 * It runs against a live stack because the states that have to look right are
 * data states. Without BASE_URL the suite skips itself loudly.
 */

const BASE_URL = process.env.BASE_URL;

test.skip(
  !BASE_URL,
  "brief runs against a live stack: set BASE_URL (see make e2e-brief).",
);

const SHOTS = process.env.E2E_BRIEF_SHOT_DIR ?? "/tmp/e2e-brief";

// The readings row, by the test id BriefReadingsStrip hands its StatStrip. Not
// by class: the plate is the shared primitive's, so a class selector would
// match every other StatStrip in the app.
/**
 * A string the app actually renders, read from the catalog the component reads.
 *
 * The app renders German. Writing the words here would put a second copy of
 * them in the tree. The two cases below name copy because a heading nobody can
 * read is a defect no box measurement can see.
 */
function copy(key: MessageKey): string {
  return de[key];
}

const STRIP = '[data-testid="brief-readings"]';
const FOCUS = "#brief-today";
const GLANCE = '[data-testid="brief-glance"]';

/**
 * Open the Brief and wait for the page to have actually settled.
 *
 * Brief fans out to five independent reads, so an arrived route is still a
 * column of skeletons. The glance paints first, so anchoring only there
 * measures a page whose work column is empty.
 *
 * So: the glance, then the work column having real content, then the entry
 * animations finished. A box read mid-fade is not where the element rests.
 */
async function openBrief(page: Page) {
  await page.goto("/#/home", { waitUntil: "networkidle" });
  await expect(page.locator(GLANCE)).toBeVisible();
  await expect(page.locator(FOCUS)).toBeVisible();
  await settled(page);
}

/**
 * Wait out the entry animations.
 *
 * The shell's core mark is excluded by name. Its only motion is an opacity
 * transition that cannot move a box, and it is almost always in flight.
 * Excluding by name keeps the check for a panel still sliding in.
 */
/** Open the Brief on its weekly view, settled the same way the morning is. */
async function openWeekly(page: Page) {
  await page.goto("/#/home?view=weekly", { waitUntil: "networkidle" });
  await expect(page.locator(GLANCE)).toBeVisible();
  await expect(page.locator("#brief-weekly")).toBeVisible();
  await settled(page);
}

async function settled(page: Page) {
  await page.waitForFunction(
    (ambient) =>
      document
        .getAnimations()
        .filter((animation) => animation.playState === "running")
        .every((animation) => {
          const effect = animation.effect;
          const target =
            effect instanceof KeyframeEffect && effect.target instanceof Element
              ? `${effect.target.tagName}.${effect.target.className}`
              : "";
          return ambient.some((mark) => target.includes(mark));
        }),
    ["core-rim", "core-glass"],
  );
}

/** The vertical position of a region, for order assertions. */
async function topOf(locator: Locator): Promise<number> {
  await expect(locator).toBeVisible();
  const box = await locator.boundingBox();
  if (!box) {
    throw new Error("region is visible but has no box — cannot order it");
  }
  return box.y;
}

/**
 * The shell rendered at all.
 *
 * A page that threw scores zero overflow and zero violations, a dead page
 * passing every check below. This is what stops that reading as a pass.
 */
async function expectShellRendered(page: Page) {
  await expect(page.locator("nav.rail")).toBeVisible();
}

/**
 * How far the page scrolls sideways, and which scroller does it.
 *
 * Not `document.body.scrollWidth`: the shell pins `.main` to `overflow: hidden`
 * and gives `.scroll` an `overflow-y: auto` that computes `overflow-x` to
 * `auto` with it. Wide content scrolls inside `.scroll` and the body never
 * grows, so a body read could not fail.
 *
 * A component with its own horizontal scroll region is not measured.
 * `.table-scroll` around a wide table is sanctioned; panning the page is not.
 */
async function pageOverflow(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const scrollers: { name: string; element: Element }[] = [
      { name: "the document", element: document.documentElement },
      ...Array.from(document.querySelectorAll(".scroll")).map((element) => ({
        name: "the shell content scroller (.scroll)",
        element,
      })),
    ];
    return scrollers
      .map(({ name, element }) => ({
        name,
        overflow: element.scrollWidth - element.clientWidth,
      }))
      .filter(({ overflow }) => overflow > 0)
      .map(({ name, overflow }) => `${name}: ${overflow}px past the viewport`);
  });
}

test.describe("the Brief — page shape", () => {
  test.beforeEach(async ({ page }) => {
    await signIn(page);
  });

  // One h1. The greeting is the page's heading; marking up the sentence
  // under it as a second h1 leaves the page with no outline.
  test("carries exactly one h1", async ({ page }) => {
    await openBrief(page);
    await expectShellRendered(page);

    await expect(page.locator("main h1")).toHaveCount(1);
  });

  test("puts focus before readings and follow-through", async ({ page }) => {
    await openBrief(page);
    await expectShellRendered(page);

    const glance = await topOf(page.locator(GLANCE));
    const focus = await topOf(page.locator(FOCUS));
    const strip = await topOf(page.locator(STRIP));
    expect(glance).toBeLessThan(focus);
    expect(focus).toBeLessThan(strip);
    expect(strip).toBeLessThan(
      await topOf(page.locator(".brief-followthrough")),
    );
  });

  test("orders the greeting above the date above the sentence", async ({
    page,
  }) => {
    await openBrief(page);
    await expectShellRendered(page);

    const heading = await topOf(page.locator("main h1"));
    const date = await topOf(page.locator(`${GLANCE} time`));
    expect(heading).toBeLessThan(date);
    expect(date).toBeLessThan(
      await topOf(page.locator('[data-testid="glance-sentence"]')),
    );
  });

  test("says the morning in one paragraph, unclipped", async ({ page }) => {
    await openBrief(page);
    await expectShellRendered(page);

    await expect(page.locator(`${GLANCE} p.glance-sentence`)).toHaveCount(1);
    const clipped = await page
      .locator('[data-testid="glance-sentence"]')
      .evaluate((el) => el.scrollHeight > el.clientHeight);
    expect(clipped).toBe(false);
  });

  // The strip is five slots on every morning, including a quiet one. A row that
  // shrank would read as fewer questions asked, so the two slots the product
  // cannot answer yet stay in the row.
  test("the readings strip is five slots wide", async ({ page }) => {
    await openBrief(page);
    await expectShellRendered(page);

    await expect(page.locator(`${STRIP} > *`)).toHaveCount(5);
  });

  // The week lives behind the Weekly dial, not at the foot of the morning.
  //
  // Frozen past above, live future below: a rep decides what next week holds by
  // reading what this one did.
  test("puts the week's plan under the week's review", async ({ page }) => {
    await openWeekly(page);
    await expectShellRendered(page);

    expect(await topOf(page.locator("#brief-weekly"))).toBeLessThan(
      await topOf(page.locator("#brief-plan")),
    );
  });

  // The weekly uses the full width, because it has no rail.
  //
  // Every rail panel answers a question about today, so none belongs beside a
  // week that closed. A grid track does not collapse when its item is missing.
  // Keeping the aside shape would draw the weekly at seventy per cent width
  // beside an empty column. Measured, because the defect is a width a reader sees.
  test("gives the weekly the whole width", async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await openWeekly(page);
    await expectShellRendered(page);

    await expect(page.locator(".brief-rail")).toHaveCount(0);
    const review = await page.locator("#brief-weekly").boundingBox();
    const header = await page.locator(".brief-head").boundingBox();
    if (!review || !header) {
      throw new Error("the weekly view drew no review to measure");
    }
    // The review fills the same content width as the page header.
    expect(review.width).toBeGreaterThan(header.width * 0.9);
  });

  // And the morning shows the morning's work, the weekly is a dial away, not
  // a section further down the same page.
  test("keeps the week off the morning", async ({ page }) => {
    await openBrief(page);
    await expectShellRendered(page);

    await expect(page.locator(FOCUS)).toBeVisible();
    await expect(page.locator("#brief-weekly")).toHaveCount(0);
  });

  // The page a German reader arrives at is in German, and says what it is.
  //
  // Every other assertion here is geometry. A page rendering raw message keys
  // would satisfy all of it. One catalog read separates a rendered page from a
  // rendered skeleton.
  test("labels focus and view controls in German", async ({ page }) => {
    await openBrief(page);
    await expectShellRendered(page);

    await expect(
      page.getByRole("heading", { name: copy("brief.feed.title") }),
    ).toBeVisible();
    await expect(
      page.getByRole("group", { name: copy("brief.view.label") }),
    ).toBeVisible();
  });

  // The dial pressed, not the address typed.
  //
  // openWeekly() navigates straight to ?view=weekly, which proves the address
  // resolves and nothing about the control. A dial wired to the wrong writer
  // leaves every other test green.
  test("switches to the week when the dial is pressed", async ({ page }) => {
    await openBrief(page);
    await expectShellRendered(page);

    await page.getByRole("button", { name: copy("brief.view.weekly") }).click();

    await expect(page.locator("#brief-weekly")).toBeVisible();
    await expect(page.locator(FOCUS)).toHaveCount(0);
    expect(page.url()).toContain("view=weekly");
  });

  test("gives focus the full content width at 1280", async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await openBrief(page);
    await expectShellRendered(page);

    const focus = await page.locator(FOCUS).boundingBox();
    const header = await page.locator(".brief-head").boundingBox();
    if (!focus || !header) {
      throw new Error("the morning drew no focus panel to measure");
    }
    expect(focus.width).toBeGreaterThan(header.width * 0.9);
  });
});

test.describe("the Brief — nothing pans", () => {
  test.beforeEach(async ({ page }) => {
    await signIn(page);
  });

  // Three widths, and the middle one is the case a desktop-only check misses.
  for (const [name, width, height] of [
    ["desktop", 1280, 800],
    ["phone", 390, 844],
    ["200% zoom", 640, 400],
  ] as const) {
    test(`does not scroll sideways at ${name}`, async ({ page }) => {
      await page.setViewportSize({ width, height });
      await openBrief(page);
      await expectShellRendered(page);

      expect(await pageOverflow(page)).toEqual([]);
    });
  }

  test("keeps follow-through below focus and readings at 390", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await openBrief(page);
    await expectShellRendered(page);

    const strip = await topOf(page.locator(STRIP));
    expect(await topOf(page.locator(FOCUS))).toBeLessThan(strip);
    expect(strip).toBeLessThan(
      await topOf(page.locator(".brief-followthrough")),
    );
  });

  // The dark palette is its own layout: tokens defined only under one scheme
  // are a real defect class, and a suite pinned to light never sees it.
  test.describe("in the dark palette", () => {
    test.use({ colorScheme: "dark" });

    test("holds its order and still does not pan", async ({ page }) => {
      await page.setViewportSize({ width: 1280, height: 800 });
      await openBrief(page);
      await expectShellRendered(page);

      expect(await topOf(page.locator(GLANCE))).toBeLessThan(
        await topOf(page.locator(STRIP)),
      );
      expect(await pageOverflow(page)).toEqual([]);
    });
  });
});

test.describe("the Brief — for the eye", () => {
  // The structural assertions above prove which regions exist and in what
  // order. None of them reads a colour, a weight or a spacing, so a page can
  // satisfy every one and still look wrong. The capture is for a human to
  // compare; it goes outside the repo, because a screenshot is session debris.
  test("captures the page", async ({ page }) => {
    await signIn(page);
    await page.setViewportSize({ width: 1280, height: 800 });
    await openBrief(page);
    await expectShellRendered(page);

    await page.screenshot({
      path: `${SHOTS}/brief-morning.png`,
      fullPage: true,
    });
  });
});
