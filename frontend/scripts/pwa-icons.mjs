// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const publicDir = join(root, "public");
const { chromium } = createRequire(join(root, "package.json"))(
  "@playwright/test",
);

const SIZES = [192, 512];
// The W3C maskable safe zone is a centred circle of radius 40% of the canvas;
// the mark's farthest point sits at this share of that radius.
const SAFE_RADIUS = 0.4;
const MARK_REACH = 0.82;

const iconSvg = readFileSync(join(publicDir, "icon.svg"), "utf8");
const ground = /<rect\b[^>]*\bfill="([^"]+)"/.exec(iconSvg)?.[1];
const mark = /<g\b[^>]*>([\s\S]*?)<\/g>/.exec(iconSvg)?.[1];
if (!ground || !mark) {
  throw new Error("public/icon.svg has no ground <rect> or no mark <g>");
}

const page = (body) =>
  `<!doctype html><style>html,body{margin:0;background:transparent}svg{display:block;width:100vw;height:100vh}</style>${body}`;

const maskable = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1">
  <rect width="1" height="1" fill="${ground}"/>
  <g id="mark">${mark}</g>
</svg>`;

function centreMark(reach) {
  const markGroup = document.getElementById("mark");
  const box = markGroup.getBBox();
  const cx = box.x + box.width / 2;
  const cy = box.y + box.height / 2;
  let farthest = 0;
  for (const path of markGroup.querySelectorAll("path")) {
    const length = path.getTotalLength();
    for (let at = 0; at <= length; at += length / 4000) {
      const point = path.getPointAtLength(at);
      farthest = Math.max(farthest, Math.hypot(point.x - cx, point.y - cy));
    }
  }
  const scale = reach / farthest;
  markGroup.setAttribute(
    "transform",
    `translate(.5 .5) scale(${scale}) translate(${-cx} ${-cy})`,
  );
}

const browser = await chromium.launch();
try {
  for (const size of SIZES) {
    const tab = await browser.newPage({
      viewport: { width: size, height: size },
      deviceScaleFactor: 1,
    });
    await tab.setContent(page(iconSvg));
    writeFileSync(
      join(publicDir, `icon-${size}x${size}.png`),
      await tab.screenshot({ omitBackground: true }),
    );
    await tab.setContent(page(maskable));
    await tab.evaluate(centreMark, SAFE_RADIUS * MARK_REACH);
    writeFileSync(
      join(publicDir, `web-app-manifest-${size}x${size}.png`),
      await tab.screenshot(),
    );
    await tab.close();
  }
} finally {
  await browser.close();
}
