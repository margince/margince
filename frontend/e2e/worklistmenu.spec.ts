import { expect, type Page, test } from "@playwright/test";
import { worklist } from "./mockapi/work";
import { mockApi } from "./seed";

/**
 * The Worklist drawer's row menu in a real browser. happy-dom has no layout
 * and no stacking, so only here can "the panel is on top and its first line
 * takes the press" be asserted. The mock answers GET /worklist only; the
 * disposition write is recorded from the request, not answered.
 */

const WAITING_ROW = {
  id: "waiting-sonya",
  source: "customer_waiting",
  category: "customer_waiting",
  band: "now",
  level: 1,
  urgent: true,
  consequence: "buyer_waits",
  title: "Meet next Tues?",
  because: [{ kind: "buyer_wrote_last" }],
  subject: { type: "contact", id: "contact-sonya", label: "Sonya Beck" },
  move: { action: "draft_reply", activity_id: "mail-sonya" },
  actions: ["open"],
  dispositions: ["handled", "snooze", "not_mine", "not_sales"],
};

const WIDTHS = [
  { name: "normal width", wide: "0" },
  { name: "expanded to the window", wide: "1" },
] as const;

async function openQueue(page: Page, wide: string) {
  await mockApi(page);
  await page.addInitScript((value) => {
    localStorage.setItem("margince.worklist.drawerWide", value);
  }, wide);
  await page.route("**/v1/worklist*", (route) =>
    route.fulfill({
      json: { ...worklist, queue: [WAITING_ROW, ...worklist.queue] },
    }),
  );
  await page.goto("/#/home?queue=1", { waitUntil: "networkidle" });
}

for (const { name, wide } of WIDTHS) {
  test(`a row menu opens above the drawer and a line runs, ${name}`, async ({
    page,
  }) => {
    await openQueue(page, wide);
    const drawer = page.getByRole("dialog");
    const menus = drawer.getByRole("button", { name: /Weitere Aktionen für/ });
    await expect(menus).toHaveCount(3);
    for (let row = 0; row < 3; row++) {
      await menus.nth(row).click();
      const panel = page.locator(".overflow-menu-items:not([hidden])");
      await expect(panel).toHaveCount(1);
      await expect(menus.nth(row)).toHaveAttribute("aria-expanded", "true");
      const covered = await panel.evaluate((element) => {
        const box = element.getBoundingClientRect();
        const top = document.elementFromPoint(
          box.left + box.width / 2,
          box.top + Math.min(20, box.height / 2),
        );
        return {
          onTop: element.contains(top),
          inside: box.top >= 0 && box.bottom <= innerHeight && box.height > 0,
        };
      });
      expect(covered).toEqual({ onTop: true, inside: true });
      await menus.nth(row).click();
      await expect(panel).toHaveCount(0);
    }

    const writes: string[] = [];
    page.on("request", (request) => {
      if (request.method() === "PUT") {
        writes.push(`${request.url()} ${request.postData()}`);
      }
    });
    await menus.first().click();
    await page
      .locator(".overflow-menu-items:not([hidden])")
      .getByRole("button", { name: "Zurückstellen", exact: true })
      .click();
    await expect.poll(() => writes.length).toBe(1);
    expect(writes[0]).toContain("/activities/");
    expect(writes[0]).toContain("/disposition");
    expect(writes[0]).toContain("snooze");
  });
}
