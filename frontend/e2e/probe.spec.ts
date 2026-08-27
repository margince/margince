import { expect, test } from "@playwright/test";

// A throwaway probe: why does the record's own h1 report hidden?
test("probe the record h1", async ({ page }) => {
  await page.goto("/#/companies/o-brandt");
  await page.waitForLoadState("networkidle");
  const info = await page.evaluate(() => {
    const h1 = document.querySelector("h1");
    if (!h1) {
      return { found: false, chain: [] as string[] };
    }
    const chain: string[] = [];
    let el: HTMLElement | null = h1 as HTMLElement;
    for (let i = 0; el && i < 7; i += 1) {
      const cs = getComputedStyle(el);
      const r = el.getBoundingClientRect();
      chain.push(
        `${el.tagName}.${el.className || "-"} vis=${cs.visibility} disp=${cs.display} op=${cs.opacity} w=${Math.round(r.width)} h=${Math.round(r.height)} ov=${cs.overflow}`,
      );
      el = el.parentElement;
    }
    return { found: true, chain };
  });
  console.log(`PROBE ${JSON.stringify(info)}`);
  expect(info.found).toBe(true);
});
