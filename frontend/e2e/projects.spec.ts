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

// One visit to a phase in the project's phase history card; each move files
// the phase it enters as a row of its own.
function visit(page: import("@playwright/test").Page, phase: string) {
  return page
    .getByRole("region", { name: "Phasenverlauf" })
    .getByRole("listitem")
    .filter({ hasText: phase });
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
  // The company is searched by name, not chosen from a preloaded list.
  await dialog.getByRole("searchbox", { name: "Unternehmen" }).fill("Brandt");
  await dialog.getByRole("button", { name: /Brandt Automotive GmbH/ }).click();
  await dialog.getByRole("button", { name: "Erstellen" }).click();

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
  // The current phase is the one step rendered as text, not as a button; it
  // also carries the days spent in it.
  const current = phase.locator('[aria-current="step"]');
  await expect(current).toContainText("Initiative");
  // The phase history is a card on the overview. The birth row (the first
  // phase, filed by whoever created the project) is on screen without a press.
  await expect(page.getByText("Phasenverlauf")).toBeVisible();
  await expect(
    page
      .getByRole("region", { name: "Phasenverlauf" })
      .getByText(/Lars Brandt hat das Projekt angelegt/),
  ).toBeVisible();
  // A new project has filed no documents, and its standing says so.
  await expect(
    page.locator(".standing-sheet").getByText("Keine angehängt"),
  ).toBeVisible();

  // 2. Attach through the company/project group in the deal's Details drawer,
  // which the head's overflow menu opens.
  await page.goto("/#/deals/d-fleet");
  await page.getByRole("button", { name: "Weitere Aktionen" }).click();
  await page.getByRole("button", { name: "Details bearbeiten" }).click();
  await page
    .getByRole("button", { name: "Unternehmen ändern", exact: true })
    .click();
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
  // The saved drawer stays open for more edits, so close it before the page
  // behind it is used again. Escape while the save is in flight would ask to
  // discard it, so wait for the group to leave its edit state first.
  await expect(
    page.getByRole("button", { name: "Speichern", exact: true }),
  ).toBeHidden();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toBeHidden();
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
  // The write itself is the claim here — that the note is filed UNDER THE DEAL
  // — so it is read off the request rather than off a rendering of it. Reading
  // it back needs the chronology tab, and pressing that on the page the write
  // just invalidated races the refetch that remounts the record; step 5 opens
  // the tab from a fresh arrival, where there is no such race, and the project
  // timeline there is what proves the note travelled.
  const filed = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      response.url().includes("/activities") &&
      response.ok(),
  );
  await logDialog
    .getByRole("button", { name: "Erfassen", exact: true })
    .click();
  expect((await filed).request().postDataJSON().subject).toBe(
    "Kickoff mit Brandt IT",
  );
  // The deal's chronology is its own tab now, not a block under the overview:
  // what was said about the deal and what was changed on it are one order of
  // events, read in one place. Every arrival at the deal opens on the overview,
  // so each read of the chronology asks for the tab first.
  const timeline = page.getByRole("region", { name: "Verlauf" });
  // The write lands, the dialog closes, and the reads it invalidated come back
  // — and a record remounted by one of those refetches opens on its first tab
  // again. So what the chronology is being read FOR is checked inside the
  // retry, not after it: the tab standing open for one moment is not the same
  // claim as the row being there to read, and a remount between the two puts
  // the panel back behind the overview with the row still in the document.
  const historyTab = () =>
    page.getByTestId("record-tabs").getByRole("button", { name: /^Verlauf/ });
  const onDealHistory = async (read: () => Promise<unknown>) => {
    await expect(logDialog).toBeHidden();
    await expect(async () => {
      // At rest before pressing: the strip is sticky, so a scrolled record
      // pins it over the head and the press lands on whatever is pinned above
      // the tab. The shell scrolls an inner container rather than the window.
      await page.evaluate(() => {
        for (const box of document.querySelectorAll(".scroll")) {
          box.scrollTop = 0;
        }
      });
      await historyTab().click();
      await read();
    }).toPass();
  };

  await page
    .getByRole("group", { name: "Phase" })
    .getByRole("button", { name: "Won" })
    .click();
  await expect(page.getByText("In die Phase Won verschieben?")).toBeVisible();
  // No contract is on the deal, so the dialog asks how it was won on opening.
  // The one Confirm that follows is the win.
  await choose(
    page,
    dialog.getByRole("combobox", { name: "Wie wurde der Deal gewonnen?" }),
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
  await expect(current).toContainText("In Umsetzung");
  await expect(visit(page, "In Umsetzung")).toBeVisible();
  // And the deal is listed on the project with its won status.
  await expect(
    page.getByRole("button", { name: "Fleet retrofit" }),
  ).toBeVisible();

  // 5. The timeline accumulates what is filed under the project: relink the
  // deal's note to the project and it appears here with the coverage count.
  await page.goto("/#/deals/d-fleet");
  // Relink sits in the row's "More actions" menu, which is portalled out of
  // the row, so the item is looked up on the page once the menu is open.
  const noteVerbs = timeline
    .locator("li", { hasText: "Kickoff mit Brandt IT" })
    .first()
    .getByRole("button", { name: "Weitere Aktionen" });
  await onDealHistory(() => expect(noteVerbs).toBeVisible({ timeout: 2000 }));
  await noteVerbs.click();
  await page.getByRole("button", { name: "Neu verknüpfen" }).click();
  await dialog
    .getByRole("searchbox", {
      name: "Kontakte, Unternehmen, Deals, Leads oder Projekte suchen",
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
  await expect(
    page.locator(".standing-timeline-title", {
      hasText: "Kickoff mit Brandt IT",
    }),
  ).toBeVisible();

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
  await expect(current).toContainText("Im Vertrieb");
  await expect(visit(page, "Im Vertrieb")).toBeVisible();
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
  await expect(current).toContainText("Abgeschlossen");
  await expect(visit(page, "Abgeschlossen")).toBeVisible();
  await expect(
    visit(page, "Abgeschlossen").getByText(
      "Go-live abgenommen, Hypercare an den Support übergeben.",
      { exact: false },
    ),
  ).toBeVisible();
});
