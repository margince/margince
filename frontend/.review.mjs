import { chromium } from "@playwright/test";

const root =
  "/private/tmp/claude-502/-Users-josh-ziethen-Documents-Gradion-margince-poc-v1/b34e3e96-4ca3-4efd-b177-5fa8863a71d4/scratchpad";

const shots = [
  ["rev-platform", "onboarding-first-run--choosing-the-platform"],
  ["rev-microsoft", "onboarding-first-run--on-microsoft"],
  ["rev-model", "onboarding-first-run--binding-the-model"],
  ["rev-read", "onboarding-gate--mid-read"],
];

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
for (const [name, id] of shots) {
  await page.goto(`http://localhost:6006/iframe.html?id=${id}&viewMode=story`, {
    waitUntil: "networkidle",
  });
  await page.waitForTimeout(2600);
  // Full page, because what the user is reporting is partly content running
  // past the fold.
  await page.screenshot({ path: `${root}/${name}.png`, fullPage: true });
  console.log("captured", name);
}
await browser.close();
