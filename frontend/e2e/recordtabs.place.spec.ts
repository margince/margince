import { expect, type Locator, type Page, test } from "@playwright/test";
import { mockApi } from "./seed";

// jsdom lays nothing out, so only a browser shows the page pane holding the
// reader's offset across a change of tab.

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

test("a tab switch on a phone keeps the reader's place on the record", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await mockApi(page);
  await page.goto("/#/contacts/p-anna/timeline");
  const overview = page.locator('.recordtabs-tab[data-tab="overview"]');
  await expect(overview).toBeVisible();

  // The foot of the shorter body, once it stops growing. The strip is still on
  // screen there, and the longer body has room to hold the offset.
  const pane = page.locator(".main > .scroll");
  const bottom = () =>
    pane.evaluate((element) => element.scrollHeight - element.clientHeight);
  let last = -1;
  await expect
    .poll(
      async () => {
        const now = await bottom();
        const settled = now === last && now > 100;
        last = now;
        return settled;
      },
      { message: "the record is too short to scroll" },
    )
    .toBe(true);
  const offset = await pane.evaluate((element) => {
    element.scrollTop = element.scrollHeight;
    return Math.round(element.scrollTop);
  });

  await pressInPlace(page, overview);
  await expect(page).toHaveURL(/#\/contacts\/p-anna\/overview$/);
  await expect(overview).toHaveAttribute("aria-pressed", "true");

  await expect
    .poll(() => pane.evaluate((element) => Math.round(element.scrollTop)), {
      message: "the pane did not hold the reader's offset across the switch",
    })
    .toBe(offset);
});
