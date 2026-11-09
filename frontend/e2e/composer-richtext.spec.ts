// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import AxeBuilder from "@axe-core/playwright";
import { expect, type Locator, type Page, test } from "@playwright/test";
import { de } from "../src/i18n/de";
import { mockApi } from "./seed";

// The email body in a real engine: the unit engines have no execCommand, so
// neither the undo stack nor a created link exists there.

const DRAFT =
  "Hallo Anna,\n\nDanke für das Gespräch. Hier ist der Stand.\n\nPasst ein kurzer Anruf?";
const REWRITE = "Hallo Anna,\n\nKurzer Anruf diese Woche?";

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

async function openBody(page: Page): Promise<Locator> {
  await page.goto("/#/contacts/p-anna");
  await page
    .getByRole("button", { name: "E-Mail", exact: true })
    .first()
    .click();
  const body = page.getByRole("textbox", { name: de["compose.body"] });
  await expect(body).toBeVisible();
  return body;
}

/** Answers the first ask with DRAFT and every later one with REWRITE. */
async function modelDrafts(page: Page) {
  let asks = 0;
  await page.route("**/draft-email", async (route) => {
    asks += 1;
    await route.fulfill({
      json: {
        ai_generated: true,
        body: asks === 1 ? DRAFT : REWRITE,
        subject: "Zum Gespräch",
        to: ["anna@example.com"],
        reasoning: [],
        voice_degraded: false,
      },
    });
  });
}

test("a drafted body and its rewrite each come back with Undo", async ({
  page,
}) => {
  await modelDrafts(page);
  const body = await openBody(page);
  await page
    .getByRole("textbox", { name: de["compose.newIntent"] })
    .fill("Nachfassen");
  await page.getByRole("button", { name: de["compose.draftEmail"] }).click();
  await expect(body).toContainText("Danke für das Gespräch.");
  const drafted = await body.innerHTML();
  expect(drafted).toBe(
    "<p>Hallo Anna,</p><p>Danke für das Gespräch. Hier ist der Stand.</p><p>Passt ein kurzer Anruf?</p>",
  );
  await expect(body).toBeFocused();

  await page
    .getByRole("button", { name: de["compose.rewriteShorter"] })
    .click();
  await expect(body).toContainText("Kurzer Anruf diese Woche?");
  await body.focus();

  await page.keyboard.press("ControlOrMeta+z");
  await expect.poll(() => body.innerHTML()).toBe(drafted);
  await page.keyboard.press("ControlOrMeta+Shift+z");
  await expect(body).toContainText("Kurzer Anruf diese Woche?");
  await page.keyboard.press("ControlOrMeta+z");
  await page.keyboard.press("ControlOrMeta+z");
  await expect(body).toHaveText("");
});

const EARLIER_MAIL = {
  id: "act-mail-1",
  kind: "email",
  subject: "Angebot Q3",
  body: "Anbei das Angebot.",
  occurred_at: "2026-07-01T09:00:00Z",
  is_done: true,
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-07-01T09:00:00Z",
  updated_at: "2026-07-01T09:00:00Z",
  links: [{ entity_type: "contact", entity_id: "p-anna" }],
};

/** Anna's record holds one earlier email, so the composer offers its thread. */
async function earlierThread(page: Page) {
  await page.route(/\/v1\/activities\?.*kind=email/, (route) =>
    route.fulfill({
      json: { data: [EARLIER_MAIL], page: { has_more: false } },
    }),
  );
  await page.route(`**/v1/activities/${EARLIER_MAIL.id}`, (route) =>
    route.fulfill({ json: EARLIER_MAIL }),
  );
}

test("a switch of thread is out of Undo's reach", async ({ page }) => {
  await modelDrafts(page);
  await earlierThread(page);
  const body = await openBody(page);
  await page
    .getByRole("textbox", { name: de["compose.newIntent"] })
    .fill("Nachfassen");
  await page.getByRole("button", { name: de["compose.draftEmail"] }).click();
  await expect(body).toContainText("Danke für das Gespräch.");

  await page.getByRole("button", { name: /Thread fortsetzen/ }).click();
  await page
    .getByRole("button", { name: /Angebot Q3/ })
    .first()
    .click();
  await expect(body).toHaveText("");
  await body.focus();

  for (const press of [1, 2]) {
    await page.keyboard.press("ControlOrMeta+z");
    expect(await body.textContent(), `after undo ${press}`).toBe("");
  }
});

test("Cmd/Ctrl+K in the body asks for a link, not the palette", async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  const body = await openBody(page);
  await body.click();
  await page.keyboard.type("Zur Preisliste");
  await page.keyboard.press("Shift+Home");

  await page.keyboard.press("ControlOrMeta+k");
  const ask = page.getByRole("dialog", { name: de["richtext.linkAdd"] });
  await expect(ask).toBeVisible();
  await expect(page.locator('[role="dialog"][aria-modal="true"]')).toHaveCount(
    1,
  );
  const address = ask.getByRole("textbox", {
    name: de["richtext.linkAddress"],
  });
  await expect(address).toBeFocused();
  const axe = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
    .analyze();
  expect(
    axe.violations
      .filter((found) => ["serious", "critical"].includes(found.impact ?? ""))
      .map((found) => ({
        id: found.id,
        nodes: found.nodes.map(({ target, failureSummary }) => ({
          target,
          failureSummary,
        })),
      })),
  ).toEqual([]);

  await address.fill("javascript:alert(1)");
  await page.keyboard.press("Enter");
  await expect(address).toHaveAttribute("aria-invalid", "true");
  await expect(ask).toBeVisible();

  await address.fill("example.com/preise");
  await page.keyboard.press("Enter");
  await expect(ask).toBeHidden();
  await expect(body).toBeFocused();
  await expect(body.locator("a")).toHaveAttribute(
    "href",
    "https://example.com/preise",
  );
  await expect(body.locator("a")).toHaveText("Zur Preisliste");

  await page.keyboard.press("ControlOrMeta+k");
  const edit = page.getByRole("dialog", { name: de["richtext.linkEdit"] });
  await expect(edit).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(edit).toBeHidden();
  await expect(body).toBeFocused();
});

test("Cmd/Ctrl+Enter in the body is a press of Send", async ({ page }) => {
  const body = await openBody(page);
  await body.click();

  await page.keyboard.press("ControlOrMeta+Enter");

  await expect(page.getByText(de["compose.missingBody"])).toBeVisible();
  await expect(body).toHaveText("");
});
