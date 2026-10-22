import { expect, type Page, test } from "@playwright/test";
import { de } from "../src/i18n/de";
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

// A mailbox's posture sits under its account on Connections.
async function openMailboxPosture(page: Page) {
  const row = page.getByTestId("connector-gmail-mail-posture");
  await expect(row).toBeVisible();
  return row;
}

test("AC-mailbox-1: a mailbox says who may read it, and refuses shared without the opt-in", async ({
  page,
}) => {
  await page.goto("/#/settings/connections");
  const row = await openMailboxPosture(page);
  await expect(row).toBeVisible();

  // The stored posture, and the sentence saying what it does.
  await expect(row).toContainText("Sichtbarkeit der E-Mails");
  await expect(row).toContainText("Bis zur Prüfung zurückgehalten");
  await expect(row).toContainText(
    "Neue Nachrichten bleiben auf die Beteiligten beschränkt",
  );
  // The refusal rides on the ROW, not on the option label: a listbox option
  // ellipsises, and the half that gets cut is the reason a reader needs.
  await expect(row).toContainText(
    "muss ein Admin für dieses Unternehmen erlauben",
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
  const row = await openMailboxPosture(page);
  await row.getByRole("combobox").click();
  await page.getByRole("option", { name: "Immer zurückgehalten" }).click();

  // Only a narrowing asks about history, and it asks unticked: reaching back
  // into mail already brought in is the reader's call, never the default.
  const history = page.getByTestId("connector-gmail-mail-posture-history");
  await expect(history).toContainText(
    de["connectors.mailPosture.historyTitle"],
  );
  const alsoHistory = history.getByRole("checkbox", {
    name: de["connectors.mailPosture.historyApply"],
  });
  await expect(alsoHistory).not.toBeChecked();
  await alsoHistory.check();
  const posture = page.waitForRequest(
    (req) =>
      req.method() === "PUT" &&
      req.url().endsWith("/connectors/gmail/mail-posture"),
  );
  await page
    .getByRole("region", { name: de["saveBar.unsaved"] })
    .getByRole("button", { name: de["saveBar.save"], exact: true })
    .click();
  expect((await posture).postDataJSON()).toEqual({
    posture: "held",
    apply_to_history: true,
  });

  await expect(row).toContainText("Immer zurückgehalten");
  await expect(row).toContainText("unabhängig von der Einstufung");
});

test("AC-mailbox-3: the admin opt-in states what turning it on asserts", async ({
  page,
}) => {
  // The switch that binds the whole workspace is CHANGED on Capture rules;
  // the reader's own Connections page only states the rule it sets.
  await page.goto("/#/settings/capture");
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

// The senders list opens in a drawer from its summary row on Capture activity.
async function openSenders(page: Page) {
  await page.goto("/#/settings/capture-activity");
  await page
    .getByRole("button", {
      name: de["connectionOutcomes.viewNamed"].replace(
        "{name}",
        de["senders.title"],
      ),
    })
    .click();
  return page.getByRole("dialog", { name: de["senders.title"] });
}

// A sender's verbs sit in its row's menu, named by the address.
async function openRowMenu(page: Page, address: string) {
  await page
    .getByRole("button", {
      name: de["table.rowActions"].replace("{name}", address),
    })
    .click();
}

test("AC-mailbox-4: the Senders page shows every decision and takes an overrule", async ({
  page,
}) => {
  const senders = await openSenders(page);
  await expect(senders).toBeVisible();
  await expect(senders).toContainText(de["senders.kind.contact"]);
  await expect(senders).toContainText(de["senders.kind.newsletter"]);
  await expect(senders).toContainText(de["senders.kind.personal"]);

  const row = senders.locator("tr", { hasText: "news@substack.com" });
  await openRowMenu(page, "news@substack.com");
  await page
    .getByRole("button", { name: de["senders.markBusiness"], exact: true })
    .click();
  // An overruled row says whose answer stands, because the reader auditing this
  // list needs to tell their own decisions from the classifier's.
  await expect(row).toContainText(de["senders.byYou"]);
  await openRowMenu(page, "news@substack.com");
  await expect(
    page.getByRole("button", { name: de["senders.withdraw"], exact: true }),
  ).toBeVisible();
});

test("AC-mailbox-5: a sender kept out is told what that destroys", async ({
  page,
}) => {
  const senders = await openSenders(page);
  await expect(senders).toContainText("anne@hotmail.com");
  await openRowMenu(page, "anne@hotmail.com");
  await page
    .getByRole("button", { name: de["senders.keepOut"], exact: true })
    .click();

  const dialog = page.getByRole("dialog", {
    name: de["senders.keepOutTitle"],
  });
  await expect(dialog).toContainText("dauerhaft ausschließen");
  await expect(dialog).toContainText("vernichtet");
  // What SURVIVES is the half a reader cannot guess: a colleague's own import
  // of the same message is theirs, and the purge does not reach it.
  await expect(dialog).toContainText("bleiben bei diesem Teammitglied");
});

test("AC-mailbox-6: the connect form says what happens before it asks for a password", async ({
  page,
}) => {
  await page.goto("/#/settings/connections");
  await page.getByRole("button", { name: de["connectors.addOpen"] }).click();
  await page.getByRole("radio", { name: de["connectFlow.otherHost"] }).click();
  // Account, then what it brings in: the IMAP form is the sign-in step.
  const next = page.getByRole("button", {
    name: de["connectFlow.next"],
    exact: true,
  });
  await next.click();
  await next.click();

  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("Margince liest dieses Postfach");
  await expect(dialog).toContainText(
    "Ein neues Postfach wird standardmäßig zurückgehalten",
  );
  // The notice asks for nothing: there is no checkbox and no field the server
  // records, and a mailbox connects with it unread. An acknowledgement here
  // would be a consent record nobody gave.
  await expect(dialog.getByRole("checkbox")).toHaveCount(0);
});
