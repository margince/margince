import { expect, test } from "@playwright/test";
import { MOCK_MINTED_KEY } from "./projectmock";
import { mockApi } from "./seed";

/**
 * A project from birth to close, the way a rep works it: created from the
 * Projects list, attached to a deal on the deal's own form, moved into
 * delivery by the deal's win rather than by hand, accumulating the timeline
 * rows filed under it, advanced by hand, and closed with the reason the
 * close demands. German chrome, as the app renders it (playwright.config.ts
 * pins de-DE).
 *
 * The mock remembers every write and enforces If-Match (e2e/projectmock.ts),
 * so each step below reads back the state the previous step wrote.
 */

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

async function choose(
  page: import("@playwright/test").Page,
  combobox: import("@playwright/test").Locator,
  option: string | RegExp,
) {
  await combobox.click();
  await page.getByRole("option", { name: option }).click();
}

test("a project is created, a deal is attached, the win starts delivery, the timeline fills, and the close records its reason", async ({
  page,
}) => {
  // 1. Create. The list carries the seeded project; the new one joins it.
  await page.goto("/#/projects");
  await expect(
    page.getByRole("link", { name: /^Flottenumbau Brandt/ }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Neues Projekt" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("textbox", { name: "Projektname" }).fill("Brandt ERP");
  await choose(
    page,
    dialog.getByRole("combobox", { name: "Unternehmen" }),
    "Brandt Automotive GmbH",
  );
  await dialog.getByRole("button", { name: "Anlegen" }).click();

  // The create lands on the project's own page, born in Initiative with the
  // birth row in its history and nothing filed under it yet.
  await expect(page).toHaveURL(/#\/projects\/pr-new-1$/);
  // The dialog never asked for a key — the server mints one — so the page must
  // SHOW the key it was given. This is the half a create form could not prove
  // once the field was removed: without it, a create that came back with no key
  // at all would look exactly like a success.
  // Scoped to the key chip — the element whose hover text explains what THIS
  // key is for — and asserted to read the key alone: a bare text match would
  // pass on the explanation, which renders whether or not the project actually
  // got a key.
  const keyChip = page.getByTitle(`[${MOCK_MINTED_KEY}]`);
  await expect(keyChip).toBeVisible();
  await expect(keyChip).toHaveText(MOCK_MINTED_KEY);
  await expect(
    page.getByRole("heading", { level: 1, name: "Brandt ERP" }),
  ).toBeVisible();
  const phase = page.getByRole("group", { name: "Phase" });
  // The current phase is the one step rendered as text, not as a button.
  const current = phase.locator('[aria-current="step"]');
  await expect(current).toHaveText("Initiative");
  // The phase history lives in the details column, which is open on arrival,
  // so the birth row is on screen without a press.
  await expect(page.getByText("Gestartet in Initiative")).toBeVisible();
  await expect(
    page.getByText("Unter diesem Projekt ist noch nichts abgelegt", {
      exact: false,
    }),
  ).toBeVisible();

  // 2. Attach through the company/project group in the deal's Details pane.
  await page.goto("/#/deals/d-fleet");
  await page.getByRole("button", { name: "Firma ändern", exact: true }).click();
  await choose(
    page,
    page.getByRole("combobox", { name: "Projekt", exact: true }),
    "Brandt ERP",
  );
  const patched = page.waitForRequest(
    (request) =>
      request.url().endsWith("/v1/deals/d-fleet") &&
      request.method() === "PATCH",
  );
  await page.getByRole("button", { name: "Speichern", exact: true }).click();
  expect((await patched).postDataJSON().project_id).toBe("pr-new-1");
  // The deal header now carries the project as a chip linking to its page.
  await expect(page.getByTestId("deal-project")).toHaveText("Brandt ERP");

  // 3. Log something on the deal, then win it. The note is filed under the
  // deal, not the project — the project timeline picks it up in step 5.
  //
  // Through the head's own verb: the deal page carries no standing log form,
  // because a form open on every read asks for a note nobody came to write.
  await page
    .getByRole("button", { name: "Aktivität erfassen", exact: true })
    .click();
  // Inside the dialog, and exactly: the head's own verb is still on the page
  // behind it, and "Erfassen" matches "Aktivität erfassen" as a substring.
  const logDialog = page.getByRole("dialog");
  await logDialog.getByLabel(/^Betreff/).fill("Kickoff mit Brandt IT");
  await logDialog
    .getByRole("button", { name: "Erfassen", exact: true })
    .click();
  // The deal's chronology is its own tab now, not a block under the overview:
  // what was said about the deal and what was changed on it are one order of
  // events, read in one place. Every arrival at the deal opens on the overview,
  // so each read of the chronology asks for the tab first.
  const timeline = page.getByRole("region", { name: "Verlauf" });
  const openDealHistory = async () => {
    // The write lands, the dialog closes, and the reads it invalidated come
    // back — and a record remounted by one of those refetches opens on its
    // first tab again. So the tab is asked for until the chronology is the
    // thing on screen, rather than once into a page still settling.
    await expect(logDialog).toBeHidden();
    await expect(async () => {
      await page
        .getByTestId("record-tabs")
        .getByRole("button", { name: "Verlauf", exact: true })
        .click();
      await expect(timeline).toBeVisible({ timeout: 2000 });
    }).toPass();
  };
  await openDealHistory();
  await expect(timeline.getByText("Kickoff mit Brandt IT")).toBeVisible();

  await page
    .getByRole("group", { name: "Phase" })
    .getByRole("button", { name: "Won" })
    .click();
  await expect(page.getByText("Nach Won verschieben?")).toBeVisible();
  // The first confirm is refused: no contract is on the deal, so the server
  // answers win_evidence_required and the dialog stays open asking how it was
  // won. Only the answered second confirm is the win.
  const refused = page.waitForResponse(
    (response) =>
      response.url().endsWith("/v1/deals/d-fleet/advance") &&
      response.request().method() === "POST",
  );
  await page.getByRole("button", { name: "Bestätigen" }).click();
  expect((await refused).status()).toBe(422);
  await choose(
    page,
    dialog.getByRole("combobox", { name: "Wie wurde er gewonnen?" }),
    "Per Bestellung",
  );
  const won = page.waitForRequest(
    (request) =>
      request.url().endsWith("/v1/deals/d-fleet/advance") &&
      request.method() === "POST",
  );
  await page.getByRole("button", { name: "Bestätigen" }).click();
  expect((await won).postDataJSON().won_without_contract_reason).toBe(
    "purchase_order",
  );

  // 4. The win moved the project into delivery — nobody pressed a phase.
  await page.getByTestId("deal-project").click();
  await expect(page).toHaveURL(/#\/projects\/pr-new-1$/);
  await expect(current).toHaveText("In Umsetzung");
  await expect(page.getByText("Initiative → In Umsetzung")).toBeVisible();
  // And the deal is listed on the project with its won status.
  await expect(
    page.getByRole("button", { name: "Fleet retrofit" }),
  ).toBeVisible();

  // 5. The timeline accumulates what is filed under the project: relink the
  // deal's note to the project and it appears here with the coverage count.
  await page.goto("/#/deals/d-fleet");
  await openDealHistory();
  await timeline.getByRole("button", { name: "Neu verknüpfen" }).click();
  await dialog
    .getByRole("searchbox", {
      name: "Kontakt, Firma, Deal, Lead oder Projekt suchen",
    })
    .fill("Brandt ERP");
  await dialog.getByRole("button", { name: "Brandt ERP" }).click();
  await dialog.getByRole("button", { name: "Neu verknüpfen" }).click();
  await expect(dialog).toBeHidden();

  // A hash change keeps the document, and with it the 360 read cached a
  // moment ago; reload so the page reads the relinked state afresh.
  await page.goto("/#/projects/pr-new-1");
  await page.reload();
  // The timeline row IS the proof the relink landed. The coverage strip that
  // used to be asserted here — "{attributed} zugeordnet · {awaiting} warten
  // auf Entscheidung · …" — was removed by #2408 so the page answers one
  // question, and project360.test.tsx now asserts its absence twice. An e2e
  // assertion on copy the product deliberately dropped tests nothing about the
  // relink and fails for a reason that has nothing to do with it.
  await expect(timeline.getByText("Kickoff mit Brandt IT")).toBeVisible();

  // 6. Advance by hand: a non-closing move takes an optional reason and
  // lands in the history with it.
  await phase.getByRole("button", { name: "Im Vertrieb" }).click();
  await expect(
    page.getByRole("heading", { name: "Wechsel zu Im Vertrieb" }),
  ).toBeVisible();
  await page
    .getByTestId("project-advance-reason")
    .fill("Erweiterung wird neu verhandelt.");
  await page.getByRole("button", { name: "Wechseln" }).click();
  await expect(current).toHaveText("Im Vertrieb");
  await expect(page.getByText("In Umsetzung → Im Vertrieb")).toBeVisible();
  await expect(
    page.getByText("Erweiterung wird neu verhandelt.", { exact: false }),
  ).toBeVisible();

  // 7. Close with a reason. The close button stays disabled until a reason
  // is typed, and the reason is what the history shows afterwards.
  await phase.getByRole("button", { name: "Abgeschlossen" }).click();
  await expect(
    page.getByRole("heading", { name: "Wechsel zu Abgeschlossen" }),
  ).toBeVisible();
  const close = page.getByRole("button", { name: "Projekt abschließen" });
  await expect(close).toBeDisabled();
  await page
    .getByTestId("project-advance-reason")
    .fill("Go-live abgenommen, Hypercare an den Support übergeben.");
  const closed = page.waitForRequest(
    (request) =>
      request.url().endsWith("/v1/projects/pr-new-1/advance") &&
      request.method() === "POST",
  );
  await close.click();
  const body = (await closed).postDataJSON();
  expect(body.to_phase).toBe("closed");
  expect(body.reason).toBe(
    "Go-live abgenommen, Hypercare an den Support übergeben.",
  );
  await expect(current).toHaveText("Abgeschlossen");
  await expect(page.getByText("Im Vertrieb → Abgeschlossen")).toBeVisible();
  await expect(
    page.getByText("Go-live abgenommen, Hypercare an den Support übergeben.", {
      exact: false,
    }),
  ).toBeVisible();
});
