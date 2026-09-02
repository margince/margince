import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { extname, join } from "node:path";
import { chromium } from "@playwright/test";

const root =
  "/private/tmp/claude-502/-Users-josh-ziethen-Documents-Gradion-margince-poc-v1/b34e3e96-4ca3-4efd-b177-5fa8863a71d4/scratchpad";
const types = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css" };
const server = createServer(async (req, res) => {
  const path = decodeURIComponent(new URL(req.url, "http://x").pathname);
  try {
    const body = await readFile(join(root, path));
    res.writeHead(200, { "Content-Type": types[extname(path)] ?? "text/plain" });
    res.end(body);
  } catch {
    res.writeHead(404);
    res.end("no");
  }
});
await new Promise((r) => server.listen(4611, r));

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
await page.goto("http://localhost:4611/firstrun.html", { waitUntil: "networkidle" });
await page.waitForTimeout(1200);

for (let i = 0; i < 9; i++) {
  // The crawl and the counters are the point, so give each step time to run.
  await page.waitForTimeout(2500);
  const stop = await page.evaluate(() => ({
    stop: document.getElementById("bandstop")?.textContent,
    sub: document.getElementById("bandsub")?.textContent,
  }));
  await page.screenshot({ path: `${root}/art-${i}.png` });
  console.log(i, JSON.stringify(stop));
  const next = await page.$("#next");
  if (next) await next.click();
}

await browser.close();
server.close();
