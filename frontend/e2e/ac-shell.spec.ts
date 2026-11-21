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
 * Scoped to the top bar's trailing cluster rather than found by name alone:
 * WHERE it is is half of what the restructure promises — the contact sits at the
 * end of the session strip, opposite the trail that says where you are — and a
 * second account control appearing anywhere else would still satisfy a bare
 * name lookup. The name itself is deliberately a substring: the trigger carries
 * who is signed in into its accessible name (WCAG 2.5.3), so it reads
 * "<contact> — Konto".
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

// settleAnimations' own case, because the gap it closes is invisible to every
// other test in this file: none of them touches a WAAPI-driven control before
// sweeping, so all of them pass whether or not an in-flight animation is
// settled.
//
// The design system's Select drives its ~140ms open through the Web Animations
// API and reads prefers-reduced-motion AT RENDER TIME, so a test that clicks
// the control and then emulates has started something the media query cannot
// call off. Reproduced here directly rather than through a screen, so the case
// is about the harness and not about whichever surface happens to carry a
// Select today.
test("the settle waits for an animation already in flight to land", async ({
  page,
}) => {
  await page.goto("/#/home");
  // Anchored before the animation starts, because the page's OWN boot carries
  // finite work — the Core's fallback dress transitions its opacity out over
  // 300ms once the canvas takes over — and a settle that ran during boot would
  // be reporting that rather than the animation this case is about.
  await expect(page.locator("nav.rail .navlevel a.navitem")).toHaveCount(
    primaryDestinations.length,
  );
  await settleAnimations(page);

  await page.evaluate(() => {
    // What Select does: a finite WAAPI animation on a real element, started
    // BEFORE reduced motion is emulated. `animation: none` does not govern it.
    document.body.animate([{ opacity: 1 }, { opacity: 0.5 }], {
      duration: 140,
      iterations: 1,
    });
  });

  // Without the landing wait this fails on that animation, mid-flight, which is
  // the flake #5298 recorded — and axe would have read the frame it was at.
  await settleAnimations(page);
});

test("AC-shell-1: the rail renders the primary destinations in order", async ({
  page,
}) => {
  await page.goto("/#/home");
  // evaluateAll never waits — anchor on the rendered count first, or the
  // read races the auth splash and sees an empty rail.
  // Scoped to the level the panel is showing: the DESTINATIONS are its rows,
  // while the foot's Settings door rides the same `.navitem` geometry without
  // being one of them.
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
// report or a settings surface the shell's page head mints it. On a RECORD the
// record's own identity block IS it, and the head prints the trail back
// instead — the name at heading level twice would leave a screen reader
// choosing between two page titles for the same record.
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
  // The trail that leads back to the list stands in the top bar, where it is
  // true of the page rather than part of the document the reader is reading.
  await expect(page.locator(".topbar .crumbs a").last()).toHaveText("Kontakte");
  await expect(page.locator('.topbar [aria-current="page"]')).toHaveText(
    "Anna Weber",
  );

  // An id segment that names no record is the screen's own state, so it is named
  // in WORDS and never as the slug it is addressed by: #/settings/privacy is the
  // privacy surface, and the heading over the level's first group carries "Settings" — the
  // page said that word twice while naming the surface never. "privacy" itself
  // is a route slug no reader should ever be shown.
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
  // "Pipeline" is the word this row used to print, kept as its alias — typing it
  // still has to land on the screen, or a relabeled destination becomes
  // unreachable for everyone who knows it by its old name.
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
  // strip. A BUTTON, never a field: the palette owns the query, and a second
  // input taking one here is exactly what this criterion forbids.
  await expect(topbar.locator("input")).toHaveCount(0);
  await topbar.locator(".topbar-search").click();
  await expect(input).toBeVisible();
  // And it is not a destination of its own — the links AC-shell-1 counts are
  // unchanged by search leaving the sidebar.
  await expect(page.locator("nav.rail .navlevel a.navitem")).toHaveCount(
    primaryDestinations.length,
  );
  await input.fill("Pipeline");
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/#\/deals$/);
  await expect(input).toHaveCount(0);
});

// The account menu carries what belongs to the CONTACT rather than to the page:
// the one door into Settings, the appearance they read in, and the way out. It
// is the product's only settings door now — the sidebar carries destinations and
// nothing else — so a second one appearing anywhere is the regression, not a
// second row inside here.
//
// Appearance sits here rather than on a settings page because it is the one
// preference a reader changes to see the thing they are looking at differently,
// and walking to a settings tab to do it means leaving what they were reading.
test("features/10 §7: the account menu holds the settings door, the appearance choice and the way out", async ({
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

test("features/10 §7: the locale switch flips the chrome DE↔EN", async ({
  page,
}) => {
  await page.goto("/#/home");
  await expect(page.locator('nav.rail a[aria-label="Kontakte"]')).toBeVisible();
  // The language is a preference of this contact rather than a destination, so it
  // lives on Settings → Account; the account block at the sidebar foot carries
  // the three places it can take you and nothing that changes a setting. Three
  // locales ship, so the control is a list rather than a toggle — a toggle
  // cannot say where the next click lands.
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
  // The surface around the control follows the choice, not just the control's
  // own face: every word on it is rendered from the catalog that just changed.
  await expect(
    page.getByRole("heading", { name: en["account.signInCard"] }),
  ).toBeVisible();
  await expect(page.getByRole("combobox", { name: "Language" })).toBeVisible();
});

// Appearance is chosen from Settings AND from the account menu — one setting
// with two doors, because it is the setting a reader changes most often and from
// wherever they happen to be standing.
//
// The language control beside it has to still be there, or an account card that
// failed to render entirely would pass this.
test("features/10 §7: Settings → Account offers language and appearance", async ({
  page,
}) => {
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
 * restoration, but jsdom does not move `document.activeElement` when the focused
 * node is detached — so a list that closed WITHOUT handing focus back would still
 * read as focused there. Only a browser blanks the selection to `<body>`, which
 * is exactly the stranding these two tests exist to catch: this is the only
 * in-app control for changing language, so its reader is the one least able to
 * recover from being dropped at the top of the document.
 */
test("features/10 §7: Escape closes the language list back onto its control", async ({
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

test("features/10 §7: Tab closes the language list and carries on", async ({
  page,
}) => {
  await page.goto("/#/settings/account");
  const trigger = page.getByRole("combobox", { name: "Sprache" });
  await trigger.click();
  const list = page.getByRole("listbox");
  await expect(list).toBeVisible();
  await page.keyboard.press("Tab");
  await expect(list).toHaveCount(0);
  // Tab says the reader is leaving, so focus must land on the next control —
  // neither back on the trigger they just left nor nowhere at all.
  await expect(trigger).not.toBeFocused();
  await expect(page.locator("body")).not.toBeFocused();
});
