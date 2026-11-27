import { expect, test } from "@playwright/test";
import { de } from "../src/i18n/de";
import { mockApi } from "./seed";

/**
 * A draft survives the reader leaving the page it is on.
 *
 * Asserted end to end because the thing under test is where the guard SITS. It
 * is rendered above the routed screen (App.tsx), and the reason is only visible
 * across a whole navigation: a guard installed inside the settings screen
 * unmounted with that screen, so it caught a move from one settings entry to the
 * next and lost the draft without a word the moment the reader clicked anything
 * in the sidebar. A unit test of the guard cannot tell those two arrangements
 * apart — both hold content when the address changes — so the proof has to be a
 * real route change in a real shell.
 */

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

type Page = import("@playwright/test").Page;

// The signature layout on Company profile → Email. The page's save bar holds
// its draft, so the edit is still unsaved on the way out of the page.
const SIGNATURE_LAYOUT = "<p><b>{name}</b></p>";

const template = (page: Page) =>
  page.getByRole("textbox", { name: de["signatureTemplate.label"] });

// The address the product MINTS for this page, not one it redirects from, so
// the guard is asserted against an address the app keeps.
const typeIntoTemplate = async (page: Page, text: string) => {
  await page.route(/\/v1\/email-signature-template$/, (route) =>
    route.fulfill({ json: { template: SIGNATURE_LAYOUT } }),
  );
  await page.goto("/#/settings/company/email");
  await template(page).fill(text);
};

// Selected by destination rather than by name: this link's accessible name is
// the installation's own company, which a fixture is free to change, and what
// the test needs is only that it goes somewhere outside settings.
const leaveSettings = (page: Page) => page.locator('a[href="/#/home"]').click();

test("a settings draft holds the page when the reader leaves for another screen", async ({
  page,
}) => {
  await typeIntoTemplate(page, "<p>{name} at Brandt</p>");

  // Out of settings entirely, which is the move the old guard could not see.
  // The brand link is the way out: while settings is open the rail holds the
  // settings entries, so leaving means the shell's own header.
  await leaveSettings(page);

  const asking = page.getByRole("dialog");
  await expect(asking).toBeVisible();

  // Dismissing the question is the SAFE answer: it keeps the edit and puts the
  // address back, so the reader is where their work is.
  await page.keyboard.press("Escape");
  await expect(asking).toBeHidden();
  await expect(page).toHaveURL(/#\/settings\/company\/email$/);
  await expect(template(page)).toHaveValue("<p>{name} at Brandt</p>");
});

test("discarding leaves for the screen the reader asked for", async ({
  page,
}) => {
  await typeIntoTemplate(page, "<p>{name}</p><p>Brandt</p>");
  await leaveSettings(page);

  await page
    .getByRole("dialog")
    .getByRole("button", { name: /verwerfen/i })
    .click();
  await expect(page.getByRole("dialog")).toBeHidden();
  await expect(page).toHaveURL(/#\/home$/);
});

// The reporting time zone, edited in the dialog the company card's verb opens.
const timezone = (page: Page) =>
  page.getByRole("textbox", { name: de["installationSettings.timezone"] });

const editTimezone = (page: Page) =>
  page.getByRole("button", {
    name: de["installationSettings.editField"].replace(
      "{field}",
      de["installationSettings.timezone"],
    ),
  });

test("Cancel discards a settings dialog's draft, so leaving the page asks nothing", {
  tag: "@smoke",
}, async ({ page }) => {
  await page.goto("/#/settings/company/regional");
  await editTimezone(page).click();
  const saved = await timezone(page).inputValue();
  await timezone(page).fill("Europe/Vienna");
  await page.getByRole("button", { name: de["create.cancel"] }).click();
  await expect(page.getByRole("dialog")).toBeHidden();

  await editTimezone(page).click();
  await expect(timezone(page)).toHaveValue(saved);
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toBeHidden();

  await leaveSettings(page);
  await expect(page).toHaveURL(/#\/home$/);
});

test("Escape over a changed settings dialog asks before it discards", async ({
  page,
}) => {
  await page.goto("/#/settings/company/regional");
  await editTimezone(page).click();
  await timezone(page).fill("Europe/Vienna");
  await page.keyboard.press("Escape");

  const question = page.getByRole("dialog", { name: de["unsaved.title"] });
  await expect(question).toBeVisible();
  await question.getByRole("button", { name: de["unsaved.discard"] }).click();
  await expect(page.getByRole("dialog")).toBeHidden();
});
