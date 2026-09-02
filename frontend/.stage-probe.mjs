import { chromium } from "@playwright/test";

const id = process.argv[2] ?? "onboarding-gate--at-rest";
const b = await chromium.launch();
const w = Number(process.argv[3] ?? 1440);
const p = await b.newPage({ viewport: { width: w, height: 900 } });
await p.goto(
  `http://localhost:6006/iframe.html?id=${id}&viewMode=story`,
  { waitUntil: "networkidle" },
);
await p.locator(".ob-stage").waitFor({ timeout: 20000 });
const out = await p.evaluate(() => {
  const q = (s) => document.querySelector(s);
  const box = (s) => {
    const e = q(s);
    if (!e) return "MISSING";
    const r = e.getBoundingClientRect();
    return {
      top: Math.round(r.top),
      bottom: Math.round(r.bottom),
      h: Math.round(r.height),
      row: getComputedStyle(e).gridRowStart,
    };
  };
  const st = q(".ob-stage");
  return {
    viewport: window.innerHeight,
    stageRows: getComputedStyle(st).gridTemplateRows,
    padTop: getComputedStyle(st).paddingTop,
    band: box(".ob-stage-band"),
    core: box(".ob-stage-core"),
    column: box(".ob-stage-column"),
    title: box(".ob-stage-title"),
  };
});
console.log(id, JSON.stringify(out, null, 1));
await b.close();
