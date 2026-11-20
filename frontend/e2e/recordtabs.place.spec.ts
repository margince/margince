import { expect, type Locator, type Page, test } from "@playwright/test";
import { de } from "../src/i18n/de";
import { mockApi } from "./seed";

// jsdom lays nothing out, so only a browser shows the page pane holding the
// reader's offset across a change of tab.

const ACCOUNT = "/#/companies/o-brandt";
const ABOVE = 66;

// A finger on the tab where it stands. Not locator.click(): its own scroll into
// view moves the pane before the press, which is the offset under test.
async function pressInPlace(page: Page, tab: Locator) {
  const box = await tab.boundingBox();
  if (!box) throw new Error("the tab has no box");
  const x = box.x + box.width / 2;
  const y = box.y + box.height / 2;
  const onTop = await page.evaluate(
    ([px, py, name]) =>
      document.elementFromPoint(px, py)?.closest(`[data-tab="${name}"]`) !==
      null,
    [x, y, await tab.getAttribute("data-tab")] as const,
  );
  expect(onTop, "the tab is covered or off screen at the reader's offset").toBe(
    true,
  );
  await page.mouse.click(x, y);
}

// The strip's top against the pane's visible top, which runs under the bar.
function stripBelowVisibleTop(
  pane: Locator,
  sample: "current" | "before-press" | "at-press" = "current",
): Promise<number> {
  return pane.evaluate((element, sample) => {
    const read = () => {
      const row = element.querySelector(".recordtabs-row");
      if (!row) throw new Error("the record drew no tab strip");
      const underBar = Math.max(
        0,
        -Number.parseFloat(getComputedStyle(element).marginTop) || 0,
      );
      return (
        row.getBoundingClientRect().top -
        (element.getBoundingClientRect().top + underBar)
      );
    };
    if (sample === "before-press") {
      element.addEventListener(
        "click",
        () => element.setAttribute("data-strip-at-press", String(read())),
        { capture: true, once: true },
      );
    }
    if (sample === "at-press") {
      const atPress = element.getAttribute("data-strip-at-press");
      if (atPress === null) throw new Error("the tab was not pressed");
      return Number(atPress);
    }
    return read();
  }, sample);
}

// Puts the strip `below` px under the pane's visible top, once the body has
// stopped growing. A negative `below` leaves it above the screen.
async function placeStrip(pane: Locator, below: number): Promise<number> {
  let last = Number.NaN;
  await expect
    .poll(async () => {
      const height = await pane.evaluate((element) => element.scrollHeight);
      const settled = height === last;
      last = height;
      return settled;
    })
    .toBe(true);
  return pane.evaluate((element, gap) => {
    const row = element.querySelector<HTMLElement>(".recordtabs-row");
    if (!row) throw new Error("the record drew no tab strip");
    const underBar = Math.max(
      0,
      -Number.parseFloat(getComputedStyle(element).marginTop) || 0,
    );
    const strip =
      row.getBoundingClientRect().top -
      element.getBoundingClientRect().top +
      element.scrollTop;
    element.scrollTop = strip - underBar - gap;
    return Math.round(element.scrollTop);
  }, below);
}

// What the next tab draws, once the press has read the strip. It adds `above`
// px over the strip and a body long enough for any offset under test.
function onNextPress(page: Page, { above }: Readonly<{ above: number }>) {
  return page.evaluate((height) => {
    const block = (px: number) => {
      const element = document.createElement("div");
      element.style.blockSize = `${px}px`;
      element.style.flex = "none";
      return element;
    };
    document.addEventListener(
      "click",
      () => {
        const pane = document.querySelector(".main > .scroll");
        pane?.prepend(block(height));
        pane?.append(block(4000));
      },
      { once: true },
    );
  }, above);
}

test.describe("a tab switch on a phone", () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await mockApi(page);
    await page.goto(ACCOUNT);
    await expect(
      page.locator('.recordtabs-tab[data-tab="contacts"]'),
    ).toBeVisible();
    // The Overview's body has loaded: its skeleton holds a fixed height, so an
    // unchanged page height alone cannot tell a placeholder from the body.
    const standing = de["standing.title"].split(" ·")[0];
    await expect(
      page.getByRole("heading", { name: new RegExp(`^${standing}`) }),
    ).toBeVisible();
    await expect(page.locator(".main > .scroll .skeleton")).toHaveCount(0);
  });

  test("keeps the reader's place on the record", async ({ page }) => {
    const pane = page.locator(".main > .scroll");
    const contacts = page.locator('.recordtabs-tab[data-tab="contacts"]');
    const offset = await placeStrip(pane, 120);
    expect(offset, "the record is too short to scroll").toBeGreaterThan(0);

    // Capture the same geometry as the tab handler, after any layout motion.
    await stripBelowVisibleTop(pane, "before-press");

    // The next tab draws a line above the strip, as a record's newest message
    // does off its overview.
    await onNextPress(page, { above: ABOVE });
    await pressInPlace(page, contacts);
    await expect(page).toHaveURL(/#\/companies\/o-brandt\/contacts$/);
    await expect(contacts).toHaveAttribute("aria-pressed", "true");

    // Within a pixel: an offset is fractional on this display.
    await expect
      .poll(
        async () =>
          Math.abs(
            (await pane.evaluate((element) => element.scrollTop)) -
              (offset + ABOVE),
          ),
        { message: "the pane did not keep the strip where the reader left it" },
      )
      .toBeLessThanOrEqual(1);
    const below = await stripBelowVisibleTop(pane, "at-press");
    expect(await stripBelowVisibleTop(pane)).toBeCloseTo(below, 0);
  });

  test("raises the pane just enough to show a strip left above the screen", async ({
    page,
  }) => {
    const pane = page.locator(".main > .scroll");
    const contacts = page.locator('.recordtabs-tab[data-tab="contacts"]');
    const offset = await placeStrip(pane, -300);
    expect(await stripBelowVisibleTop(pane)).toBeLessThan(-250);

    // A keyboard reader can press a tab the screen no longer shows.
    await onNextPress(page, { above: 0 });
    await contacts.evaluate((tab) => tab.focus({ preventScroll: true }));
    await page.keyboard.press("Enter");
    await expect(contacts).toHaveAttribute("aria-pressed", "true");

    await expect
      .poll(async () => Math.round(await stripBelowVisibleTop(pane)), {
        message: "the strip is not at the top of the visible pane",
      })
      .toBe(0);
    expect(
      await pane.evaluate((element) => Math.round(element.scrollTop)),
    ).toBeLessThan(offset);
  });
});
