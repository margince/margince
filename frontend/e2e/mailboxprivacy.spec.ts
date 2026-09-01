import { expect, test } from "@playwright/test";
import { mockApi } from "./seed";

// The mailbox-privacy surfaces, asserted in the language they ship in.
//
// Every string here is a claim about who may read somebody's mail, so the words
// are the product rather than decoration: "held until classified" and "shared
// with the team" are the difference between a colleague seeing a message and
// not. A screen whose German drifted would still pass a typecheck and still
// render, and nobody would notice until a customer read it.
//
// The suite runs against the network-edge mock (`mockApi`), so it is hermetic
// and runs in CI. Point BASE_URL at a live stack to run the same assertions
// unmocked.

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

test("AC-mailbox-1: a mailbox says who may read it, and refuses shared without the opt-in", async ({
  page,
}) => {
  await page.goto("/#/settings/connections");
  const row = page.getByTestId("connector-gmail-mail-posture");
  await expect(row).toBeVisible();

  // The stored posture, and the sentence saying what it does.
  await expect(row).toContainText("Wer E-Mails aus diesem Postfach lesen darf");
  await expect(row).toContainText("Zurückgehalten bis eingestuft");
  await expect(row).toContainText(
    "Eine neue Nachricht bleibt auf die Beteiligten beschränkt",
  );
  // The refusal rides on the ROW, not on the option label: a listbox option
  // ellipsises, and the half that gets cut is the reason a reader needs.
  await expect(row).toContainText(
    "muss eine Administratorin für diese Organisation erlauben",
  );

  await row.getByRole("combobox").click();
  const shared = page.getByRole("option", { name: "Mit dem Team geteilt" });
  await expect(shared).toHaveAttribute("aria-disabled", "true");
  await expect(
    page.getByRole("option", { name: "Immer zurückgehalten" }),
  ).not.toHaveAttribute("aria-disabled", "true");
});

test("AC-mailbox-2: narrowing the posture offers to narrow the history too", async ({
  page,
}) => {
  await page.goto("/#/settings/connections");
  const row = page.getByTestId("connector-gmail-mail-posture");
  await row.getByRole("combobox").click();
  await page.getByRole("option", { name: "Immer zurückgehalten" }).click();

  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("Und die bereits erfassten E-Mails?");
  // One question and one modifier, never two rival verbs: the German labels do
  // not both fit the compact confirm, and a clipped primary button is what the
  // three-button version shipped as.
  const alsoHistory = dialog.getByRole("checkbox");
  await expect(alsoHistory).not.toBeChecked();
  await alsoHistory.check();
  await dialog.getByRole("button", { name: "Sichtbarkeit ändern" }).click();

  await expect(row).toContainText("Immer zurückgehalten");
  await expect(row).toContainText("unabhängig von jeder Einstufung");
});

test("AC-mailbox-3: the admin opt-in states what turning it on asserts", async ({
  page,
}) => {
  await page.goto("/#/settings/connections");
  const toggle = page.getByTestId("shared-posture-allowed-toggle");
  await expect(toggle).toBeVisible();
  await expect(toggle).toHaveAttribute("aria-checked", "false");
  // The warning belongs to ON. This is the one capture setting whose default
  // withholds, so it is the one row where the permissive answer is the one that
  // needs saying out loud.
  await expect(page.getByText(/Betriebsvereinbarung/)).toHaveCount(0);

  await toggle.click();
  await expect(page.getByText(/Betriebsvereinbarung/)).toBeVisible();
  await expect(page.getByText(/Margince prüft das nicht/)).toBeVisible();
});

test("AC-mailbox-4: the Senders page shows every decision and takes an overrule", async ({
  page,
}) => {
  await page.goto("/#/settings/connections");
  const senders = page.locator(".panel", { hasText: "Absender" }).first();
  await expect(senders).toBeVisible();
  await expect(senders).toContainText("Eine Person");
  await expect(senders).toContainText("Ein Newsletter");
  await expect(senders).toContainText("Privat");

  const row = senders.locator("tr", { hasText: "news@substack.com" });
  await row.getByRole("button", { name: "Geschäftlich" }).click();
  // An overruled row says whose answer stands, because the reader auditing this
  // list needs to tell their own decisions from the classifier's.
  await expect(row).toContainText("von Ihnen entschieden");
  await expect(row.getByRole("button", { name: "Zurücknehmen" })).toBeVisible();
});

test("AC-mailbox-5: a sender kept out is told what that destroys", async ({
  page,
}) => {
  await page.goto("/#/settings/connections");
  const senders = page.locator(".panel", { hasText: "Absender" }).first();
  await senders
    .locator("tr", { hasText: "anne@hotmail.com" })
    .getByRole("button", { name: "Aussperren" })
    .click();

  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("dauerhaft aussperren");
  await expect(dialog).toContainText("vernichtet");
  // What SURVIVES is the half a reader cannot guess: a colleague's own import
  // of the same message is theirs, and the purge does not reach it.
  await expect(dialog).toContainText("bleiben ihr erhalten");
});

test("AC-mailbox-6: the connect form says what happens before it asks for a password", async ({
  page,
}) => {
  await page.goto("/#/settings/connections");
  await page.getByRole("button", { name: "Konto verbinden" }).click();
  await page.getByTestId("connector-add-imap").getByRole("button").click();

  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("Margince liest dieses Postfach");
  await expect(dialog).toContainText(
    "Ein neues Postfach ist standardmäßig zurückgehalten",
  );
  // The notice asks for nothing: there is no checkbox and no field the server
  // records, and a mailbox connects with it unread. An acknowledgement here
  // would be a consent record nobody gave.
  await expect(dialog.getByRole("checkbox")).toHaveCount(0);
});
