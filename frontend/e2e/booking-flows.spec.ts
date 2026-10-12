import { expect, test } from "@playwright/test";
import { de } from "../src/i18n/de";
import {
  bookingConnection,
  bookingContact,
  bookingInvitation,
  bookingProfile,
  bookingSlots,
} from "../src/screens/book.testkit";
import { copy } from "./copy";
import { mockApi } from "./seed";
import { openedAt } from "./waits";

test.beforeEach(async ({ page, context }) => {
  await mockApi(context);
  await page.clock.setFixedTime(new Date("2026-09-27T06:00:00Z"));
});

test("Meetings opens a booking for the contact", async ({ page }) => {
  await page.goto("/#/contacts/p-anna/meetings");
  await page
    .getByRole("button", { name: de["scheduling.bookContact"] })
    .click();
  await expect(page).toHaveURL(/#\/book\/contact-p-anna$/);
  await expect(
    page.getByRole("heading", {
      level: 1,
      name: copy(de["scheduling.bookWith"]),
    }),
  ).toBeVisible();
});

test("reconnecting in another tab refreshes permissions without losing the draft", async ({
  page,
  context,
}) => {
  let reconnected = false;
  await context.route("**/v1/connectors", (route) =>
    route.fulfill({
      json: {
        data: [
          {
            ...bookingConnection,
            scopes: reconnected
              ? bookingConnection.scopes
              : ["https://www.googleapis.com/auth/calendar.readonly"],
          },
        ],
      },
    }),
  );
  await context.route("**/v1/scheduling/profile", (route) =>
    route.fulfill({
      json: {
        ...bookingProfile,
        provider: reconnected ? "gcal" : "",
        enabled: false,
      },
    }),
  );
  await context.route("**/v1/scheduling/calendars?*", (route) =>
    route.fulfill({
      json: [
        { id: "work-id", name: "Work calendar", primary: true, writable: true },
      ],
    }),
  );
  await page.goto(`/#/book/contact-${bookingContact.id}`);
  await expect(page.getByText(de["scheduling.setupInSettings"])).toBeVisible();
  await page.getByLabel(de["scheduling.agenda"]).fill("Keep this draft");
  const popupPromise = page.waitForEvent("popup");
  await page.getByRole("link", { name: de["scheduling.openSettings"] }).click();
  const popup = await openedAt(popupPromise, /settings\/meetings/);
  await expect(
    popup.getByText(de["scheduling.readOnlyCalendar"]),
  ).toBeVisible();
  reconnected = true;
  await popup.close();
  await page.bringToFront();
  await page.evaluate(() =>
    window.dispatchEvent(new Event("visibilitychange")),
  );
  await expect(
    page.getByRole("link", { name: de["scheduling.openSettings"] }),
  ).toHaveCount(0);
  await expect(page.getByLabel(de["scheduling.agenda"])).toHaveValue(
    "Keep this draft",
  );
});

test("connected calendar setup keeps the draft, then sends and cancels an invitation", async ({
  page,
  context,
}) => {
  let configured = false;
  let status = "pending";
  const invitations: unknown[] = [];
  await context.route("**/v1/connectors", (route) =>
    route.fulfill({ json: { data: [bookingConnection] } }),
  );
  await context.route("**/v1/scheduling/calendars?*", (route) =>
    route.fulfill({
      json: [
        { id: "work-id", name: "Work calendar", primary: true, writable: true },
      ],
    }),
  );
  await context.route("**/v1/scheduling/profile", async (route) => {
    if (route.request().method() === "PUT") {
      expect(route.request().postDataJSON()).toMatchObject({
        provider: "gcal",
        calendar_id: "work-id",
        enabled: false,
      });
      configured = true;
    }
    await route.fulfill({
      json: {
        ...bookingProfile,
        provider: configured ? "gcal" : "",
        calendar_id: "work-id",
        enabled: false,
      },
    });
  });
  await context.route(`**/v1/contacts/${bookingContact.id}`, (route) =>
    route.fulfill({ json: bookingContact }),
  );
  await context.route("**/v1/availability?*", (route) =>
    route.fulfill({ json: { slots: bookingSlots, truncated: false } }),
  );
  await context.route("**/v1/scheduling/invitations", async (route) => {
    invitations.push(route.request().postDataJSON());
    await route.fulfill({ json: bookingInvitation, status: 201 });
  });
  await context.route(
    `**/v1/scheduling/invitations/${bookingInvitation.id}`,
    async (route) => {
      if (route.request().method() === "PATCH") {
        expect(route.request().postDataJSON()).toMatchObject({
          action: "cancel",
        });
        status = "canceling";
      }
      await route.fulfill({ json: { ...bookingInvitation, status } });
    },
  );
  await page.goto(`/#/book/contact-${bookingContact.id}`);
  await page.getByLabel(de["scheduling.agenda"]).fill("Discuss scope");
  const settingsPromise = page.waitForEvent("popup");
  await page.getByRole("link", { name: de["scheduling.openSettings"] }).click();
  const settings = await settingsPromise;
  await expect(
    settings.getByRole("heading", { name: de["scheduling.availabilityTitle"] }),
  ).toBeVisible();
  await expect(
    settings.getByRole("combobox", { name: de["scheduling.provider"] }),
  ).toHaveCount(0);
  await settings
    .getByRole("button", { name: de["scheduling.save"], exact: true })
    .click();
  await expect(
    settings.getByText(de["settings.saved"], { exact: true }),
  ).toBeVisible();
  await settings.close();
  await page.bringToFront();
  await page.evaluate(() =>
    window.dispatchEvent(new Event("visibilitychange")),
  );
  await expect(page.getByLabel(de["scheduling.agenda"])).toHaveValue(
    "Discuss scope",
  );
  await page
    .getByRole("radio", { name: new RegExp(de["scheduling.invite"]) })
    .check();
  await page.locator(".meeting-week button").first().click();
  await page
    .getByRole("button", { name: copy(de["scheduling.sendInviteAt"]) })
    .click();
  await expect(page.getByRole("status")).toContainText(
    de["scheduling.pending"],
  );
  expect(invitations).toHaveLength(1);
  expect(invitations[0]).toEqual({
    contact_id: bookingContact.id,
    attendee_email: bookingContact.primary_email,
    description: "Discuss scope",
    subject: bookingProfile.title,
    location: "",
    video_call: true,
    ...bookingSlots[0],
  });
  await page.getByRole("button", { name: de["scheduling.cancel"] }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: de["scheduling.cancel"] })
    .click();
  await expect(page.getByRole("status")).toContainText(
    de["scheduling.canceling"],
  );
});

test("personal proposal opens the guest page and accepts the chosen time", async ({
  page,
  context,
}) => {
  await context.route("**/v1/connectors", (route) =>
    route.fulfill({ json: { data: [bookingConnection] } }),
  );
  await context.route(`**/v1/contacts/${bookingContact.id}`, (route) =>
    route.fulfill({ json: bookingContact }),
  );
  await context.route("**/v1/availability?*", (route) =>
    route.fulfill({ json: { slots: bookingSlots, truncated: false } }),
  );
  await context.route("**/v1/scheduling/proposals", async (route) => {
    expect(route.request().postDataJSON().options).toEqual(bookingSlots);
    await route.fulfill({
      status: 201,
      json: {
        id: "proposal-1",
        url: "/#/book/proposal-personal",
        expires_at: "2026-10-06T09:00:00Z",
      },
    });
  });
  await context.route("**/v1/public/proposal/personal", async (route) => {
    if (route.request().method() === "POST") {
      expect(route.request().postDataJSON()).toMatchObject({
        ...bookingSlots[0],
        consent: { policy_version: "2026-07" },
      });
      await route.fulfill({
        json: { ...bookingInvitation, management_token: "guest-booking" },
      });
    } else
      await route.fulfill({
        json: {
          profile: { ...bookingProfile, title: "Personal discovery" },
          description: "Discuss scope",
          options: bookingSlots,
          expires_at: "2026-10-06T09:00:00Z",
          used: false,
        },
      });
  });
  await context.route(
    "**/v1/public/proposal/personal/availability?*",
    (route) => route.fulfill({ json: { slots: [], truncated: false } }),
  );
  await page.goto(`/#/book/contact-${bookingContact.id}`);
  await page.locator(".meeting-week button").nth(0).click();
  await page.locator(".meeting-week button").nth(1).click();
  await page
    .getByRole("button", { name: copy(de["scheduling.reviewTimes_other"]) })
    .click();
  await expect(page.getByText(copy(de["scheduling.linkReady"]))).toBeVisible();
  await page.goto("/#/book/proposal-personal");
  await expect(
    page.getByRole("heading", { name: "Personal discovery" }),
  ).toBeVisible();
  await page
    .locator(".bookguest-suggested .meeting-slots button")
    .first()
    .click();
  const confirm = page.getByRole("button", {
    name: copy(de["scheduling.confirmAt"]),
  });
  await expect(confirm).toBeDisabled();
  await page.getByRole("checkbox", { name: de["book.consentWording"] }).check();
  await confirm.click();
  await expect(page).toHaveURL(/#\/book\/manage-guest-booking$/);
});

test("guest reschedules only after confirming the replacement time", async ({
  page,
  context,
}) => {
  let status = "confirmed";
  const changes: unknown[] = [];
  await context.route("**/v1/public/meeting/guest-booking", async (route) => {
    if (route.request().method() === "PATCH") {
      changes.push(route.request().postDataJSON());
      status = "rescheduling";
    }
    await route.fulfill({ json: { ...bookingInvitation, status } });
  });
  await context.route(
    "**/v1/public/meeting/guest-booking/availability?*",
    (route) =>
      route.fulfill({ json: { slots: bookingSlots, truncated: false } }),
  );
  await page.goto("/#/book/manage-guest-booking");
  await page.getByRole("button", { name: de["scheduling.reschedule"] }).click();
  await page.locator(".meeting-slots button").last().click();
  expect(changes).toHaveLength(0);
  await page.getByRole("button", { name: de["scheduling.saveTime"] }).click();
  await expect(page.getByRole("status")).toContainText(
    de["scheduling.rescheduling"],
  );
  expect(changes).toEqual([
    { action: "reschedule", version: 1, ...bookingSlots[1] },
  ]);
});

test("a busy week says so per day, finds later times, and stops paging at the booking horizon", async ({
  page,
  context,
}) => {
  let calls = 0;
  await context.route("**/v1/connectors", (route) =>
    route.fulfill({ json: { data: [bookingConnection] } }),
  );
  await context.route("**/v1/availability?*", (route) =>
    route.fulfill({
      json: { slots: ++calls === 1 ? [] : bookingSlots, truncated: false },
    }),
  );
  await page.goto(`/#/book/contact-${bookingContact.id}`);
  await expect(
    page.getByText(de["scheduling.noFreeTime"]).first(),
  ).toBeVisible();
  await page.getByRole("button", { name: de["scheduling.findNext"] }).click();
  await expect(page.locator(".meeting-week button")).toHaveCount(2);
  expect(calls).toBe(2);
  const next = page.getByRole("button", { name: de["scheduling.nextWeek"] });
  for (let week = 0; week < 4; week++) await next.click();
  await expect(next).toBeDisabled();
});

test("meeting settings shows the reusable link, saves hours, and previews a paused page", async ({
  page,
  context,
}) => {
  const availability: string[] = [];
  await context.route("**/v1/availability?*", async (route) => {
    const url = new URL(route.request().url());
    expect(url.searchParams.get("reliable")).toBe("true");
    availability.push(url.searchParams.get("from") ?? "");
    await route.fulfill({ json: { slots: bookingSlots, truncated: false } });
  });
  let profile = { ...bookingProfile, enabled: false };
  const writes: unknown[] = [];
  await context.route("**/v1/connectors", (route) =>
    route.fulfill({ json: { data: [bookingConnection] } }),
  );
  await context.route("**/v1/scheduling/calendars?*", (route) =>
    route.fulfill({
      json: [{ id: "primary", name: "Work", primary: true, writable: true }],
    }),
  );
  await context.route("**/v1/scheduling/profile", async (route) => {
    if (route.request().method() === "PUT") {
      const input = route.request().postDataJSON();
      writes.push(input);
      profile = { ...profile, notice_minutes: input.notice_minutes };
    }
    await route.fulfill({ json: profile });
  });
  await page.goto("/#/settings/meetings");
  await expect(
    page.getByRole("textbox", { name: de["scheduling.myLink"] }),
  ).toHaveValue(bookingProfile.public_url ?? "");
  await expect(
    page.getByRole("button", { name: de["scheduling.copyLink"], exact: true }),
  ).toBeVisible();
  await expect(page.getByLabel("Firmenname")).toHaveCount(0);
  await expect(
    page.getByPlaceholder("https://meet.google.com/abc-defg-hij"),
  ).toBeVisible();
  await page.getByLabel(de["scheduling.notice"]).fill("24");
  await page
    .getByRole("button", { name: de["scheduling.save"], exact: true })
    .click();
  await expect(
    page.getByText(de["settings.saved"], { exact: true }),
  ).toBeVisible();
  expect(writes).toHaveLength(1);
  expect(writes[0]).toMatchObject({ notice_minutes: 1440, enabled: false });
  await page.getByRole("button", { name: de["scheduling.preview"] }).click();
  await expect(page).toHaveURL(/#\/book\/preview$/);
  await expect(page.getByText(de["scheduling.previewPaused"])).toBeVisible();
  await expect(
    page.getByRole("heading", { name: bookingProfile.title }),
  ).toBeVisible();
  await page.getByRole("button", { name: de["calendar.nextMonth"] }).click();
  // October in Berlin holds the clock going back, so it runs past the
  // server's 31-day bound and is read in two windows after September's one.
  await expect.poll(() => availability.length).toBe(3);
  const slot = page.locator(".bookguest-times .meeting-slots button").first();
  await expect(slot).toBeVisible();
  await slot.click();
  await page.getByLabel(de["book.name"], { exact: true }).fill("Demo guest");
  await page
    .getByLabel(de["book.email"], { exact: true })
    .fill("guest@example.test");
  await page.getByRole("checkbox").check();
  await expect(
    page.getByRole("button", { name: copy(de["scheduling.confirmAt"]) }),
  ).toBeDisabled();
  await page.getByRole("button", { name: de["scheduling.changeTime"] }).click();
  await expect(slot).toBeVisible();
  expect(writes).toHaveLength(1);
});

test("calendar setup enables booking with the existing Account name", async ({
  page,
  context,
}) => {
  let profile = { ...bookingProfile, provider: "", enabled: false };
  const writes: unknown[] = [];
  await context.route("**/v1/connectors", (route) =>
    route.fulfill({ json: { data: [bookingConnection] } }),
  );
  await context.route("**/v1/scheduling/calendars?*", (route) =>
    route.fulfill({
      json: [{ id: "primary", name: "Work", primary: true, writable: true }],
    }),
  );
  await context.route("**/v1/scheduling/profile", async (route) => {
    if (route.request().method() === "PUT") {
      const input = route.request().postDataJSON();
      writes.push(input);
      profile = { ...profile, ...input, host_name: bookingProfile.host_name };
    }
    await route.fulfill({ json: profile });
  });
  await page.goto("/#/settings/meetings");
  await expect(
    page.getByText(de["scheduling.hostName"], { exact: true }),
  ).toBeVisible();
  await expect(page.locator(".factlist")).toContainText(
    bookingProfile.host_name ?? "",
  );
  await expect(
    page
      .getByRole("main")
      .getByRole("link", { name: de["settings.tab.account"], exact: true }),
  ).toHaveAttribute("href", "#/settings/account");
  await expect(
    page.getByRole("button", { name: de["scheduling.resume"], exact: true }),
  ).toBeDisabled();
  await page
    .getByRole("button", { name: de["scheduling.save"], exact: true })
    .click();
  await expect(
    page.getByText(de["settings.saved"], { exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: de["scheduling.resume"], exact: true })
    .click();
  await expect(
    page.getByText(de["scheduling.active"], { exact: true }),
  ).toHaveCount(2);
  expect(writes).toHaveLength(2);
  expect(writes[1]).toMatchObject({
    provider: "gcal",
    enabled: true,
    host_name: bookingProfile.host_name,
  });
  await expect(
    page.getByRole("button", { name: "Signaturlink kopieren" }),
  ).toHaveCount(0);
});
