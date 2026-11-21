// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, type Page, test } from "@playwright/test";
import { loadLocale } from "../src/i18n/catalogs";
import { de } from "../src/i18n/de";
import { en } from "../src/i18n/en";
import { expectNoAaViolations, settleAnimations } from "./a11y";
import { expectShellRendered } from "./screens";
import { mockApi } from "./seed";
import { pageOverflow } from "./waits";

test.beforeAll(async () => {
  await loadLocale("de");
});

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

/**
 * The one account affordance, at the foot of the sidebar.
 *
 * Scoped to the top bar's trailing cluster, because its place is part of the
 * contract: the end of the session strip, opposite the trail.
 * A bare name lookup would also pass for a second account control elsewhere.
 * The name is a substring because the accessible name also carries who is
 * signed in (WCAG 2.5.3).
 */
function accountTrigger(page: Page) {
  return page.locator(".topbar .topbar-trail").getByRole("button", {
    name: "Konto",
  });
}

// The translated destinations are an acceptance expectation, independent of
// NAV_GROUPS: importing production's list would only prove it renders itself.
// Home owns the work queue; the queue opens in a drawer rather than occupying
// a second sidebar destination.
const primaryDestinations = [
  "Home",
  "Kontakte",
  "Unternehmen",
  "Leads",
  "Deals",
  "Projekte",
  "Filter und Ansichten",
  "Analytics",
  "Mein Fortschritt",
  "Umsatzziele",
];

// settleAnimations' own case. No other test here touches a WAAPI-driven control
// before sweeping, so they pass whether or not an in-flight animation settles.
//
// The design system's Select animates its ~140ms open through the Web Animations
// API. It reads prefers-reduced-motion at render time, so emulating after a
// click cannot stop that animation.
// The case runs on a bare element so it tests the harness, not whichever
// screen carries a Select today.
test("the settle waits for an animation already in flight to land", async ({
  page,
}) => {
  await page.goto("/#/home");
  // Anchored before the animation starts, because boot has finite animations of
  // its own (the Core's fallback dress fades out over 300ms).
  // A settle during boot would report those instead of the case's animation.
  await expect(page.locator("nav.rail .navlevel a.navitem")).toHaveCount(
    primaryDestinations.length,
  );
  await settleAnimations(page);

  await page.evaluate(() => {
    // What Select does: a finite WAAPI animation on a real element, started
    // before reduced motion is emulated. `animation: none` does not govern it.
    document.body.animate([{ opacity: 1 }, { opacity: 0.5 }], {
      duration: 140,
      iterations: 1,
    });
  });

  // Without the landing wait this fails mid-flight, and axe would read a frame
  // of the animation.
  await settleAnimations(page);
});

test("AC-shell-1: the rail renders the primary destinations in order", async ({
  page,
}) => {
  await page.goto("/#/home");
  // evaluateAll never waits. Anchor on the rendered count first, or the read
  // races the auth splash and sees an empty rail.
  // Scoped to the shown level: its rows are the destinations.
  // The foot's Settings door shares the `.navitem` class but is not one of them.
  await expect(page.locator("nav.rail .navlevel a.navitem")).toHaveCount(
    primaryDestinations.length,
  );
  const labels = await page
    .locator("nav.rail .navlevel a.navitem")
    .evaluateAll((links) =>
      links.map((link) => link.getAttribute("aria-label")),
    );
  expect(labels).toEqual(primaryDestinations);
});

test("AC-shell-2: exactly one rail item is active and tracks the route", async ({
  page,
}) => {
  await page.goto("/#/deals");
  await expect(page.locator("nav.rail a.navitem.active")).toHaveCount(1);
  await expect(page.locator("nav.rail a.navitem.active")).toHaveAttribute(
    "aria-label",
    "Deals",
  );
  await page.locator('nav.rail a[aria-label="Analytics"]').click();
  await expect(page.locator("nav.rail a.navitem.active")).toHaveAttribute(
    "aria-label",
    "Analytics",
  );
  await expect(page.locator("nav.rail a.navitem.active")).toHaveCount(1);
});

// One page-level heading per railed page, and it names the page. On a list, a
// report or a settings surface the shell's page head mints it.
// On a record the record's own identity block is the heading, and the head
// prints the trail back instead.
// Two h1s with the same name would give a screen reader two page titles.
test("AC-shell-1k: one h1 per railed page, and on a record it is the record's own", async ({
  page,
}) => {
  await page.goto("/#/contacts");
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("Kontakte");

  await page.goto("/#/contacts/p-anna");
  const heading = page.getByRole("heading", { level: 1 });
  await expect(heading).toHaveCount(1);
  await expect(heading).toHaveText("Anna Weber");
  await expect(page.locator(".record-split-head h1")).toHaveText("Anna Weber");
  // The trail back to the list stands in the top bar, outside the document the
  // reader is reading.
  await expect(page.locator(".topbar .crumbs a").last()).toHaveText("Kontakte");
  await expect(page.locator('.topbar [aria-current="page"]')).toHaveText(
    "Anna Weber",
  );

  // An id segment that names no record is named in words, never as its slug.
  // The h1 names the privacy surface; "Settings" belongs to the rail heading.
  // "privacy" is a route slug no reader should see.
  await page.goto("/#/settings/privacy");
  const settingsHeading = page.getByRole("heading", { level: 1 });
  await expect(settingsHeading).toHaveCount(1);
  await expect(settingsHeading).toHaveText("Datenschutz und Aufbewahrung");
  await expect(page.locator(".rail .navheading").first()).toHaveText(
    "Einstellungen",
  );
  await expect(page.locator("main")).not.toContainText("privacy");
});

test("AC-shell-3/4/5: ⌘K opens focused+empty, filters, Enter navigates", async ({
  page,
}) => {
  await openForKeys(page, "home");
  await page.keyboard.press("ControlOrMeta+k");
  const input = page.getByRole("searchbox", { name: "Befehlspalette" });
  await expect(input).toBeFocused();
  // "Pipeline" is the row's old label, kept as an alias.
  // Typing it must still land on the screen for readers who know the old name.
  await input.fill("Pipeline");
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/#\/deals$/);
});

// A dialog makes the rest of the app unreachable, the palette with it. Raised
// over one, Escape closed the dialog underneath and left the palette standing.
test("AC-shell-3: ⌘K does nothing while a dialog is up", async ({ page }) => {
  await page.goto("/#/deals/new");
  const form = page.getByRole("dialog", { name: "Neuer Deal" });
  await expect(form).toBeVisible();
  await page.keyboard.press("ControlOrMeta+k");
  await page.keyboard.press("Escape");
  await expect(form).toHaveCount(0);
  await expect(
    page.getByRole("searchbox", { name: "Befehlspalette" }),
  ).toHaveCount(0);
});

test("AC-shell-7: the top bar's search opens the palette, and Enter on a row closes it for good", async ({
  page,
}) => {
  await page.goto("/#/home");
  const input = page.getByRole("searchbox", { name: "Befehlspalette" });
  const topbar = page.locator(".topbar");
  // One search affordance in the product, and it is the centre of the session
  // strip. It is a button, never a field: the palette owns the query, and this
  // criterion forbids a second input taking one here.
  await expect(topbar.locator("input")).toHaveCount(0);
  await topbar.locator(".topbar-search").click();
  await expect(input).toBeVisible();
  // Search is not a destination, so the links AC-shell-1 counts stay unchanged.
  await expect(page.locator("nav.rail .navlevel a.navitem")).toHaveCount(
    primaryDestinations.length,
  );
  await input.fill("Pipeline");
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/#\/deals$/);
  await expect(input).toHaveCount(0);
});

// The account menu carries what belongs to the signed-in user, not the page:
// the door into Settings, the appearance, and the way out.
// It is the only settings door, because the sidebar carries destinations only.
//
// Appearance sits here too, because a reader changes it to see the current
// screen differently. Going to a settings tab would mean leaving that screen.
test("the account menu holds the settings door, the appearance choice and the way out", async ({
  page,
}) => {
  await page.goto("/#/home");
  await accountTrigger(page).click();
  const menu = page.locator(".topbar [role='menu']").first();
  await expect(
    menu.getByRole("menuitem", { name: "Einstellungen", exact: true }),
  ).toHaveAttribute("href", "/#/settings");
  await expect(
    menu.getByRole("menuitem", { name: de["scheduling.myLink"] }),
  ).toHaveAttribute("href", "/#/settings/meetings");
  // The head is the third link: the seat's own page, named by who it is for.
  await expect(
    menu.getByRole("menuitem", {
      name: new RegExp(
        `^${de["shell.accountSettingsAria"].replace("{who}", ".+")}$`,
      ),
    }),
  ).toHaveAttribute("href", "/#/settings/account");
  await expect(menu.locator("a[href]")).toHaveCount(3);
  await expect(menu.getByRole("menuitem", { name: "Abmelden" })).toBeVisible();

  // Appearance is a submenu, not a control sitting open in the menu: three
  // choices spelled out flat would out-weigh the two destinations either side.
  const theme = menu.getByRole("menuitem", { name: "Design" });
  await expect(theme).toHaveAttribute("aria-expanded", "false");
  await theme.click();
  const choices = page.locator(".accountsub");
  await expect(choices.getByRole("radio")).toHaveCount(3);
  await choices.getByRole("radio", { name: "Dunkel" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await expect(choices.getByRole("radio", { name: "Dunkel" })).toBeChecked();
});

// Help and feedback is a submenu of real menu items, and the shortcuts it opens
// are a dialog. Both are swept by axe in the state a reader reaches them in.
test("help and feedback and the keyboard shortcuts meet AA", async ({
  page,
}) => {
  await page.goto("/#/home");
  await page.waitForLoadState("networkidle");
  await accountTrigger(page).click();
  const help = page
    .locator(".topbar [role='menu']")
    .first()
    .getByRole("menuitem", { name: de["shell.helpMenu"] });
  await help.press("ArrowRight");
  const submenu = page.getByRole("menu", { name: de["shell.helpMenu"] });
  await expect(submenu.getByRole("menuitem").first()).toBeFocused();
  await settleAnimations(page);
  await expectNoAaViolations(page, "home — help and feedback");

  await submenu
    .getByRole("menuitem", { name: de["shell.help.shortcuts"] })
    .click();
  const dialog = page.getByRole("dialog", { name: de["shell.help.shortcuts"] });
  await expect(dialog).toBeVisible();
  await settleAnimations(page);
  await expectNoAaViolations(page, "home — keyboard shortcuts");
  await page.keyboard.press("Escape");
  await expect(accountTrigger(page)).toBeFocused();
});

test("About Margince meets AA", async ({ page }) => {
  await page.goto("/#/home");
  await page.waitForLoadState("networkidle");
  await accountTrigger(page).click();
  await page
    .locator(".topbar [role='menu']")
    .first()
    .getByRole("menuitem", { name: de["shell.helpMenu"] })
    .press("ArrowRight");
  await page
    .getByRole("menu", { name: de["shell.helpMenu"] })
    .getByRole("menuitem", { name: de["about.title"] })
    .click();
  const dialog = page.getByRole("dialog", { name: de["about.title"] });
  await expect(dialog).toBeVisible();
  await settleAnimations(page);
  await expectNoAaViolations(page, "home — about margince");
  await page.keyboard.press("Escape");
  await expect(accountTrigger(page)).toBeFocused();
});

// Headless Chromium never offers to install, so the test makes the offer itself.
for (const colorScheme of ["light", "dark"] as const) {
  test.describe(`the install hint on Home at 390px in ${colorScheme}`, () => {
    test.use({
      viewport: { width: 390, height: 844 },
      hasTouch: true,
      isMobile: true,
      colorScheme,
    });

    test("meets AA, sits in the tab order, and Not now hands focus to the page", async ({
      page,
    }) => {
      await page.addInitScript(() => {
        const earlier = new Date(Date.now() - 2 * 86_400_000);
        window.localStorage.setItem(
          "margince.installHint",
          JSON.stringify({ firstDay: earlier.toISOString().slice(0, 10) }),
        );
      });
      await page.goto("/#/home");
      await expectShellRendered(page);
      await page.evaluate(() => {
        const offer = Object.assign(
          new Event("beforeinstallprompt", { cancelable: true }),
          {
            platforms: ["web"],
            userChoice: Promise.resolve({
              outcome: "dismissed",
              platform: "web",
            }),
            prompt: async () => undefined,
          },
        );
        window.dispatchEvent(offer);
      });
      const hint = page.locator(".appbanner", {
        hasText: de["installHint.title"],
      });
      await expect(hint).toBeVisible();
      await settleAnimations(page);
      await expectNoAaViolations(
        page,
        `home — install hint (390px, ${colorScheme})`,
      );
      expect(await pageOverflow(page)).toEqual([]);

      const install = hint.getByRole("button", {
        name: de["settings.installAppAction"],
      });
      const notNow = hint.getByRole("button", {
        name: de["installHint.notNow"],
      });
      await notNow.focus();
      await page.keyboard.press("Tab");
      await expect(install).toBeFocused();
      await page.keyboard.press("Shift+Tab");
      await expect(notNow).toBeFocused();
      await page.keyboard.press("Enter");
      await expect(hint).toHaveCount(0);
      expect(
        await page.evaluate(
          () =>
            document.activeElement !== null &&
            document.activeElement !== document.body,
        ),
      ).toBe(true);
    });
  });
}

// A page whose heading has drawn, with focus out of every field, so a key
// press reaches the app's shortcuts.
async function openForKeys(page: Page, hash: string) {
  await page.goto(`/#/${hash}`);
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  await page.evaluate(() => {
    const focused = document.activeElement;
    if (focused instanceof HTMLElement) {
      focused.blur();
    }
  });
}

test("keyboard shortcuts: ? opens the list of shortcuts, and Esc closes it", async ({
  page,
}) => {
  await openForKeys(page, "home");
  await page.keyboard.press("?");
  const dialog = page.getByRole("dialog", { name: de["shell.help.shortcuts"] });
  await expect(dialog).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
});

test("keyboard shortcuts: / opens search", async ({ page }) => {
  await openForKeys(page, "home");
  await page.keyboard.press("/");
  await expect(
    page.getByRole("searchbox", { name: "Befehlspalette" }),
  ).toBeFocused();
});

test("keyboard shortcuts: G then C goes to Contacts, G then D to Deals", async ({
  page,
}) => {
  await openForKeys(page, "home");
  await page.keyboard.press("g");
  await page.keyboard.press("c");
  // The list may already have spelled its opening sort into the address.
  await expect(page).toHaveURL(/#\/contacts(\?[^/]*)?$/);

  await openForKeys(page, "home");
  await page.keyboard.press("g");
  await page.keyboard.press("d");
  await expect(page).toHaveURL(/#\/deals$/);
});

test("keyboard shortcuts: keys typed into a field stay in the field", async ({
  page,
}) => {
  await openForKeys(page, "settings/audit");
  const field = page.getByRole("textbox", { name: de["settings.auditActor"] });
  await field.click();
  await field.pressSequentially("gc?");
  await expect(field).toHaveValue("gc?");
  await expect(page).toHaveURL(/#\/settings\/audit/);
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

test("keyboard shortcuts: with single keys off, ? does nothing and ⌘K still opens search", async ({
  page,
}) => {
  await openForKeys(page, "settings/account");
  const singleKeys = page.getByRole("switch", {
    name: de["account.singleKeyShortcuts"],
  });
  await expect(singleKeys).toBeChecked();
  await singleKeys.click();
  await expect(singleKeys).not.toBeChecked();

  await openForKeys(page, "home");
  await page.keyboard.press("?");
  // ⌘K lands after ?, so the palette on screen proves ? was already heard.
  await page.keyboard.press("ControlOrMeta+k");
  await expect(
    page.getByRole("searchbox", { name: "Befehlspalette" }),
  ).toBeFocused();
  await expect(
    page.getByRole("dialog", { name: de["shell.help.shortcuts"] }),
  ).toHaveCount(0);
});

test("keyboard shortcuts: N on a contact opens Log activity", async ({
  page,
}) => {
  await openForKeys(page, "contacts/p-anna");
  await expect(page.locator('button[aria-keyshortcuts="n"]')).toBeEnabled();
  await page.keyboard.press("n");
  await expect(
    page.getByRole("dialog", { name: de["log.title"] }),
  ).toBeVisible();
});

test("the locale switch flips the chrome DE↔EN", async ({ page }) => {
  await page.goto("/#/home");
  await expect(page.locator('nav.rail a[aria-label="Kontakte"]')).toBeVisible();
  // The language is a preference, not a destination, so it lives on
  // Settings → Account.
  // Three locales ship, so the control is a list: a toggle cannot say where the
  // next click lands.
  await page.goto("/#/settings/account");
  // A card title on the page, so the check below reads chrome the language
  // control does not draw itself.
  await expect(
    page.getByRole("heading", { name: de["account.signInCard"] }),
  ).toBeVisible();
  await page.getByRole("combobox", { name: "Sprache" }).click();
  await page.getByRole("option", { name: "English" }).click();
  // The pick waits in the page's save bar; the language turns once it is saved.
  await page
    .getByRole("region", { name: de["saveBar.unsaved"] })
    .getByRole("button", { name: de["saveBar.save"], exact: true })
    .click();
  // The whole page follows the choice, not only the control, because every word
  // on it comes from the catalog that just changed.
  await expect(
    page.getByRole("heading", { name: en["account.signInCard"] }),
  ).toBeVisible();
  await expect(page.getByRole("combobox", { name: "Language" })).toBeVisible();
});

// Appearance is chosen from Settings and from the account menu.
// It is the setting a reader changes most often, from wherever they are.
//
// The language control beside it has to still be there, or an account card that
// failed to render entirely would pass this.
test("Settings → Account offers language and appearance", async ({ page }) => {
  await page.goto("/#/settings/account");
  await expect(
    page.getByRole("heading", { name: de["account.browserCard"] }),
  ).toBeVisible();
  await expect(page.getByRole("combobox", { name: "Sprache" })).toBeVisible();
  await expect(page.getByRole("group", { name: "Darstellung" })).toBeVisible();
});

/**
 * Where the language control's focus contract is actually testable.
 *
 * The jsdom suite (`src/design-system/select.test.tsx`) asserts the same
 * restoration, but jsdom keeps `document.activeElement` on a detached node.
 * There, a list that closed without handing focus back still reads as focused.
 * Only a browser drops focus to `<body>`, and these two tests catch that.
 * This is the only in-app language control, so its reader is least able to
 * recover from losing focus.
 */
test("Escape closes the language list back onto its control", async ({
  page,
}) => {
  await page.goto("/#/settings/account");
  const trigger = page.getByRole("combobox", { name: "Sprache" });
  await trigger.click();
  const list = page.getByRole("listbox");
  await expect(list).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(list).toHaveCount(0);
  await expect(trigger).toBeFocused();
});

test("Tab closes the language list and carries on", async ({ page }) => {
  await page.goto("/#/settings/account");
  const trigger = page.getByRole("combobox", { name: "Sprache" });
  await trigger.click();
  const list = page.getByRole("listbox");
  await expect(list).toBeVisible();
  await page.keyboard.press("Tab");
  await expect(list).toHaveCount(0);
  // Tab means the reader is leaving, so focus lands on the next control.
  // It must land neither on the trigger nor nowhere.
  await expect(trigger).not.toBeFocused();
  await expect(page.locator("body")).not.toBeFocused();
});
