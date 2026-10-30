import { expect, type Page, test } from "@playwright/test";
import { MENU_EDGE_GAP } from "../src/design-system/anchored";
import { worklist } from "./mockapi/work";
import { mockApi } from "./seed";

/**
 * The Worklist drawer's row menu in a real browser. happy-dom has no layout
 * and no stacking, so only here can "the panel is on top and its first line
 * takes the press" be asserted. The mock answers GET /worklist only; the
 * disposition write is recorded from the request, not answered.
 */

// A row as GET /v1/worklist returns it for an inbound mail to the reader that
// asks for something and has no reply yet.
const WAITING_ROW = {
  above_next: { comparator: "waiting_days" },
  actions: ["open", "reply"],
  band: "now",
  because: [
    { kind: "buyer_wrote_last" },
    { kind: "waiting_days", value: { days: 0, kind: "days" } },
  ],
  brief_section: "respond_now",
  category: "customer_waiting",
  changed_since_brief: false,
  consequence: "buyer_waits",
  contact: {
    id: "01a16e34-d56e-74e8-b4e8-431c007701d5",
    label: "Sonya Beck",
    touch: {
      last_inbound_at: "2026-10-24T20:39:57+07:00",
      last_outbound_at: null,
    },
  },
  destination: "today",
  dispositions: ["handled", "snooze", "not_sales"],
  email_summary: {
    activity_id: "01a16e36-ef6a-7107-a907-b0a605203cfc",
    attachment_count: 0,
    direction: "inbound",
    display_status: "workspace",
    move: "needs_reply",
    occurred_at: "2026-10-24T19:00:56+07:00",
    preview: "Could you let me know by Friday?",
    request_has_reminder: false,
    subject: "Price list for the pumps",
    version: 2,
  },
  id: "01a16e36-ef6a-7107-a907-b0a605203cfc",
  level: 1,
  move: {
    action: "draft_reply",
    activity_id: "01a16e36-ef6a-7107-a907-b0a605203cfc",
  },
  occurred_at: "2026-10-24T19:00:56+07:00",
  owner: {
    id: "01a16e1f-78d2-770f-825e-0c4a8cc62423",
    kind: "user",
    label: "Demo Admin",
  },
  primary_action: "open",
  source: "customer_waiting",
  subject: { id: "01a16e34-d56e-74e8-b4e8-431c007701d5", type: "contact" },
  title: "Price list for the pumps",
  urgent: true,
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
    await expect(menus).toHaveCount(2);
    for (let row = 0; row < 2; row++) {
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

// The lines' heights and whether every line is on screen, for the open panel.
function openPanelShape(page: Page) {
  return page
    .locator(".overflow-menu-items:not([hidden])")
    .evaluate((panel) => {
      const box = panel.getBoundingClientRect();
      return {
        lines: [...panel.children].map((line) =>
          Math.round(line.getBoundingClientRect().height),
        ),
        scrolls: panel.scrollHeight > panel.clientHeight + 1,
        inside: box.top >= 0 && box.bottom <= innerHeight && box.height > 0,
      };
    });
}

// Room below the trigger less than the panel needs, so the panel must open
// upward rather than squeeze its lines into the room below.
test("a row menu with little room below opens upward at full height", async ({
  page,
}) => {
  await openQueue(page, "0");
  const menu = page
    .getByRole("dialog")
    .getByRole("button", { name: /Weitere Aktionen für Price list/ });
  const bottom = await menu.evaluate(
    (trigger) => trigger.getBoundingClientRect().bottom,
  );
  await page.setViewportSize({ width: 1280, height: Math.round(bottom) + 170 });
  await menu.click();
  const shape = await openPanelShape(page);
  expect(shape.lines).toHaveLength(7);
  for (const line of shape.lines) {
    expect(line).toBeGreaterThanOrEqual(30);
  }
  expect(shape).toMatchObject({ scrolls: false, inside: true });
});

// No room for the whole menu on either side: the lines keep their height and
// the panel scrolls.
test("a row menu taller than the window scrolls instead of squeezing", async ({
  page,
}) => {
  await openQueue(page, "0");
  const menu = page
    .getByRole("dialog")
    .getByRole("button", { name: /Weitere Aktionen für Price list/ });
  await page.setViewportSize({ width: 1280, height: 360 });
  const CLEAR_OF_LANES = 4;
  // The trigger sits CLEAR_OF_LANES below the sticky lanes, so the click's hit
  // test reaches it and the click does not scroll it elsewhere.
  const placed = await menu.evaluate((trigger, clear) => {
    const body = trigger.closest(".drawer-body");
    const lanes = body?.querySelector(".worklist-lanes");
    if (!body || !lanes) {
      throw new Error("the drawer body or its sticky lanes are missing");
    }
    body.scrollTop +=
      trigger.getBoundingClientRect().top -
      (lanes.getBoundingClientRect().bottom + clear);
    return {
      top: trigger.getBoundingClientRect().top,
      wanted: lanes.getBoundingClientRect().bottom + clear,
    };
  }, CLEAR_OF_LANES);
  expect(Math.abs(placed.top - placed.wanted)).toBeLessThanOrEqual(1);
  await menu.click();
  const room = await menu.evaluate((trigger, gap) => {
    const box = trigger.getBoundingClientRect();
    return {
      top: box.top,
      above: box.top - 2 * gap,
      below: innerHeight - box.bottom - 2 * gap,
    };
  }, MENU_EDGE_GAP);
  expect(room.top).toBe(placed.top);
  const menuHeight = await page
    .locator(".overflow-menu-items:not([hidden])")
    .evaluate((panel) => panel.scrollHeight);
  const shape = await openPanelShape(page);
  for (const line of shape.lines) {
    expect(line).toBeGreaterThanOrEqual(30);
  }
  expect(room.above).toBeLessThan(menuHeight);
  expect(room.below).toBeLessThan(menuHeight);
  expect(shape).toMatchObject({ scrolls: true, inside: true });
});
