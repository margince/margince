// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, type Locator, type Page, test } from "@playwright/test";
import { loadLocale } from "../src/i18n/catalogs";
import { de } from "../src/i18n/de";
import { viewMenuName } from "../src/testing/listview.names";
import { expectNoAaViolations, settleAnimations } from "./a11y";
import { copy } from "./copy";
import { mockApi } from "./seed";
import { openedAt, pageOverflow } from "./waits";

test.beforeAll(async () => {
  await loadLocale("de");
});

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

/** Board or Table is a choice in the deals list's View panel. */
async function showDealsAsTable(page: Page) {
  await page.getByRole("button", { name: viewMenuName("de") }).click();
  await page
    .getByRole("dialog", { name: viewMenuName("de") })
    .getByRole("button", { name: de["deals.viewTable"], exact: true })
    .click();
}

test("AC-pipeline-7: board↔table swaps views preserving the deal set", async ({
  page,
}) => {
  await page.goto("/#/deals");
  // Both views draw a deal as a link, so middle-click and open-in-new-tab work.
  // Role therefore cannot tell the views apart. The board side is found by the
  // card's own `data-deal` instead.
  //
  // Neither side is asserted on text alone. Text says nothing about which
  // element it found. Other elements repeat the name too: the table's hidden
  // "<name> auswählen" label and the agent panel's status line.
  //
  // The card's link is matched by substring. A board card's accessible name is
  // the whole card read out: name, company, value, age and badges.
  const boardCard = page.locator('[data-deal="d-fleet"]');
  await expect(boardCard).toBeVisible();
  // The card holds the link rather than being one. It carries two links: the
  // deal and the company behind it. An anchor inside an anchor is invalid, so
  // the deal's link is stretched over the card.
  //
  // The link is asserted on the anchor that carries it, which is what a reader
  // presses for a middle-click or a new tab.
  const boardLink = boardCard.getByRole("link", { name: /Fleet retrofit/ });
  await expect(boardLink).toHaveAttribute("href", /#\/deals\/d-fleet/);
  await expect(boardCard).toContainText("Fleet retrofit");
  await showDealsAsTable(page);
  // The board is gone, so its card locator is the proof the view swapped
  // rather than drew both at once.
  await expect(boardCard).toHaveCount(0);
  await expect(
    page.getByRole("link", { name: "Fleet retrofit" }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Service contract" }),
  ).toBeVisible();
});

/**
 * One stage, in pixels: `--board-col-w` in design-system/composed.css.
 *
 * Written out here rather than read off the sheet, because the number is the
 * acceptance. A test that asked the stylesheet would agree with any width the
 * stylesheet set, including a column that grows into leftover room.
 */
const STAGE_WIDTH_PX = 300;

/**
 * The stage geometry a reader is actually handed, read off the rendered board.
 *
 * `inner` is the width the stages are laid out in, without the board's own
 * padding. The phone proportion below is measured against it. A stage is named
 * by the words in its head. `data-stage` carries only the stage's id, which
 * means nothing to someone reading a failure. `BoardLayout` labels every
 * column it draws.
 */
async function boardGeometry(page: Page) {
  const geometry = await page.evaluate(() => {
    const board = document.querySelector(".board");
    if (board === null) {
      return null;
    }
    const style = getComputedStyle(board);
    return {
      inner:
        board.clientWidth -
        Number.parseFloat(style.paddingLeft) -
        Number.parseFloat(style.paddingRight),
      gap: Number.parseFloat(style.columnGap),
      snap: style.scrollSnapType,
      scrollsSideways: board.scrollWidth > board.clientWidth,
      columns: Array.from(board.querySelectorAll(".board-col")).map(
        (column) => ({
          stage: column.getAttribute("aria-label"),
          folded: column.classList.contains("board-col-collapsed"),
          width: Math.round(column.getBoundingClientRect().width),
          past: column.scrollWidth - column.clientWidth,
          // Read rather than assumed: it decides whether `past` above means
          // anything at all.
          clipping: getComputedStyle(column).overflowX,
        }),
      ),
    };
  });
  // Thrown rather than asserted, so what comes back has no null in it.
  // Otherwise every reader below would need a fallback, and a made-up value
  // would pass on a board that is not there.
  if (geometry === null) {
    throw new Error("this screen drew no .board, so it has no stage geometry");
  }
  if (geometry.columns.length === 0) {
    throw new Error("the board drew no stages, so there is nothing to measure");
  }
  // A `clip` box reports `scrollWidth === clientWidth` however much it cuts
  // off. Outside a list surface the column keeps the `clip` from composed.css.
  // There the spill reading below would call any board clean. Refused here,
  // because a caller cannot tell that zero from a real one.
  const unmeasurable = geometry.columns.filter(
    ({ clipping }) => clipping !== "hidden",
  );
  if (unmeasurable.length > 0) {
    throw new Error(
      `${unmeasurable.map(({ stage, clipping }) => `${stage}: overflow-x ${clipping}`).join(", ")} — a clip box reports no horizontal overflow whatever it cuts, so this helper can only measure a board whose columns are scrollers, which is a board inside a list surface`,
    );
  }
  return geometry;
}

/** The stages that scroll sideways, each naming itself and by how much. */
function stagesSpillingSideways(
  geometry: Awaited<ReturnType<typeof boardGeometry>>,
) {
  return geometry.columns
    .filter(({ past }) => past > 0)
    .map(({ stage, past }) => `${stage}: ${past}px past its column`);
}

// A stage has one width, and the window decides how many stages are on screen.
// A column that grows into leftover room would show four stages to one reader
// and six to another.
//
// A stage also never scrolls sideways itself. Its totals are `nowrap` money.
// A column that scrolls its cards vertically resolves the other axis to `auto`.
// So a figure that does not ellipsise puts a useless scrollbar under every stage.
test("AC-pipeline-8: one stage width at every window, and no stage scrolls sideways", async ({
  page,
}) => {
  await page.goto("/#/deals");
  await expect(page.locator(".board-col").first()).toBeVisible();

  // 768 is inside the fold and 1440 is a wide desktop, and the stage width is
  // the same at both. Each width is kept against its window, so a failure
  // names the window that disagreed.
  const stageWidths: Record<number, number> = {};
  for (const width of [1440, 1280, 1024, 768]) {
    await page.setViewportSize({ width, height: 900 });
    const geometry = await boardGeometry(page);
    expect(stagesSpillingSideways(geometry)).toEqual([]);
    stageWidths[width] = geometry.columns[0].width;
  }
  expect(stageWidths).toEqual({
    1440: STAGE_WIDTH_PX,
    1280: STAGE_WIDTH_PX,
    1024: STAGE_WIDTH_PX,
    768: STAGE_WIDTH_PX,
  });

  // The board itself is what scrolls, and the shell around it never does.
  await page.setViewportSize({ width: 1280, height: 800 });
  const desktop = await boardGeometry(page);
  expect(
    desktop.scrollsSideways,
    "six stages at 240px do not fit 1280px, so the board has to scroll",
  ).toBe(true);
  expect(await pageOverflow(page)).toEqual([]);

  // On a phone the reader swipes. One stage and the gap after it take four
  // fifths of the board, so about a quarter of the next stage shows. That peek
  // is the only sign the pipeline carries on.
  await page.setViewportSize({ width: 390, height: 844 });
  const phone = await boardGeometry(page);
  expect(stagesSpillingSideways(phone)).toEqual([]);
  expect(await pageOverflow(page)).toEqual([]);
  const stage = phone.columns[0].width;
  const stageAndGap = stage + phone.gap;
  expect(Math.abs(stageAndGap - phone.inner * 0.8)).toBeLessThanOrEqual(2);
  const peek = phone.inner - stageAndGap;
  expect(peek / stage).toBeGreaterThan(0.2);
  expect(peek / stage).toBeLessThan(0.35);
  // Snapping is what stops a swipe parking the reader between two stages.
  expect(phone.snap).toBe("x mandatory");
});

// A folded stage hands its width back to the open stages. A fold that kept the
// shared stage width would give them no room. The lead board ships this: its
// two terminal stages are folded until somebody opens one, beside three open
// ones.
test("AC-pipeline-9: a folded stage keeps none of the shared stage width", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto("/#/leads?view=board");
  await expect(page.locator(".board-col-collapsed").first()).toBeVisible();

  const geometry = await boardGeometry(page);
  const folded = geometry.columns.filter((column) => column.folded);
  const open = geometry.columns.filter((column) => !column.folded);
  expect(
    open.length,
    "a fold is only worth anything beside a stage that did not fold",
  ).toBeGreaterThan(0);
  expect(open.filter(({ width }) => width !== STAGE_WIDTH_PX)).toEqual([]);
  expect(folded.filter(({ width }) => width >= STAGE_WIDTH_PX)).toEqual([]);
  expect(stagesSpillingSideways(geometry)).toEqual([]);
  expect(await pageOverflow(page)).toEqual([]);
});

/**
 * Which scroller's bar is lit, named per scroller.
 *
 * Read back as the words `neutral` and `accent` rather than as colours, so a
 * failure does not print raw colour triples. A thumb that is neither is
 * reported as it comes, so a third colour can never be read as one of these.
 *
 * The two are resolved by asking the browser what the sheet's own declaration
 * computes to on this page. A literal `rgb(...)` here would be a second copy of
 * the palette. It would pass on a colour nobody paints any more. It would fail
 * on a correct token change.
 */
async function scrollbarStates(page: Page) {
  return page.evaluate(() => {
    // Off-screen, because a probe under the pointer would take the accent from
    // the very rule being measured and report it as the resting colour.
    function declared(thumb: string) {
      const probe = document.createElement("span");
      probe.style.position = "absolute";
      probe.style.left = "-9999px";
      probe.style.scrollbarColor = `var(${thumb}) transparent`;
      document.body.append(probe);
      const computed = getComputedStyle(probe).scrollbarColor;
      probe.remove();
      return computed;
    }
    const words = new Map([
      [declared("--borderControl"), "neutral"],
      [declared("--accent"), "accent"],
    ]);
    function read(element: Element | null) {
      if (element === null) {
        return "no such scroller on this page";
      }
      const painted = getComputedStyle(element).scrollbarColor;
      return words.get(painted) ?? painted;
    }
    return {
      shell: read(document.querySelector(".scroll")),
      board: read(document.querySelector(".board")),
      stages: Array.from(document.querySelectorAll(".board-col")).map(
        (column) => ({
          stage: column.getAttribute("aria-label"),
          bar: read(column),
        }),
      ),
    };
  });
}

/**
 * The bars, plus what the pointer is standing on, as `tag.class`.
 *
 * The rule under test picks one element, so the test must know which. A grey
 * stage with the pointer a pixel outside differs from a grey stage with the
 * pointer on it. Asserted only where it
 * matters: at rest and on a card, any landing gives "no stage is lit".
 */
async function barsAndPointer(page: Page) {
  const bars = await scrollbarStates(page);
  const pointerOver = await page.evaluate(() => {
    // Document order is ancestor-first along the one hover chain, so the last
    // match is the element lib/pointertarget.ts marks.
    const deepest = Array.from(document.querySelectorAll(":hover")).at(-1);
    if (deepest === undefined) {
      return "nothing: the pointer is outside the document";
    }
    return [deepest.tagName.toLowerCase(), ...deepest.classList].join(".");
  });
  return { ...bars, pointerOver };
}

/**
 * A point that is the first stage's own box and no other element's.
 *
 * Not a corner. A column's padding edge is one pixel's rounding from the board
 * behind it. The top inset belongs to the sticky head, a different element.
 * Inside a list surface a column is stretched to the surface's height. So a
 * half-empty stage owns the run from its last child down to its bottom edge.
 * The middle of that run is far from any edge. A full stage keeps the flex gap
 * between its first two cards, which is the same ground.
 */
async function pointInsideFirstStage(page: Page) {
  const point = await page.evaluate(() => {
    const column = document.querySelector(".board-col");
    if (column === null) {
      return "the board drew no stage, so there is nothing to hover";
    }
    const box = column.getBoundingClientRect();
    const x = box.left + box.width / 2;
    const last = Array.from(column.children).at(-1)?.getBoundingClientRect();
    // In a run thinner than a card's own gap, rounding decides the hit again,
    // as it does at the padding edge.
    const roomToStand = 8;
    if (last !== undefined && box.bottom - last.bottom >= roomToStand) {
      return { x, y: (last.bottom + box.bottom) / 2 };
    }
    const cards = Array.from(column.querySelectorAll(".deal-card"), (card) =>
      card.getBoundingClientRect(),
    );
    if (cards.length >= 2) {
      return { x, y: (cards[0].bottom + cards[1].top) / 2 };
    }
    return "the first stage is filled to its bottom edge by a single card, so it has neither an empty run nor a gap between cards to hover";
  });
  if (typeof point === "string") {
    throw new Error(point);
  }
  return point;
}

// One bar takes the accent, and it is the bar the pointer is on.
//
// Hover is a chain, so a bare `*:hover` would light every scroller between the
// cursor and the root. app.css lights only the element lib/pointertarget.ts
// marks: the one whose own box (bar, padding, gutter) is under the pointer.
// The board nests a scroller inside a scroller inside a scroller.
test("AC-pipeline-10: the bar under the pointer is the one that lights", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto("/#/deals");
  const stages = page.locator(".board-col");
  await expect(stages.first()).toBeVisible();
  expect(
    await stages.count(),
    "this is about ONE scroller among several, so it takes at least two stages to be about anything",
  ).toBeGreaterThan(1);

  // At rest: the pointer in the corner of the window, on the chrome beside the
  // board rather than anywhere in it.
  await page.mouse.move(0, 0);
  const resting = await scrollbarStates(page);
  const everyStageNeutral = resting.stages.map(({ stage }) => ({
    stage,
    bar: "neutral",
  }));
  expect(resting).toEqual({
    shell: "neutral",
    board: "neutral",
    stages: everyStageNeutral,
  });

  // On the first stage, on ground that is the column's own box only. The seed
  // has too few cards for a stage to overflow, so there is no vertical bar to
  // aim at. The empty run below the last card is where that bar would be.
  const probe = await pointInsideFirstStage(page);
  await page.mouse.move(probe.x, probe.y);
  await expect
    .poll(() => barsAndPointer(page))
    .toEqual({
      shell: "neutral",
      board: "neutral",
      pointerOver: "section.board-col",
      stages: resting.stages.map(({ stage }, index) => ({
        stage,
        bar: index === 0 ? "accent" : "neutral",
      })),
    });

  // On a card, the deepest element is inside the stage rather than the stage.
  // No scroller is then under the pointer, so every bar is grey again.
  const card = await page.locator(".deal-card").first().boundingBox();
  if (card === null) {
    throw new Error("the board drew no deal card to hover");
  }
  await page.mouse.move(card.x + card.width / 2, card.y + card.height / 2);
  await expect
    .poll(() => scrollbarStates(page))
    .toEqual({
      shell: "neutral",
      board: "neutral",
      stages: everyStageNeutral,
    });
});

// Both views name a deal's owner. The card's mark names the owner on hover.
// The table gives the owner a column of its own.
test("AC-pipeline-11: both views name a deal's owner", async ({ page }) => {
  await page.goto("/#/deals");
  const mark = page
    .locator('[data-deal="d-fleet"]')
    .getByRole("img", { name: "Lena Fischer" });
  // Playwright refuses to hover a covered element. So this also proves the
  // mark sits above the deal's link, which is stretched over the card.
  await mark.hover();
  await expect(page.getByRole("tooltip")).toHaveText("Lena Fischer");
  // Raised above the link, the mark still opens the deal like the rest of the
  // card. The middle button opens a new tab, and a click opens it in place.
  const tab = page.context().waitForEvent("page");
  await mark.click({ button: "middle" });
  await (await openedAt(tab, /#\/deals\/d-fleet$/)).close();
  await mark.click();
  await expect(page).toHaveURL(/#\/deals\/d-fleet$/);
  await page.goto("/#/deals");
  await showDealsAsTable(page);
  const row = page.getByRole("row", { name: /Fleet retrofit/ });
  await expect(row.getByRole("cell", { name: "Lena Fischer" })).toBeVisible();
});

// A stage busier than one column's page reads its next page from its own Load
// more, and no other column reads or offers one.
test("AC-pipeline-12: a busy stage's column loads its next page on its own", async ({
  page,
}) => {
  test.skip(Boolean(process.env.BASE_URL), "it reads the busy-stage mock");
  await mockApi(page, { busyStage: true });
  await page.goto("/#/deals");
  const qualify = page.locator('[data-stage="s1"]');
  // The head states the report's count over every deal, not the page's cards.
  await expect(qualify.locator(".board-col-count .optioncount")).toHaveText(
    "31",
  );
  // The seeded Qualify deal and thirty more: 31, of which a page reads 25.
  const cards = qualify.locator("[data-deal]");
  await expect(cards).toHaveCount(25);
  const loadMore = page.getByRole("button", { name: "Mehr laden" });
  await expect(loadMore).toHaveCount(1);

  await qualify.getByRole("button", { name: "Mehr laden" }).click();

  await expect(cards).toHaveCount(31);
  await expect(loadMore).toHaveCount(0);
});

// The note a won close leaves. A shown probability follows it after a "·".
const closedWonNote = new RegExp(`^${de["deals.closedWon"]}`);

async function centreOf(part: Locator) {
  const box = await part.boundingBox();
  if (!box) throw new Error("a board part to drag between is not laid out");
  return { x: box.x + box.width / 2, y: box.y + box.height / 2 };
}

async function centreBetween(first: Locator, second: Locator) {
  const between = ((await centreOf(first)).x + (await centreOf(second)).x) / 2;
  await first.evaluate((part, at) => {
    const board = part.closest(".board");
    if (!board) return;
    const box = board.getBoundingClientRect();
    board.scrollLeft += at - (box.left + box.width / 2);
  }, between);
}

async function outsideBoard(...parts: Locator[]): Promise<string[]> {
  const outside: string[] = [];
  for (const part of parts) {
    const off = await part.evaluate((element) => {
      const board = element.closest(".board")?.getBoundingClientRect();
      const box = element.getBoundingClientRect();
      const x = box.left + box.width / 2;
      const inside = board !== undefined && x > board.left && x < board.right;
      return inside ? null : element.outerHTML.slice(0, 60);
    });
    if (off !== null) outside.push(off);
  }
  return outside;
}

// Not dragTo: it may scroll the board after the press, and Chromium then starts no drag.
async function dragOnto(page: Page, card: Locator, column: Locator) {
  const from = await centreOf(card);
  const to = await centreOf(column);
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(to.x, to.y, { steps: 8 });
  await expect(column).toHaveClass(/\bdroptarget\b/);
  await page.mouse.up();
}

test("AC-deal-6: a pointer drag onto Won opens the 🟡 confirm, and nothing is sent before Confirm", async ({
  page,
}) => {
  const writes: string[] = [];
  page.on("request", (request) => {
    const { pathname } = new URL(request.url());
    if (
      request.method() !== "GET" &&
      /^\/v1\/deals\/d-fleet(\/|$)/.test(pathname)
    ) {
      writes.push(`${request.method()} ${pathname}`);
    }
  });
  await page.goto("/#/deals");
  // Waited for by the locator the drag actually uses, so the wait and the
  // action cannot disagree about which element the test is about.
  const card = page.locator('[data-deal="d-fleet"]');
  await expect(card).toBeVisible();
  const won = page.locator('[data-stage="s4"]');
  await settleAnimations(page);
  // A rep scrolls the board until both ends of the drag show.
  await centreBetween(card, won);
  expect(
    await outsideBoard(card, won),
    "the card and the Won column must both be on screen for a pointer drag",
  ).toEqual([]);
  await dragOnto(page, card, won);

  await expect(page.getByText("In die Phase Won verschieben?")).toBeVisible();
  await expect(page.getByText(closedWonNote)).toHaveCount(0);
  expect(writes).toEqual([]);
});

/** A board card's "…" trigger, reached from the card's own link with Tab. */
async function tabToCardMenu(page: Page, dealId: string, name: string) {
  const card = page.locator(`[data-deal="${dealId}"]`);
  const trigger = card.getByRole("button", {
    name: copy(de["record.moreActionsFor"], { subject: name }),
  });
  await card.locator(".deal-open").focus();
  for (let presses = 0; presses < 12; presses++) {
    if (await trigger.evaluate((el) => el === document.activeElement)) {
      return trigger;
    }
    await page.keyboard.press("Tab");
  }
  throw new Error(`Tab never reached the "…" menu of ${name}`);
}

test("AC-deal-7: a card's menu moves a deal to the next stage with the keyboard alone", async ({
  page,
}) => {
  await page.goto("/#/deals");
  await expect(page.locator('[data-deal="d-fleet"]')).toBeVisible();
  const trigger = await tabToCardMenu(page, "d-fleet", "Fleet retrofit");
  await page.keyboard.press("Enter");
  await expect(trigger).toHaveAttribute("aria-expanded", "true");
  await page
    .getByRole("button", {
      name: de["deal.card.moveTo"].replace("{stage}", "Negotiation"),
    })
    .press("Enter");

  await expect(page.getByText("Nach Negotiation verschoben")).toBeVisible();
  const moved = page.locator('[data-stage="s3"] [data-deal="d-fleet"]');
  await expect(moved).toBeVisible();
  await expect(
    page.locator('[data-stage="s2"] [data-deal="d-fleet"]'),
  ).toHaveCount(0);
  await expect(moved.locator(".deal-open")).toBeFocused();
});

test("AC-deal-7: a card's menu asks the terminal confirm for Mark won, and Cancel moves nothing", async ({
  page,
}) => {
  await page.goto("/#/deals");
  const trigger = await tabToCardMenu(page, "d-fleet", "Fleet retrofit");
  await page.keyboard.press("Enter");
  await page
    .getByRole("button", { name: de["deal.card.markWon"], exact: true })
    .click();
  await expect(page.getByText("In die Phase Won verschieben?")).toBeVisible();
  await expect(page.getByText("Wie wurde der Deal gewonnen?")).toBeVisible();
  await expect(page.getByText(closedWonNote)).toHaveCount(0);

  await page.getByRole("button", { name: "Abbrechen" }).click();
  await expect(page.getByText("In die Phase Won verschieben?")).toHaveCount(0);
  await expect(trigger).toHaveAttribute("aria-expanded", "false");
  await expect(
    page.locator('[data-stage="s2"] [data-deal="d-fleet"]'),
  ).toBeVisible();
  await expect(
    page.locator('[data-stage="s4"] [data-deal="d-fleet"]'),
  ).toHaveCount(0);
});

test("AC-deal-7: a card's menu picks any stage by searching the stage picker", async ({
  page,
}) => {
  await page.goto("/#/deals");
  const trigger = await tabToCardMenu(page, "d-fleet", "Fleet retrofit");
  await page.keyboard.press("Enter");
  await page
    .getByRole("button", { name: de["deals.bulkStage"], exact: true })
    .click();
  await page
    .getByRole("combobox", { name: de["deals.bulkStagePick"] })
    .fill("Neg");
  await page.getByRole("option", { name: "Negotiation" }).click();

  await expect(page.getByText("Nach Negotiation verschoben")).toBeVisible();
  await expect(
    page.locator('[data-stage="s3"] [data-deal="d-fleet"]'),
  ).toBeVisible();
  await expect(
    page.locator('[data-stage="s2"] [data-deal="d-fleet"]'),
  ).toHaveCount(0);
  await expect(trigger).toHaveAttribute("aria-expanded", "false");
});

test("AC-deal-7: Escape closes a card's menu and returns focus to its trigger", async ({
  page,
}) => {
  await page.goto("/#/deals");
  const trigger = await tabToCardMenu(page, "d-fleet", "Fleet retrofit");
  await page.keyboard.press("Enter");
  await expect(trigger).toHaveAttribute("aria-expanded", "true");
  await page.keyboard.press("Escape");
  await expect(trigger).toHaveAttribute("aria-expanded", "false");
  await expect(trigger).toBeFocused();
});

test.describe("AC-deal-7: a card's menu, in light mode", () => {
  test.use({ colorScheme: "light" });

  test("meets WCAG 2.2 AA with the menu open", async ({ page }) => {
    await page.goto("/#/deals");
    const trigger = await tabToCardMenu(page, "d-fleet", "Fleet retrofit");
    await page.keyboard.press("Enter");
    await expect(trigger).toHaveAttribute("aria-expanded", "true");
    await settleAnimations(page);
    await expectNoAaViolations(page, "deals, card menu open");
  });
});

test.describe("AC-deal-7: a card's menu at 390px", () => {
  test.use({ viewport: { width: 390, height: 844 }, hasTouch: true });

  test("opens by tap and moves the deal to the next stage", async ({
    page,
  }) => {
    await page.goto("/#/deals");
    const card = page.locator('[data-deal="d-fleet"]');
    await expect(card).toBeAttached();
    await card
      .getByRole("button", {
        name: copy(de["record.moreActionsFor"], { subject: "Fleet retrofit" }),
      })
      .tap();
    await page
      .getByRole("button", {
        name: de["deal.card.moveTo"].replace("{stage}", "Negotiation"),
      })
      .tap();
    await expect(page.getByText("Nach Negotiation verschoben")).toBeVisible();
    await expect(
      page.locator('[data-stage="s3"] [data-deal="d-fleet"]'),
    ).toBeAttached();
  });
});
