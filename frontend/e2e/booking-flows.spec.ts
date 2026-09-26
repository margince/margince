import { expect, test } from "@playwright/test";
import { de } from "../src/i18n/de";
import { bookingConnection, bookingContact, bookingInvitation, bookingProfile, bookingSlots } from "../src/screens/book.testkit";
import { mockApi } from "./seed";

test.beforeEach(async ({ page }) => { await mockApi(page); });

test("Meetings opens a booking for the contact", async ({page}) => {
 await page.goto("/#/contacts/p-anna/meetings");
 await page.getByRole("button", {name: de["scheduling.bookContact"]}).click();
 await expect(page).toHaveURL(/#\/book\/contact-p-anna$/);
 await expect(page.getByRole("heading", {name: de["scheduling.new"]})).toBeVisible();
});

test("reconnecting in another tab refreshes permissions without losing the draft", async ({page}) => {
 let reconnected = false;
 await page.route("**/v1/connectors", route => route.fulfill({json: {data: [{...bookingConnection, scopes: reconnected ? bookingConnection.scopes : ["https://www.googleapis.com/auth/calendar.readonly"]}]}}));
 await page.route("**/v1/scheduling/profile", route => route.fulfill({json: {...bookingProfile, provider: "", enabled: false}}));
 await page.route("**/v1/scheduling/calendars?*", route => route.fulfill({json: [{id: "work-id", name: "Work calendar", primary: true, writable: true}]}));
 await page.goto(`/#/book/contact-${bookingContact.id}`);
 await expect(page.getByText(de["scheduling.readOnlyCalendar"])).toBeVisible();
 await page.getByLabel(de["scheduling.agenda"]).fill("Keep this draft");
 const popupPromise = page.waitForEvent("popup");
 await page.getByRole("link", {name: de["scheduling.manageConnection"]}).click();
 const popup = await popupPromise;
 await expect(popup).toHaveURL(/settings\/connections/);
 reconnected = true;
 await popup.close();
 await page.bringToFront();
 await page.evaluate(() => window.dispatchEvent(new Event("visibilitychange")));
 await expect(page.getByRole("button", {name: de["scheduling.useCalendar"]})).toBeEnabled();
 await expect(page.getByLabel(de["scheduling.agenda"])).toHaveValue("Keep this draft");
});

test("connected calendar setup keeps the draft, then sends and cancels an invitation", async ({page}) => {
 let configured = false;
 let status = "pending";
 const invitations: unknown[] = [];
 await page.route("**/v1/connectors", route => route.fulfill({json: {data: [bookingConnection]}}));
 await page.route("**/v1/scheduling/calendars?*", route => route.fulfill({json: [{id: "work-id", name: "Work calendar", primary: true, writable: true}]}));
 await page.route("**/v1/scheduling/profile", async route => {
  if (route.request().method() === "PUT") {
   expect(route.request().postDataJSON()).toMatchObject({provider: "gcal", calendar_id: "work-id", enabled: false}); configured = true;
  }
  await route.fulfill({json: {...bookingProfile, provider: configured ? "gcal" : "", calendar_id: "work-id", enabled: false}});
 });
 await page.route(`**/v1/contacts/${bookingContact.id}`, route => route.fulfill({json: bookingContact}));
 await page.route("**/v1/availability?*", route => route.fulfill({json: {slots: bookingSlots, truncated: false}}));
 await page.route("**/v1/scheduling/invitations", async route => {invitations.push(route.request().postDataJSON()); await route.fulfill({json: bookingInvitation, status: 201});});
 await page.route(`**/v1/scheduling/invitations/${bookingInvitation.id}`, async route => {
  if (route.request().method() === "PATCH") { expect(route.request().postDataJSON()).toMatchObject({action: "cancel"}); status = "canceling"; }
  await route.fulfill({json: {...bookingInvitation, status}});
 });
 await page.goto(`/#/book/contact-${bookingContact.id}`);
 await page.getByLabel(de["scheduling.agenda"]).fill("Discuss scope");
 await page.getByRole("button", {name: de["scheduling.useCalendar"]}).click();
 await expect(page.getByLabel(de["scheduling.agenda"])).toHaveValue("Discuss scope");
 await page.getByRole("combobox", {name: de["scheduling.method"]}).click();
 await page.getByRole("option", {name: de["scheduling.invite"]}).click();
 await page.locator(".meeting-slots button").first().click();
 await page.getByRole("button", {name: de["scheduling.invite"], exact: true}).click();
 await expect(page.getByRole("heading", {name: de["scheduling.pending"]})).toBeVisible();
 expect(invitations).toHaveLength(1);
 expect(invitations[0]).toEqual({contact_id: bookingContact.id, attendee_email: bookingContact.primary_email, description: "Discuss scope", subject: bookingProfile.title, location: bookingProfile.location, ...bookingSlots[0]});
 await page.getByRole("button", {name: de["scheduling.cancel"]}).click();
 await page.getByRole("dialog").getByRole("button", {name: de["scheduling.cancel"]}).click();
 await expect(page.getByRole("heading", {name: de["scheduling.canceling"]})).toBeVisible();
});

test("personal proposal opens the guest page and accepts the chosen time", async ({page}) => {
 await page.route("**/v1/connectors", route => route.fulfill({json: {data: [bookingConnection]}}));
 await page.route(`**/v1/contacts/${bookingContact.id}`, route => route.fulfill({json: bookingContact}));
 await page.route("**/v1/availability?*", route => route.fulfill({json: {slots: bookingSlots, truncated: false}}));
 await page.route("**/v1/scheduling/proposals", async route => {
  expect(route.request().postDataJSON().options).toEqual(bookingSlots);
  await route.fulfill({status:201, json: {id:"proposal-1", url:"/#/book/proposal-personal", expires_at:"2026-10-06T09:00:00Z"}});
 });
 await page.route("**/v1/public/proposal/personal", async route => {
  if(route.request().method()==="POST") {
   expect(route.request().postDataJSON()).toMatchObject({...bookingSlots[0],consent:{policy_version:"2026-07"}});
   await route.fulfill({json:{...bookingInvitation,management_token:"guest-booking"}});
  } else await route.fulfill({json:{profile:{...bookingProfile,title:"Personal discovery"},description:"Discuss scope",options:bookingSlots,expires_at:"2026-10-06T09:00:00Z",used:false}});
 });
 await page.route("**/v1/public/proposal/personal/availability?*", route => route.fulfill({json:{slots:[],truncated:false}}));
 await page.goto(`/#/book/contact-${bookingContact.id}`);
 await page.locator(".meeting-slots button").nth(0).click();
 await page.locator(".meeting-slots button").nth(1).click();
 await page.getByRole("button",{name:de["scheduling.reviewProposal"]}).click();
 await page.getByRole("link",{name:de["scheduling.personalLink"]}).click();
 await expect(page.getByRole("heading",{name:"Personal discovery"})).toBeVisible();
 await page.locator(".meeting-slots button").first().click();
 await expect(page.getByRole("button",{name:de["scheduling.book"]})).toBeDisabled();
 await page.getByRole("checkbox",{name:de["book.consentWording"]}).check();
 await page.getByRole("button",{name:de["scheduling.book"]}).click();
 await expect(page).toHaveURL(/#\/book\/manage-guest-booking$/);
});

test("guest reschedules only after confirming the replacement time", async ({page}) => {
 let status="confirmed";
 const changes:unknown[]=[];
 await page.route("**/v1/public/meeting/guest-booking", async route => {
  if(route.request().method()==="PATCH") {changes.push(route.request().postDataJSON());status="rescheduling";}
  await route.fulfill({json:{...bookingInvitation,status}});
 });
 await page.route("**/v1/public/meeting/guest-booking/availability?*", route => route.fulfill({json:{slots:bookingSlots,truncated:false}}));
 await page.goto("/#/book/manage-guest-booking");
 await page.getByRole("button",{name:de["scheduling.reschedule"]}).click();
 await page.locator(".meeting-slots button").last().click();
 expect(changes).toHaveLength(0);
 await page.getByRole("button",{name:de["scheduling.saveTime"]}).click();
 await expect(page.getByRole("heading",{name:de["scheduling.rescheduling"]})).toBeVisible();
 expect(changes).toEqual([{action:"reschedule",version:1,...bookingSlots[1]}]);
});
