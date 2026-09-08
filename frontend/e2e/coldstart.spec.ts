import { expect, type Page, test } from "@playwright/test";
import { mockApi } from "./seed";

/**
 * The cold start's way onward is on the screen, whatever the question costs in
 * height.
 *
 * Setup asks its questions in one room (`design-system/onboarding-stage`) whose
 * bottom rail carries the step's Continue — deliberately there rather than at
 * the end of the board, because a board can run long and a verb at the end of a
 * long board is a verb the reader has scrolled away from. The rail said so and
 * did not do it: it was `position: sticky`, and the room is `overflow: hidden`,
 * which makes the ROOM the scrollport a sticky child resolves against — a box
 * that never scrolls, so the rail never stuck. What scrolled instead was the
 * page, and Continue sat at the bottom of a card several screens tall.
 *
 * Nothing in the unit suite could see it: jsdom lays nothing out, so the defect
 * and the fix produce identical DOM. It takes a browser and a window with a
 * height, which is what this file is.
 *
 * German chrome, as the app renders it.
 */

// Short on purpose. The claim is about a board that does not fit, so a window
// tall enough to hold one would pass on either side of the fix and prove
// nothing — which is why every case below also asserts that the board really
// did overflow before it looks at the button.
const SHORT = { width: 1280, height: 520 };
const PHONE = { width: 390, height: 560 };

type Room = Readonly<{
  /** The question's own height against the window's — true on both sides of
   * the fix, which is what makes it usable as the guard that this window is
   * really too short for this board. */
  questionOutgrowsWindow: boolean;
  /** Whether the board is the box giving ground, which is the whole mechanism. */
  boardScrolls: boolean;
  pageScroll: number;
  documentScroll: number;
}>;

async function roomOf(page: Page): Promise<Room> {
  return page.evaluate(() => {
    const board = document.querySelector(".ob-stage-board");
    const scroller = document.querySelector(".scroll");
    const root = document.documentElement;
    const content = board?.scrollHeight ?? 0;
    return {
      questionOutgrowsWindow: content > globalThis.innerHeight,
      boardScrolls: board !== null && content > board.clientHeight + 1,
      pageScroll:
        scroller === null ? 0 : scroller.scrollHeight - scroller.clientHeight,
      documentScroll: root.scrollHeight - root.clientHeight,
    };
  });
}

async function openTheModelQuestion(page: Page): Promise<void> {
  await mockApi(page, { journey: "unconfigured" });
  await page.goto("/#/onboarding");
  // The step whose form is the longest thing the cold start puts on a board:
  // an installation that has bound no model is asked for one before it is
  // asked for a website, because a read it cannot perform is a question it
  // should not put.
  await expect(page.getByText("Hier kann noch nichts denken")).toBeVisible();
}

for (const [where, viewport] of Object.entries({
  desktop: SHORT,
  phone: PHONE,
})) {
  test(`the cold start's Continue is in the window on a board that does not fit (${where})`, async ({
    page,
  }) => {
    await page.setViewportSize(viewport);
    await openTheModelQuestion(page);

    // FIRST, and before anything is concluded from the button: a window this
    // board fits into would pass whether or not the rail works, and a guard
    // measured on the box that changed (the board's own scrollability) reports
    // the defect as "nothing overflowed" instead of "the verb is off-screen".
    // The question's height against the window's is the one comparison that
    // holds on both sides of the fix.
    const room = await roomOf(page);
    expect(room.questionOutgrowsWindow).toBe(true);

    const onward = page.getByRole("button", { name: "Weiter" });
    await expect(onward).toBeInViewport({ ratio: 1 });

    // And by the intended mechanism rather than by luck: the board is what
    // gives ground, and the page underneath has grown no scroll of its own for
    // the rail to ride down with.
    expect(room.boardScrolls).toBe(true);
    expect(room.pageScroll).toBe(0);
    expect(room.documentScroll).toBe(0);

    // And it stays there from the far end of the board, which is where the
    // reader who has answered the last field actually presses it.
    await page.evaluate(() => {
      const board = document.querySelector(".ob-stage-board");
      board?.scrollTo(0, board.scrollHeight);
    });
    await expect(onward).toBeInViewport({ ratio: 1 });
  });
}
