import { expect, type Locator, type Page, test } from "@playwright/test";
import { mockApi } from "./seed";

// jsdom lays nothing out, so only a browser shows the page pane holding the
// reader's offset across a change of tab.

const ACCOUNT = "/#/companies/o-brandt";

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
function stripBelowVisibleTop(pane: Locator): Promise<number> {
  return pane.evaluate((element) => {
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
  });
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

test.describe("a tab switch on a phone", () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await mockApi(page);
    await page.goto(ACCOUNT);
    await expect(
      page.locator('.recordtabs-tab[data-tab="contacts"]'),
    ).toBeVisible();
  });

  test("keeps the reader's place on the record", async ({ page }) => {
    const pane = page.locator(".main > .scroll");
    const contacts = page.locator('.recordtabs-tab[data-tab="contacts"]');
    const offset = await placeStrip(pane, 120);
    expect(offset, "the record is too short to scroll").toBeGreaterThan(0);

    await pressInPlace(page, contacts);
    await expect(page).toHaveURL(/#\/companies\/o-brandt\/contacts$/);
    await expect(contacts).toHaveAttribute("aria-pressed", "true");

    await expect
      .poll(() => pane.evaluate((element) => Math.round(element.scrollTop)), {
        message: "the pane did not hold the reader's offset across the switch",
      })
      .toBe(offset);
  });

  test("raises the pane just enough to show a strip left above the screen", async ({
    page,
  }) => {
    const pane = page.locator(".main > .scroll");
    const contacts = page.locator('.recordtabs-tab[data-tab="contacts"]');
    const offset = await placeStrip(pane, -300);
    expect(await stripBelowVisibleTop(pane)).toBeLessThan(-250);

    // A keyboard reader can press a tab the screen no longer shows.
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
