import { expect, test } from "@playwright/test";
import { mockApi } from "./seed";

// jsdom lays nothing out, so only a browser shows the page pane holding the
// reader's offset across a change of tab.
test("a tab switch on a phone keeps the reader's place on the record", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await mockApi(page);
  await page.goto("/#/contacts/p-anna/timeline");
  const overview = page.locator('.recordtabs-tab[data-tab="overview"]');
  await expect(overview).toBeVisible();

  // The foot of the shorter body, where the strip is still on screen to press
  // and the longer body has room to hold the offset.
  const pane = page.locator(".main > .scroll");
  const bottom = () =>
    pane.evaluate((element) => element.scrollHeight - element.clientHeight);
  await expect
    .poll(bottom, { message: "the record is too short to scroll" })
    .toBeGreaterThan(100);
  const offset = await pane.evaluate((element) => {
    element.scrollTop = element.scrollHeight;
    return Math.round(element.scrollTop);
  });

  await overview.click();
  await expect(page).toHaveURL(/#\/contacts\/p-anna\/overview$/);
  await expect(overview).toHaveAttribute("aria-pressed", "true");

  await expect
    .poll(() => pane.evaluate((element) => Math.round(element.scrollTop)), {
      message: "the pane did not hold the reader's offset across the switch",
    })
    .toBe(offset);
});
