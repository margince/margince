import { expect, type Locator, type Page, test } from "@playwright/test";

/**
 * The Brief — the page a rep and a lead open first.
 *
 * Behavioural tests (vitest) already prove the data flow: which rows render,
 * what a checkbox does not send, which absence draws which plate. What none of
 * them can see is that the readings strip came out as two stacked rows, that
 * the rail landed under the work column at desktop, or that a panel spilled
 * sideways under a long German label. That is what this suite is for.
 *
 * Two describes, and the split is the same one company-record.spec.ts makes:
 *
 *   - page shape — which regions exist and in what order;
 *   - what a reader can reach — nothing pans, at three widths and in both
 *     themes.
 *
 * Never pixel-equality and never the drawn English strings. The app renders
 * German, so a copy change must not fail a layout suite; assertions are
 * structural, or they are FLOORS ("at least this prominent"), which is the
 * claim a layout actually makes.
 *
 * It runs against a LIVE stack because the states that have to look right are
 * data states. Without BASE_URL the suite skips itself loudly rather than
 * passing on an app it never loaded.
 */

const BASE_URL = process.env.BASE_URL;

test.skip(
  !BASE_URL,
  "brief runs against a live stack: set BASE_URL (see make e2e-brief).",
);

const SHOTS = process.env.E2E_BRIEF_SHOT_DIR ?? "/tmp/e2e-brief";

// The readings row, by the test id HomeReadingsStrip hands its StatStrip. Not
// by class: the plate is the shared primitive's, so a class selector would
// match every other StatStrip in the app.
const STRIP = '[data-testid="home-readings"]';
const GLANCE = '[data-testid="home-glance"]';

/**
 * Sign in as the dev bootstrap admin, unless the session is already up.
 *
 * A dev stack already logged into redirects /#/login straight to the app, so
 * the form is ABSENT on the happy path — waiting for a navigation that never
 * happens is a 30s timeout rather than a login failure.
 */
async function signIn(page: Page) {
  await page.goto("/#/login", { waitUntil: "networkidle" });
  const email = page
    .locator('input[type="email"], input[name="email"]')
    .first();
  if ((await email.count()) === 0) {
    return;
  }
  await email.fill(process.env.E2E_EMAIL ?? "admin@demo.test");
  await page
    .locator('input[type="password"], input[name="password"]')
    .first()
    .fill(process.env.E2E_PASSWORD ?? "demo-password-123");
  await page.locator('button[type="submit"]').first().click();
  await expect(page.locator("nav.rail").first()).toBeVisible();
}

/**
 * Open the Brief and wait for the page to have actually settled.
 *
 * THREE waits, and the first two on their own are not enough. Home fans out to
 * five independent reads, so a route that has arrived is still a column of
 * skeletons; the glance paints first, so anchoring there measures a page whose
 * work column is empty and whose readings are still fading in. The first
 * capture taken that way photographed exactly that.
 *
 * So: the glance, then the work column having real content under it, then the
 * entry animations having finished — a box read mid-fade is where the element
 * passed through rather than where it rests.
 */
async function openBrief(page: Page) {
  await page.goto("/#/home", { waitUntil: "networkidle" });
  await expect(page.locator(GLANCE)).toBeVisible();
  await expect(page.locator(".home-main section").first()).toBeVisible();
  await settled(page);
}

/**
 * Wait out the entry animations.
 *
 * The shell's core mark is excluded by NAME: its only motion is an opacity
 * transition that cannot move a box, and it is in flight more or less always,
 * so waiting on it would time out on every call. Excluded by name rather than
 * by giving up on the check, which would also excuse a panel still sliding in.
 */
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

/** The horizontal position of a region, for beside-versus-under. */
async function leftOf(locator: Locator): Promise<number> {
  await expect(locator).toBeVisible();
  const box = await locator.boundingBox();
  if (!box) {
    throw new Error("region is visible but has no box — cannot place it");
  }
  return box.x;
}

/** A computed pixel measure, for the scale floors. */
async function px(locator: Locator, property: string): Promise<number> {
  await expect(locator).toBeVisible();
  const raw = await locator.evaluate(
    (element, name) => getComputedStyle(element).getPropertyValue(name),
    property,
  );
  return Number.parseFloat(raw);
}

/**
 * The shell rendered at all.
 *
 * A page that threw scores zero overflow and zero violations — a dead page
 * passing every check below. This is what stops that reading as a pass.
 */
async function expectShellRendered(page: Page) {
  await expect(page.locator("nav.rail")).toBeVisible();
}

/**
 * How far the PAGE scrolls sideways, and which scroller does it.
 *
 * NOT `document.body.scrollWidth`: the shell pins `.main` to `overflow: hidden`
 * and gives `.scroll` an `overflow-y: auto` that computes `overflow-x` to
 * `auto` with it, so wide content scrolls INSIDE the page's own scroller and
 * the body never grows a pixel. A body read here would be structurally
 * incapable of failing.
 *
 * A component that declares a horizontal scroll region of its own is not
 * measured: `.table-scroll` around a table too wide for a phone is the
 * sanctioned answer to wide content, and the reader panning the PAGE is what
 * this forbids.
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

  // Exactly one. The greeting is the page's heading, and the sentence under it
  // would be tempting to mark up as a second — which is both a layout defect
  // and an accessibility one, because a page with two h1s has no outline.
  test("carries exactly one h1", async ({ page }) => {
    await openBrief(page);
    await expectShellRendered(page);

    await expect(page.locator("main h1")).toHaveCount(1);
  });

  // The order the day sets. The glance says what is true this morning, the
  // readings qualify it, and the work follows — a strip of numbers above the
  // sentence that frames them is a page asking to be read backwards.
  test("puts the glance above the readings, and the readings above the work", async ({
    page,
  }) => {
    await openBrief(page);
    await expectShellRendered(page);

    const glance = await topOf(page.locator(GLANCE));
    const strip = await topOf(page.locator(STRIP));
    expect(glance).toBeLessThan(strip);
    // The FIRST section of the work column, whichever the day put there: the
    // deck leads when a decision is waiting and the ranked queue leads when it
    // is not, and the claim is about the strip sitting above the work, not
    // about which work it is.
    expect(strip).toBeLessThan(
      await topOf(page.locator(".home-main section").first()),
    );
  });

  // The retrospective is LAST, under the work. It is what a rep reads once on
  // Monday, after the work waiting on them today; above either of those it puts
  // last week ahead of this morning.
  test("keeps last week below today's work", async ({ page }) => {
    await openBrief(page);
    await expectShellRendered(page);

    expect(await topOf(page.locator("#home-today"))).toBeLessThan(
      await topOf(page.locator("#home-weekly")),
    );
  });

  // Frozen past above, live future below: a rep decides what next week holds by
  // reading what this one did.
  test("puts the week's plan under the week's review", async ({ page }) => {
    await openBrief(page);
    await expectShellRendered(page);

    expect(await topOf(page.locator("#home-weekly"))).toBeLessThan(
      await topOf(page.locator("#brief-plan")),
    );
  });

  // At desktop the rail is BESIDE the work, not under it. This is the assertion
  // that fails when a grid rule stops applying and the aside quietly reflows to
  // a second full-width block nobody notices from reading the code.
  test("keeps the rail beside the work at 1280", async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await openBrief(page);
    await expectShellRendered(page);

    expect(await leftOf(page.locator(".home-rail"))).toBeGreaterThan(
      await leftOf(page.locator(".home-main")),
    );
  });

  // A reading's VALUE must outweigh its label. Equal weight is the dense
  // admin-tool rendering these tests exist to catch; a floor, not equality,
  // because a value larger than the mockup drew is right and 12px is not.
  test("draws a reading's figure larger than its label", async ({ page }) => {
    await openBrief(page);
    await expectShellRendered(page);

    const card = page.locator(`${STRIP} .stat-card`).first();
    const value = await px(
      card.locator(".stat-card-value").first(),
      "font-size",
    );
    const label = await px(
      card.locator(".stat-card-label").first(),
      "font-size",
    );
    expect(value).toBeGreaterThan(label);
    expect(value).toBeGreaterThanOrEqual(13);
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

  // At phone width the rail stacks UNDER the work rather than being squeezed
  // beside it. A two-column layout at 390px is how a page comes to pan.
  test("stacks the rail under the work at 390", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await openBrief(page);
    await expectShellRendered(page);

    expect(await topOf(page.locator(".home-rail"))).toBeGreaterThan(
      await topOf(page.locator(".home-main")),
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
