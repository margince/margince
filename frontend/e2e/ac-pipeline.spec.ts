// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, type Page, test } from "@playwright/test";
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
  // Both views draw a deal as a LINK now — a card that opens a record is an
  // anchor, so middle-click and open-in-new-tab work on it — which is why the
  // board side is identified by the card's own `data-deal` and not by its role.
  // Role used to tell the two views apart, and it cannot any more; asserting it
  // alone would pass on either view and stop testing the swap at all.
  //
  // Neither side is asserted on text alone: text says nothing about which
  // element it found, and it goes ambiguous the moment anything else on the page
  // legitimately repeats the name — the table's visually-hidden "<name>
  // auswählen" bulk-select label does, and so does the agent panel's spoken
  // status line ("Reading the <name> deal").
  //
  // The card is matched by substring on purpose: a board card's accessible name
  // is the whole card read out — name, company, value, age, badges — so the
  // deal's name is a fragment of it by construction, and the assertion is "a
  // card for this deal is on the board", not "this text is the card".
  const boardCard = page.locator('[data-deal="d-fleet"]');
  await expect(boardCard).toBeVisible();
  // The card HOLDS the link rather than being one. It carries two
  // destinations now — the deal and the company behind it — and an anchor
  // inside an anchor is invalid markup the browser unnests, so the deal's link
  // is stretched over the card instead of wrapped around it.
  //
  // The link is still asserted, because it is the behaviour worth having: a
  // middle-click and an open-in-new-tab on a deal card. It is asserted on the
  // anchor that carries it, which is what a reader actually presses.
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
 * One stage, in pixels — `--board-col-w` in design-system/composed.css.
 *
 * Written out here rather than read off the sheet, and that is the point: the
 * NUMBER is the acceptance. A test that asked the stylesheet how wide a stage
 * should be would agree with whatever the stylesheet said, including the
 * grow-into-the-leftover-room sizing this replaced, and would have passed on
 * the defect it exists to catch.
 */
const STAGE_WIDTH_PX = 300;

/**
 * The stage geometry a reader is actually handed, read off the rendered board.
 *
 * `inner` is the width the stages are laid out in — the board's own padding is
 * not board a reader can see — and it is what the phone proportion below is
 * measured against. A stage is named by the words in its HEAD: `data-stage`
 * carries the stage's id, which names it to the database and to nobody reading
 * a failure. `BoardLayout` labels every column it draws, so there is no
 * unnamed stage to stand in for.
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
  // Thrown rather than asserted, so what comes back has no null in it: every
  // reader below would otherwise be optional, and a width divided by a divisor
  // invented on the spot is an assertion that passes on a board that is not
  // there.
  if (geometry === null) {
    throw new Error("this screen drew no .board, so it has no stage geometry");
  }
  if (geometry.columns.length === 0) {
    throw new Error("the board drew no stages, so there is nothing to measure");
  }
  // A `clip` box reports `scrollWidth === clientWidth` however much it is
  // cutting off, so on a board outside a list surface — where the column keeps
  // the `clip` composed.css declares — the spill reading below is not a smaller
  // answer but NO answer, and it would call any board clean however far its
  // contents ran past the stage. Refused here rather than returned, because a
  // caller cannot tell the two zeroes apart.
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

// A stage is ONE width, and the window decides how many of them are on screen
// rather than how wide one is. A column that grows into leftover room shows the
// same pipeline as four stages to one reader and six to another, and neither
// can say how many there are or compare a stage against the same stage on
// somebody else's screen.
//
// The other half is that a stage never scrolls sideways ITSELF. Its totals are
// `nowrap` money, and a column that scrolls its cards vertically resolves the
// other axis to `auto` alongside it — so a figure that does not ellipsise puts
// a scrollbar under every stage on the board, which reveals nothing a reader
// can use and costs each of them a line.
test("AC-pipeline-8: one stage width at every window, and no stage scrolls sideways", async ({
  page,
}) => {
  await page.goto("/#/deals");
  await expect(page.locator(".board-col").first()).toBeVisible();

  // 768 is inside the fold and 1440 is a wide desktop: the point is that the
  // stage does not care. Each width is kept against the window that produced
  // it, so a failure names the window that disagreed instead of printing four
  // bare numbers and leaving the reader to count along.
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

  // On a phone the reader swipes: one stage and the gap after it take four
  // fifths of the board, which leaves about a quarter of the next stage on
  // screen. That peek is the only thing saying the pipeline carries on.
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

// A FOLDED stage hands its width back, and the shared stage width is exactly
// what it has to escape — a fold that still drew 240px would have bought the
// stages a reader works out of nothing at all, which is the whole reason to
// fold one. The lead board is where this ships: its two terminal stages are
// folded until somebody opens one, beside three open ones.
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
 * Which scroller's bar is LIT, named per scroller.
 *
 * Read back as the words `neutral` and `accent` rather than as colours: what
 * has to be checked is "the board is grey and the stage under the pointer is
 * not", and a failure printing two triples of numbers makes the reader do the
 * palette arithmetic themselves. A thumb that is neither is reported as it
 * comes, so a third colour can never be read as one of these two.
 *
 * The two are resolved by asking the browser what the sheet's own declaration
 * computes to on this page. A literal `rgb(...)` here would be a second copy of
 * the palette: it would keep passing on a colour nobody paints any more, and
 * fail on a token change that was correct.
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
 * The bars, plus WHAT the pointer is standing on, as `tag.class`.
 *
 * The rule under test picks ONE element, so which element it picked is the
 * claim's premise: a stage that stayed grey because the pointer landed a pixel
 * outside it is a different defect from one that stayed grey with the pointer
 * squarely on it, and two lists of colours cannot tell those apart. Asserted
 * only where the premise is the point — at rest and on a card the claim is
 * "no stage is lit", which every landing inside the board satisfies equally.
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
 * A point that is the first stage's OWN box and no other element's.
 *
 * Not the corner a reader might reach for. A column's padding edge is one
 * pixel's rounding from the board behind it — CI hovered `first.x + 4,
 * first.y + 4` and got the board — and the top inset belongs to the sticky
 * head, which is a different element again. The empty run below the last thing
 * in the column has neither problem: inside a list surface a column is
 * stretched to the surface's height and scrolls its own cards, so a stage the
 * seed leaves half-empty is its own ground from its last child down to its
 * bottom edge, and the middle of that run is nowhere near an edge of anything.
 * A stage full enough to have no such run keeps the flex gap between its first
 * two cards, which is the same ground.
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
    // A run thinner than a card's own gap is back to being decided by
    // rounding, which is the whole reason the padding edge was given up.
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

// ONE bar takes the accent, and it is the bar the pointer is on.
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

  // AT REST: the pointer in the corner of the window, on the chrome beside the
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

  // ON THE FIRST STAGE, on ground that is the column's own box and nothing
  // else's. The seeded board holds too few cards for a stage to overflow, so
  // there is no vertical bar to aim at — the empty run below the last card is
  // the strip that bar would be drawn in, and either way it is the deepest
  // element at that point.
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

  // ON A CARD: the deepest element is inside the stage rather than the stage,
  // so no scroller on the page is the one under the pointer and every bar is
  // grey again.
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

// Whose deal it is reads the same in both views: the card's mark names the
// owner on hover, and the table gives the owner a column of its own.
test("AC-pipeline-11: both views name a deal's owner", async ({ page }) => {
  await page.goto("/#/deals");
  const mark = page
    .locator('[data-deal="d-fleet"]')
    .getByRole("img", { name: "Lena Fischer" });
  // Playwright refuses to hover an element another one covers, so this also
  // holds the mark above the deal's link, which is stretched over the card.
  await mark.hover();
  await expect(page.getByRole("tooltip")).toHaveText("Lena Fischer");
  // Raised above the link, the mark still opens the deal like the rest of the
  // card: the middle button in a new tab, a click in place.
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

test("AC-deal-6: a terminal-stage drop is a 🟡 confirm — nothing runs before Confirm", async ({
  page,
}) => {
  await page.goto("/#/deals");
  // Waited for by the locator the drag actually uses, so the wait and the
  // action cannot disagree about which element the test is about.
  const card = page.locator('[data-deal="d-fleet"]');
  await expect(card).toBeVisible();
  const won = page.locator('[data-stage="s4"]');
  // The board scrolls sideways and the terminal stage starts at this window's
  // edge, so it is brought into view the way a rep scrolls to it before a drop.
  await won.scrollIntoViewIfNeeded();
  await card.dragTo(won);
  await expect(page.getByText("In die Phase Won verschieben?")).toBeVisible();

  // This deal has no signed contract, and a win without one has to say how it
  // was won. So the dialog asks before Confirm and not after a refusal. A deal
  // with its contract is never asked.
  await expect(page.getByText("Wie wurde der Deal gewonnen?")).toBeVisible();
  // Still nothing has run: the drop is not applied while the dialog is open.
  await expect(page.getByText(closedWonNote)).toHaveCount(0);

  await page
    .getByRole("combobox", { name: "Wie wurde der Deal gewonnen?" })
    .click();
  await page.getByRole("option", { name: "Per Bestellung" }).click();
  await page.getByRole("button", { name: "Bestätigen" }).click();
  await expect(page.getByText(closedWonNote)).toBeVisible();
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
