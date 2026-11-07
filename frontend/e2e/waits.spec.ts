import { expect, test } from "@playwright/test";
import { openedAt } from "./waits";

test("an opened tab waits for its document before UI assertions start", async ({
  page,
  context,
}) => {
  let releaseModule = () => {};
  const moduleGate = new Promise<void>((resolve) => {
    releaseModule = resolve;
  });
  await context.route("**/popup-readiness", (route) =>
    route.fulfill({
      contentType: "text/html",
      body: '<p>Opening the document</p><script type="module" src="/popup-readiness.js"></script>',
    }),
  );
  await context.route("**/popup-readiness.js", async (route) => {
    await moduleGate;
    await route.fulfill({
      contentType: "application/javascript",
      body: 'document.body.textContent = "Document ready";',
    });
  });
  await context.route("**/popup-opener", (route) =>
    route.fulfill({ contentType: "text/html", body: "<p>Opener</p>" }),
  );
  await page.goto("/popup-opener");
  const opening = page.waitForEvent("popup");
  await page.evaluate(() => window.open("/popup-readiness", "_blank"));
  let settled = false;
  const ready = openedAt(opening, /popup-readiness$/).then((popup) => {
    settled = true;
    return popup;
  });
  const popup = await opening;
  try {
    await expect(popup.getByText("Opening the document")).toBeVisible();
    expect(settled).toBe(false);
  } finally {
    releaseModule();
  }
  expect(await ready).toBe(popup);
  await expect(popup.getByText("Document ready")).toBeVisible();
});
