import { chromium } from "@playwright/test";

const root =
  "/private/tmp/claude-502/-Users-josh-ziethen-Documents-Gradion-margince-poc-v1/b34e3e96-4ca3-4efd-b177-5fa8863a71d4/scratchpad";

const shots = [
  ["j0-shell", "onboarding-conversation-shell--a-first-run"],
  ["j1-reading", "onboarding-conversation-company-act--reading"],
  ["j2-review", "onboarding-conversation-company-act--review"],
  ["j3-clarifying", "onboarding-conversation-company-act--clarifying"],
  ["j4-confirmed", "onboarding-conversation-company-act--confirmed"],
  ["j5-voice", "onboarding-conversation-voice-act--empty"],
  ["j6-connect", "onboarding-conversation-connect-act--done"],
];

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
page.on("pageerror", (e) => console.log("[pageerror]", e.message));

for (const [name, id] of shots) {
  await page.goto(`http://localhost:6006/iframe.html?id=${id}&viewMode=story`, {
    waitUntil: "networkidle",
  });
  await page.waitForTimeout(2600);
  await page.screenshot({ path: `${root}/${name}.png` });
  console.log("captured", name);
}

await browser.close();
