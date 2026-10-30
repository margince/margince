import { expect, type Locator, type Page, test } from "@playwright/test";
import type { Worklist } from "../src/features/worklist/worklist.queries";
import { de } from "../src/i18n/de";
import type { MessageKey } from "../src/i18n/en";
import { signIn } from "./waits";

/**
 * The Brief's layout. Unit tests prove the data flow; only a browser sees a
 * region stack, lose its width or spill sideways under a long German label.
 * Assertions are structural or floors, never pixels or drawn strings.
 * It needs a live stack, and without BASE_URL it skips by name.
 */

const BASE_URL = process.env.BASE_URL;

test.skip(
  !BASE_URL,
  "brief runs against a live stack: set BASE_URL (see make e2e-brief).",
);

const SHOTS = process.env.E2E_BRIEF_SHOT_DIR ?? "/tmp/e2e-brief";

/**
 * A German string, read from the catalog the app renders from. A copy of the
 * words here would fail this suite whenever a translator improves them.
 */
function copy(key: MessageKey): string {
  return de[key];
}

// By test id, not class: the plate class belongs to every StatStrip.
const STRIP = '[data-testid="brief-readings"]';
const FOCUS = "#brief-today";
const GLANCE = '[data-testid="brief-glance"]';

/**
 * Open the Brief and wait until it rests. The glance paints before the work
 * column has content, and a box read mid-fade is not where it rests. So it
 * waits for the glance, then the work, then the entry animations.
 */
async function openBrief(page: Page): Promise<Worklist> {
  const loaded = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname.endsWith("/worklist") &&
      response.request().method() === "GET",
  );
  await page.goto("/#/home", { waitUntil: "networkidle" });
  await expect(page.locator(GLANCE)).toBeVisible();
  await expect(page.locator(FOCUS)).toBeVisible();
  await settled(page);
  const response = await loaded;
  expect(response.ok()).toBe(true);
  return response.json();
}

/** Open the Brief on its weekly view, settled the same way the morning is. */
async function openWeekly(page: Page) {
  await page.goto("/#/home?view=weekly", { waitUntil: "networkidle" });
  await expect(page.locator(GLANCE)).toBeVisible();
  await expect(page.locator("#brief-weekly")).toBeVisible();
  await settled(page);
}

/**
 * Entry motion must finish before measuring boxes. Perpetual paint-only
 * loops have no resting frame and do not change the layout.
 */
async function settled(page: Page) {
  await page.waitForFunction(
    (ambient) =>
      document
        .getAnimations()
        .filter((animation) => animation.playState === "running")
        .filter((animation) => animation.timeline instanceof DocumentTimeline)
        .every((animation) => {
          const effect = animation.effect;
          const target =
            effect instanceof KeyframeEffect && effect.target instanceof Element
              ? `${effect.target.tagName}.${effect.target.className}`
              : "";
          if (ambient.some((mark) => target.includes(mark))) return true;
          if (
            Number.isFinite(
              effect?.getComputedTiming().activeDuration ?? Infinity,
            )
          ) {
            return false;
          }
          if (
            animation instanceof CSSAnimation &&
            animation.animationName === "brief-ring-turn"
          ) {
            return true;
          }
          return false;
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

/** The shell rendered: a page that threw would pass every overflow check. */
async function expectShellRendered(page: Page) {
  await expect(page.locator("nav.rail")).toBeVisible();
}

/**
 * How far the page scrolls sideways, and which scroller does it. The body
 * never grows, because wide content scrolls inside the shell's `.scroll`.
 * A component's own scroll region, such as `.table-scroll`, is allowed.
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

  // The greeting is the page's heading. A page with two h1s has no outline.
  test("carries exactly one h1", async ({ page }) => {
    await openBrief(page);
    await expectShellRendered(page);

    await expect(page.locator("main h1")).toHaveCount(1);
  });

  // The brief and the work in one column, the readings beside it in the rail.
  test("puts the work under the brief and the readings in the rail", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await openBrief(page);
    await expectShellRendered(page);

    expect(await topOf(page.locator(GLANCE))).toBeLessThan(
      await topOf(page.locator(FOCUS)),
    );
    await expect(page.locator(`.brief-rail ${STRIP}`)).toHaveCount(1);
    await expect(page.locator(`.brief-main ${FOCUS}`)).toHaveCount(1);
    const main = await page.locator(".brief-main").boundingBox();
    const rail = await page.locator(".brief-rail").boundingBox();
    if (!main || !rail) {
      throw new Error("the desktop Brief columns have no measurable boxes");
    }
    expect(rail.x).toBeGreaterThanOrEqual(main.x + main.width);
  });

  test("keeps the greeting and date above the work, with an optional factual agenda", async ({
    page,
  }) => {
    await openBrief(page);
    await expectShellRendered(page);

    const heading = await topOf(page.locator("main h1"));
    const date = await topOf(page.locator(`${GLANCE} time`));
    const work = await topOf(page.locator(FOCUS));
    expect(heading).toBeLessThan(work);
    expect(date).toBeLessThan(work);
    const agenda = page.getByTestId("glance-sentence");
    if (await agenda.count()) expect(date).toBeLessThan(await topOf(agenda));
  });

  test("summarizes only complete positive focus, in an unclipped paragraph", async ({
    page,
  }) => {
    const day = await openBrief(page);
    await expectShellRendered(page);

    const complete =
      day.focus &&
      day.focus.total > 0 &&
      day.focus.total <= day.focus.items.length &&
      !day.scope_truncated &&
      !day.readings?.more_available &&
      !day.counts.some((count) => count.more_available) &&
      !day.sources_unavailable.some(
        (source) => source.contributes_rows !== false,
      );
    await expect(page.getByTestId("glance-sentence")).toHaveCount(
      complete ? 1 : 0,
    );
    await expect(page.locator(`${GLANCE} p.glance-sentence`)).toHaveCount(
      complete ? 1 : 0,
    );
    if (complete) {
      const clipped = await page
        .getByTestId("glance-sentence")
        .evaluate((el) => el.scrollHeight > el.clientHeight);
      expect(clipped).toBe(false);
    }
  });

  // Five slots on every morning, a quiet one too. A slot with nothing to show
  // says so, so a short row never reads as fewer questions asked.
  test("the readings strip is five slots wide", async ({ page }) => {
    await openBrief(page);
    await expectShellRendered(page);

    await expect(page.locator(`${STRIP} > *`)).toHaveCount(5);
  });

  // Frozen past above, live future below: a rep decides what next week holds by
  // reading what this one did.
  test("puts the week's plan under the week's review", async ({ page }) => {
    await openWeekly(page);
    await expectShellRendered(page);

    expect(await topOf(page.locator("#brief-weekly"))).toBeLessThan(
      await topOf(page.locator("#brief-plan")),
    );
  });

  // The weekly has no rail, since every rail panel is about today. A grid track
  // stays when its item is gone, so the width is measured, not the class.
  test("gives the weekly the whole width", async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await openWeekly(page);
    await expectShellRendered(page);

    await expect(page.locator(".brief-rail")).toHaveCount(0);
    const pane = await page.locator(".brief-weekly-page").boundingBox();
    const header = await page.locator(".brief-head").boundingBox();
    if (!pane || !header) {
      throw new Error("the weekly view drew no pane to measure");
    }
    expect(Math.abs(pane.width - header.width)).toBeLessThan(1);
  });

  // The head stands over the panes in both views, so pressing the dial does
  // not move it from under the pointer.
  test("keeps the dial in place when the view changes", async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await openBrief(page);
    await expectShellRendered(page);
    const dial = page.getByRole("group", { name: copy("brief.view.label") });
    const before = await dial.boundingBox();

    await page.getByRole("button", { name: copy("brief.view.weekly") }).click();
    await expect(page.locator("#brief-weekly")).toBeVisible();
    const after = await dial.boundingBox();
    if (!before || !after) throw new Error("the view dial has no box");
    expect(
      Math.abs(after.x + after.width - before.x - before.width),
    ).toBeLessThan(2);
    expect(Math.abs(after.y - before.y)).toBeLessThan(2);
  });

  // The weekly is a dial away, not a section further down the morning.
  test("keeps the week off the morning", async ({ page }) => {
    await openBrief(page);
    await expectShellRendered(page);

    await expect(page.locator(FOCUS)).toBeVisible();
    await expect(page.locator("#brief-weekly")).toHaveCount(0);
  });

  // The page a German reader arrives at is in German, and says what it is.
  //
  // Every other assertion here is geometry, and a page of raw message keys
  // would pass it. Five boxes in a row are five boxes whether they read
  // "Zuerst" or "brief.first.title". One catalog read is what separates a
  // rendered page from a rendered skeleton.
  test("labels focus and view controls in German", async ({ page }) => {
    await openBrief(page);
    await expectShellRendered(page);

    await expect(
      page.getByRole("heading", { name: copy("brief.first.title") }),
    ).toBeVisible();
    await expect(
      page.getByRole("group", { name: copy("brief.view.label") }),
    ).toBeVisible();
  });

  // The dial PRESSED, not the address typed.
  //
  // openWeekly() types the address, which proves nothing about the control. A
  // dial wired to the wrong writer would leave every other test here green.
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
    const column = await page.locator(".brief-main").boundingBox();
    if (!focus || !column) {
      throw new Error("the morning drew no focus panel to measure");
    }
    expect(focus.width).toBeGreaterThan(column.width * 0.9);
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

  // One column on a phone: the rail's readings follow the work.
  test("keeps the readings below the work at 390", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await openBrief(page);
    await expectShellRendered(page);

    expect(await topOf(page.locator(FOCUS))).toBeLessThan(
      await topOf(page.locator(STRIP)),
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
        await topOf(page.locator(FOCUS)),
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
