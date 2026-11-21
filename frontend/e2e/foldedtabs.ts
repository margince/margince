import type { Locator, Page } from "@playwright/test";
import { de } from "../src/i18n/de";

/**
 * The open "More" panel holding the tabs the row is too narrow for, or null
 * when every tab fits. Its entries are as much the strip's as the drawn tabs.
 */
export async function foldedTabs(page: Page, strip: Locator) {
  const more = strip.getByRole("button", { name: de["record.tabs.moreLabel"] });
  if ((await more.count()) === 0) return null;
  if ((await more.getAttribute("aria-expanded")) !== "true") await more.click();
  return page.locator(`[id="${await more.getAttribute("aria-controls")}"]`);
}

/** The control that opens a section: its tab, or its entry under "More". */
export async function sectionControl(page: Page, strip: Locator, name: string) {
  const tab = strip.getByRole("button", { name, exact: true });
  if (await tab.isVisible()) return tab;
  const folded = await foldedTabs(page, strip);
  return (folded ?? strip).getByRole("button", { name, exact: true });
}
