// One-off: screenshot + overflow census of one story at several widths.
import { join } from "node:path";
import { loadPlaywright, serveStaticStorybook } from "./lib/storybook-harness.mjs";

const [id, ...widths] = process.argv.slice(2);
const staticDir = join(process.cwd(), "frontend/storybook-static");
const { port, close } = await serveStaticStorybook(staticDir);
const { chromium } = await loadPlaywright();
const browser = await chromium.launch();
for (const w of widths.map(Number)) {
  const page = await browser.newPage({ viewport: { width: w, height: 900 } });
  await page.goto(`http://localhost:${port}/iframe.html?id=${id}&viewMode=story`);
  await page.waitForSelector("table.table", { timeout: 15000 });
  await page.waitForTimeout(500);
  const census = await page.evaluate(() => {
    const t = document.querySelector("table.table");
    const box = (el) => {
      const r = el.getBoundingClientRect();
      const cs = getComputedStyle(el);
      return { left: Math.round(r.left), right: Math.round(r.right), w: Math.round(r.width), ox: cs.overflowX, sw: el.scrollWidth, cw: el.clientWidth };
    };
    const chain = [];
    for (let el = t; el && el !== document.body; el = el.parentElement) chain.push([el.className || el.tagName, box(el)]);
    return { docScroll: document.documentElement.scrollWidth, vw: innerWidth, chain: chain.slice(0, 8) };
  });
  console.log(w, JSON.stringify(census));
  await page.screenshot({ path: join(process.cwd(), `.tmp/calls-${id}-${w}.png`) });
}
await browser.close();
close();
