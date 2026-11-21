// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, type Locator, type Page, test } from "@playwright/test";
import { loadLocale } from "../src/i18n/catalogs";
import { de } from "../src/i18n/de";
import { meFixture } from "../src/testing/mefixture";
import { expectNoAaViolations, settleAnimations } from "./a11y";
import {
  CALLED_CONTACT,
  loggedCall,
  loggedNote,
  PRIMARY_PHONE,
  taskFromCall,
  typedTask,
} from "./mockapi/calls";
import { expectShellRendered } from "./screens";
import { anna, mockApi, servePrincipal } from "./seed";
import { openSharedLinks, SHARED_LINK_ROWS } from "./sharedlinks";
import { pageOverflow } from "./waits";

test.beforeAll(async () => {
  await loadLocale("de");
});

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

test("the shared links drawer is worked by the keyboard alone", async ({
  page,
}) => {
  const trigger = await openSharedLinks(page);
  const drawer = page.getByRole("dialog", {
    name: de["analytics.share.listTitle"],
  });
  const closeLinks = drawer.getByRole("button", {
    name: de["analytics.share.revoke"],
  });

  // Focus moved in: onto the drawer itself, which opened while its list was
  // still loading, and from there Tab reaches the first link's Close link.
  const focusInDrawer = () =>
    page.evaluate(
      () => document.activeElement?.closest('[role="dialog"]') !== null,
    );
  expect(await focusInDrawer()).toBe(true);
  await page.keyboard.press("Tab");
  await expect(closeLinks.first()).toBeFocused();
  // Tab walks the other rows and the way out, then comes back round inside.
  for (let press = 0; press < SHARED_LINK_ROWS.length; press += 1) {
    await page.keyboard.press("Tab");
    expect(await focusInDrawer()).toBe(true);
  }
  await page.keyboard.press("Tab");
  await expect(closeLinks.first()).toBeFocused();

  // Close link opens its confirmation, and Escape there leaves the drawer.
  await page.keyboard.press("Enter");
  const confirm = page.getByRole("dialog", {
    name: de["analytics.share.closeTitle"],
  });
  await expect(confirm).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(confirm).toBeHidden();
  await expect(closeLinks.first()).toBeFocused();

  // Escape on the drawer closes it and hands focus back to its trigger.
  await page.keyboard.press("Escape");
  await expect(drawer).toBeHidden();
  await expect(trigger).toBeFocused();
});

// The contact header's access chip, open. Its panel is portalled over the page
// and carries the record's one radio group. Neither the closed-header sweeps
// above nor the chip's own unit suite judges it. This test checks the contrast
// of the sentence and switch, the group's name, and focus landing inside.
// The route answers the contact as shared and owned by the session's own user,
// who may write it: the case that draws the switch.
test.describe("the contact's access panel, open", () => {
  const schemes: readonly ("light" | "dark")[] = ["light", "dark"];
  for (const colorScheme of schemes) {
    test(`no AA violations with it open (${colorScheme})`, async ({ page }) => {
      await page.emulateMedia({ colorScheme });
      // A reader who may change it, so the switch is drawn and judged.
      await servePrincipal(
        page,
        meFixture({ allow: { contact: ["read", "update"] } }),
      );
      await page.route("**/contacts/p-anna/360", (route) =>
        route.fulfill({
          json: {
            as_of: "2026-06-20T09:00:00Z",
            contact: {
              ...anna,
              visibility: "workspace",
              owner_id: meFixture().user.id,
            },
            sections_omitted: [],
          },
        }),
      );
      await page.goto("/#/contacts/p-anna");
      await page.waitForLoadState("networkidle");
      await expectShellRendered(page);
      const title = new RegExp(de["recordAccess.contact.title"]);
      const chip = page.getByRole("button", { name: title });
      await chip.click();
      const panel = page.getByRole("region", { name: title });
      await expect(panel).toBeVisible();
      // A press carries focus into the panel, onto its first answer.
      await expect(
        panel.getByRole("radio", { name: de["recordAccess.option.owner"] }),
      ).toBeFocused();
      await expect(
        panel.getByRole("radio", { name: de["recordAccess.option.workspace"] }),
      ).toBeChecked();
      await settleAnimations(page);
      await expectNoAaViolations(
        page,
        `contacts/p-anna access (${colorScheme})`,
      );
      await page.keyboard.press("Escape");
      await expect(panel).toBeHidden();
      await expect(chip).toBeFocused();
    });
  }
});

// An archived contact opens on its row: the server refuses its 360, brief and
// guard. The seeded contact is live, so no sweep above reaches this page.
test.describe("an archived contact opens read-only", () => {
  const schemes: readonly ("light" | "dark")[] = ["light", "dark"];
  for (const colorScheme of schemes) {
    test(`row, reason and history, with no AA violations (${colorScheme})`, async ({
      page,
    }) => {
      await page.emulateMedia({ colorScheme });
      await page.route(/\/v1\/contacts\/p-anna$/, (route) =>
        route.fulfill({
          json: {
            ...anna,
            writable: false,
            version: 2,
            archived_at: "2026-06-20T09:00:00Z",
          },
        }),
      );
      await page.route(
        /\/v1\/contacts\/p-anna\/(360|brief|consent\/guard)$/,
        (route) =>
          route.fulfill({
            status: 404,
            json: { status: 404, code: "not_found", title: "Not Found" },
          }),
      );
      await page.route(/\/v1\/records\/contact\/p-anna\/history/, (route) =>
        route.fulfill({
          json: {
            data: [
              {
                id: "h-archive",
                actor_type: "human",
                actor_id: meFixture().user.id,
                actor_name: "Demo Admin",
                action: "archive",
                occurred_at: "2026-06-20T09:00:00Z",
                summary: "Demo Admin archived the record",
                undoable: { undoable: true },
              },
            ],
            page: { has_more: false, next_cursor: null },
          },
        }),
      );
      await page.goto("/#/contacts/p-anna");
      await expect(
        page.getByRole("heading", {
          level: 1,
          name: "Anna Weber",
          exact: true,
        }),
      ).toBeVisible();
      await expect(
        page.getByText(de["contact.rail.archivedReadOnly"]),
      ).toBeVisible();
      await expect(
        page.getByRole("button", { name: de["history.undo.action"] }),
      ).toBeVisible();
      await expect(page.getByText(de["contact.page.notOpened"])).toHaveCount(0);
      await settleAnimations(page);
      await expectNoAaViolations(
        page,
        `contacts/p-anna archived (${colorScheme})`,
      );
    });
  }
});

test.describe("calling a contact", () => {
  const tel = `tel:${PRIMARY_PHONE}`;
  const callVerb = (scope: Page | Locator) =>
    scope.getByRole("link", { name: de["record.verb.call"], exact: true });

  test("the number in the details dials the primary of two numbers", async ({
    page,
  }) => {
    await page.goto(`/#/contacts/${CALLED_CONTACT}`);
    await expectShellRendered(page);
    await expect(
      page.locator(".record-aside").locator(`a[href="${tel}"]`),
    ).toBeVisible();
  });

  test("a contact with no number offers no Call at all", async ({ page }) => {
    await page.goto("/#/contacts/p-anna");
    await expectShellRendered(page);
    await expect(
      page.getByRole("heading", { level: 1, name: "Anna Weber", exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: de["contact.action.call"], exact: true }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("button", {
        name: de["contact.action.call"],
        exact: true,
      }),
    ).toHaveCount(0);
  });

  test("on the Overview a logged call offers Call back and a note does not", async ({
    page,
  }) => {
    await page.goto(`/#/contacts/${CALLED_CONTACT}`);
    await expectShellRendered(page);
    const call = page
      .getByRole("listitem")
      .filter({ hasText: loggedCall.subject });
    const note = page
      .getByRole("listitem")
      .filter({ hasText: loggedNote.subject });
    await expect(callVerb(call)).toHaveAttribute("href", tel);
    await expect(note).toBeVisible();
    await expect(callVerb(note)).toHaveCount(0);
    await settleAnimations(page);
    await expectNoAaViolations(page, `contacts/${CALLED_CONTACT}`);
  });

  test("on History a logged call offers Call back and a note does not", async ({
    page,
  }) => {
    await page.goto(`/#/contacts/${CALLED_CONTACT}/timeline`);
    await expectShellRendered(page);
    const call = page
      .getByRole("listitem")
      .filter({ hasText: loggedCall.subject });
    const note = page
      .getByRole("listitem")
      .filter({ hasText: loggedNote.subject });
    await expect(callVerb(call)).toHaveAttribute("href", tel);
    await expect(note).toBeVisible();
    await expect(callVerb(note)).toHaveCount(0);
    await settleAnimations(page);
    await expectNoAaViolations(page, `contacts/${CALLED_CONTACT}/timeline`);
  });

  test.describe("a task on the contact, opened from Home", () => {
    test.beforeEach(async ({ page }) => {
      await mockApi(page, { callTasks: true });
    });

    async function openTask(page: Page, subject: string) {
      await page.goto("/#/home");
      await expectShellRendered(page);
      await page
        .getByRole("button", { name: new RegExp(`^${de["brief.queue.show"]}`) })
        .click();
      const opener = page.getByRole("button", { name: subject, exact: true });
      await opener.click();
      const dialog = page.getByRole("dialog", { name: subject });
      await expect(dialog).toBeVisible();
      return { opener, dialog };
    }

    test("a reply to a call offers its number, its chats, Copy number and Call, in tab order", async ({
      page,
    }) => {
      const { opener, dialog } = await openTask(page, taskFromCall.subject);
      const number = dialog.locator(`a[href="${tel}"]`).first();
      const copy = dialog.getByRole("button", {
        name: de["task.next.copyNumber"],
        exact: true,
      });
      const call = callVerb(dialog);
      await expect(number).toBeVisible();
      await expect(copy).toBeVisible();
      await expect(call).toHaveAttribute("href", tel);
      // Mail is the other way, offered after the call.
      await expect(
        dialog.getByRole("button", {
          name: de["task.next.draftEmail"],
          exact: true,
        }),
      ).toBeVisible();
      await settleAnimations(page);
      await expectNoAaViolations(page, "home (task to call open)");

      await dialog.locator("a[href], button:not([disabled])").first().focus();
      for (let press = 0; press < 12; press += 1) {
        if (await number.evaluate((node) => node === document.activeElement)) {
          break;
        }
        await page.keyboard.press("Tab");
      }
      await expect(number).toBeFocused();
      await page.keyboard.press("Tab");
      await expect(
        dialog.getByRole("button", { name: de["callAction.chats"] }),
      ).toBeFocused();
      await page.keyboard.press("Tab");
      await expect(copy).toBeFocused();
      await page.keyboard.press("Tab");
      await expect(call).toBeFocused();

      await page.keyboard.press("Escape");
      await expect(dialog).toBeHidden();
      await expect(opener).toBeFocused();
    });

    test("a task typed by hand reaches out: Draft email, Call and Open contact", async ({
      page,
    }) => {
      const { dialog } = await openTask(page, typedTask.subject);
      await expect(
        dialog.getByRole("link", { name: de["task.next.openContact"] }),
      ).toBeVisible();
      await expect(
        dialog.getByRole("button", {
          name: de["task.next.draftEmail"],
          exact: true,
        }),
      ).toBeVisible();
      await expect(callVerb(dialog)).toHaveAttribute("href", tel);
    });
  });
});

// Authorize access is reached from an AI tool's redirect, never from the rail,
// so neither route list above sweeps it.
test.describe("Authorize access on the sign-in surface", () => {
  const consentAddress = (extra: Record<string, string>) =>
    `/#/oauth-consent?${new URLSearchParams({
      response_type: "code",
      client_id: "https://claude.ai/oauth/mcp-oauth-client-metadata",
      redirect_uri: "https://claude.ai/api/mcp/auth_callback",
      scope: "read",
      code_challenge: "x",
      code_challenge_method: "S256",
      resource: "https://margince.example/mcp",
      state: "s",
      ...extra,
    }).toString()}`;

  test.beforeEach(async ({ page }) => {
    await page.route("**/v1/oauth/consent-request**", (route) =>
      route.fulfill({
        json: {
          client_name: "Claude",
          offline: true,
          scopes: ["read", "draft", "write", "send", "enrich"],
          areas: ["business", "compliance", "own_account"],
        },
      }),
    );
  });

  for (const colorScheme of ["light", "dark"] as const) {
    for (const width of [390, 1440]) {
      test(`the prompt meets AA at ${width}px in ${colorScheme}`, async ({
        page,
      }) => {
        await page.emulateMedia({ colorScheme });
        await page.setViewportSize({ width, height: 900 });
        await page.goto(consentAddress({ consent: "nonce" }));
        await expect(
          page.getByRole("heading", { level: 1, name: de["consent.title"] }),
        ).toBeVisible();
        await expect(page.locator(".auth-wordmark")).toBeVisible();
        await expect(
          page.getByRole("button", {
            name: de["consent.approve"],
            exact: true,
          }),
        ).toBeVisible();
        await settleAnimations(page);
        await expectNoAaViolations(page, `oauth-consent (${colorScheme})`);
        expect(await pageOverflow(page)).toEqual([]);
      });
    }
  }

  test("Deny sits before Authorize on the end edge, and a phone stacks them in the same order", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.goto(consentAddress({ consent: "nonce" }));
    const deny = page.getByRole("button", { name: de["consent.deny"] });
    const authorize = page.getByRole("button", {
      name: de["consent.approve"],
      exact: true,
    });
    const wide = {
      deny: await deny.boundingBox(),
      authorize: await authorize.boundingBox(),
    };
    expect(wide.deny?.x).toBeLessThan(wide.authorize?.x ?? 0);
    expect(wide.deny?.y).toBe(wide.authorize?.y);
    const contentEnd = await page
      .locator(".oauthconsent-pane")
      .evaluate((pane) => {
        const style = getComputedStyle(pane);
        return (
          pane.getBoundingClientRect().right -
          Number.parseFloat(style.borderRightWidth) -
          Number.parseFloat(style.paddingRight)
        );
      });
    expect(
      Math.abs(
        (wide.authorize?.x ?? 0) + (wide.authorize?.width ?? 0) - contentEnd,
      ),
    ).toBeLessThanOrEqual(1);

    await page.setViewportSize({ width: 390, height: 844 });
    const phone = {
      deny: await deny.boundingBox(),
      authorize: await authorize.boundingBox(),
    };
    expect(phone.deny?.y).toBeLessThan(phone.authorize?.y ?? 0);
    expect(phone.authorize?.width).toBe(phone.deny?.width);
  });

  test("the wordmark stays clear of the pane at a tablet width", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 700, height: 800 });
    await page.goto(consentAddress({ consent: "nonce" }));
    await expect(
      page.getByRole("button", { name: de["consent.approve"], exact: true }),
    ).toBeVisible();
    const mark = await page.locator(".auth-wordmark").boundingBox();
    const pane = await page.locator(".oauthconsent-pane").boundingBox();
    expect(pane?.y ?? 0).toBeGreaterThanOrEqual(
      (mark?.y ?? Number.POSITIVE_INFINITY) + (mark?.height ?? 0),
    );
  });

  test("a laptop keeps the pane at the top, beside the wordmark", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(consentAddress({ consent: "nonce" }));
    await expect(
      page.getByRole("radio", { name: de["workArea.mode.all"] }),
    ).toBeVisible();
    const mark = await page.locator(".auth-wordmark").boundingBox();
    const pane = await page.locator(".oauthconsent-pane").boundingBox();
    expect(pane?.y ?? Number.POSITIVE_INFINITY).toBeLessThan(mark?.y ?? 0);
  });

  for (const error of ["invalid_request", "method_off", "stale_consent"]) {
    test(`the ${error} refusal meets AA on the same surface`, async ({
      page,
    }) => {
      await page.setViewportSize({ width: 390, height: 844 });
      await page.goto(consentAddress({ error }));
      await expect(page.getByRole("heading", { level: 1 })).toHaveCount(1);
      await expect(page.locator(".auth-surface .auth-wordmark")).toBeVisible();
      await settleAnimations(page);
      await expectNoAaViolations(page, `oauth-consent ${error}`);
      expect(await pageOverflow(page)).toEqual([]);
    });
  }
});
