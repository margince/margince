// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, type Page, test } from "@playwright/test";
import { loadLocale } from "../src/i18n/catalogs";
import { de } from "../src/i18n/de";
import { viewMenuName } from "../src/testing/listview.names";
import { expectNoAaViolations, settleAnimations } from "./a11y";
import {
  CORE_SCREENS,
  expectSettingsViewLanded,
  expectShellRendered,
} from "./screens";
import { mockApi } from "./seed";
import { openSharedLinks } from "./sharedlinks";

test.beforeAll(async () => {
  await loadLocale("de");
});

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

// The same panel under the dark palette. At this width the stylesheet paints
// fills of its own here (the well's ground and the notch's). tokens.css
// publishes dark as its own set of values, so a pass in light says nothing about it.
test.describe("WCAG 2.2 AA (axe), the agent's panel at 390px in dark", () => {
  test.use({ viewport: { width: 390, height: 844 }, colorScheme: "dark" });

  test("no AA violations with the agent's panel open", async ({ page }) => {
    await page.goto("/#/home");
    await page.waitForLoadState("networkidle");
    await expectShellRendered(page);
    await page.getByRole("button", { name: "Agentenbereich öffnen" }).click();
    await expect(page.locator(".arpanel")).toBeVisible();
    await settleAnimations(page);
    await expectNoAaViolations(page, "brief — the agent's panel (390px, dark)");
  });
});

// The ignition is reached only by binding a model on a fresh installation, so
// no route in the sweeps above ever draws it.
test.describe("WCAG 2.2 AA (axe), the cold start's ignition at 390px", () => {
  test.use({ viewport: { width: 390, height: 844 } });
  const schemes: readonly ("light" | "dark")[] = ["light", "dark"];
  for (const colorScheme of schemes) {
    test(`no AA violations once the model is bound (${colorScheme})`, async ({
      page,
    }) => {
      await page.emulateMedia({ colorScheme, reducedMotion: "reduce" });
      await mockApi(page, { journey: "unconfigured" });
      await page.clock.install();
      await page.goto("/#/onboarding");
      await page.getByLabel(de["firstRun.ai.key"]).fill("AIza-not-a-real-key");
      await page.getByRole("button", { name: de["firstRun.continue"] }).click();
      // By tag rather than role, so a list that lost its role still reaches
      // the scan and axe's own listitem rule is what refuses it.
      const can = page.locator("ul").filter({
        hasText: de["firstRun.ignite.canNow"],
      });
      await expect(can.locator("li")).toHaveCount(3);
      // Past the Core's last timed beat, so the scan reads the settled room.
      await page.clock.runFor(4000);
      await settleAnimations(page);
      await expectNoAaViolations(
        page,
        `onboarding — the ignition (390px, ${colorScheme})`,
      );
    });
  }
});

// A phone at 200% text holds a question taller than the room. The board is the
// one box that scrolls, so a keyboard reader has to be able to land on it.
test.describe("the cold start's board on a phone at 200% text", () => {
  test.use({ viewport: { width: 320, height: 568 } });

  // The board a keyboard lands on, named after its question, inside the
  // window, and holding nothing wider than itself that it would clip.
  async function expectBoardReachable(page: Page) {
    const question = await page.locator(".ob-stage-title").innerText();
    const board = page.getByRole("region", { name: question });
    await expect(board).toHaveClass(/ob-stage-board/);
    await expect(board).toHaveAttribute("tabindex", "0");
    const box = await board.evaluate((element) => {
      const { left, right } = element.getBoundingClientRect();
      return {
        left,
        right,
        window: document.documentElement.clientWidth,
        held: element.scrollWidth,
        shown: element.clientWidth,
      };
    });
    expect(box.left).toBeGreaterThanOrEqual(0);
    expect(box.right).toBeLessThanOrEqual(box.window);
    expect(box.held).toBeLessThanOrEqual(box.shown);
  }

  test("is a tab stop named after its question, inside the window", async ({
    page,
  }) => {
    await mockApi(page, { journey: "unconfigured" });
    await page.goto("/#/onboarding");
    await page.getByLabel(de["firstRun.ai.key"]).fill("AIza-not-a-real-key");
    await page.evaluate(() => {
      document.documentElement.style.fontSize = "200%";
    });
    await expectBoardReachable(page);
    // A room this short against its text gives up the Core's row. It is folded,
    // never unmounted, or its WebGL loop restarts on the way back.
    await expect(page.locator(".ob-stage-core")).toHaveCount(1);
    await expect(page.locator(".ob-stage-core")).toBeHidden();
    // The ignition is the widest step: the sealed badge and the capability
    // lines are the lines that outgrew the board.
    await page.getByRole("button", { name: de["firstRun.continue"] }).click();
    await expect(page.locator(".ob-ig-can li")).toHaveCount(3);
    await expectBoardReachable(page);
  });

  // The same window at the default text size keeps the Core: the fold is
  // measured against the reader's text, not against the phone.
  test("keeps the Core at the default text size", async ({ page }) => {
    await mockApi(page, { journey: "unconfigured" });
    await page.goto("/#/onboarding");
    await expect(page.locator(".ob-stage-title")).toBeVisible();
    await expect(page.locator(".ob-stage-core")).toBeVisible();
  });
});

// The addresses this product answers that carry a view as well as a
// destination: a narrowed list, a record's tab, the decided queue, one report.
// They are swept apart from CORE_SCREENS because a reader reaches each as a
// state, not as a nav entry. Each draws chrome the bare screen does not: the
// chips the filter fills in, the tab strip's pressed control, the pager.
const ADDRESSED_VIEWS = [
  "companies?q=brandt&sort=name",
  "companies/o-brandt/tasks",
  "analytics/forecast",
  "analytics/pipeline",
  "analytics/questions",
  // The three record headers whose verbs are icon-only. The name a sighted
  // reader gets on hover is not the name axe checks. So this sweep checks that
  // every square carries an accessible name, and that a header of squares
  // still meets AA on its own ground. A glyph with no name fails silently, and
  // it fails for the readers who cannot see the glyph.
  "contacts/p-anna",
  "deals/d-fleet",
  // The results screen with hits on it: groups, a type filter and links per
  // row. A reader lands on it by searching, not through the nav, so no other
  // sweep reaches it.
  "search/brandt",
  "projects/pr-fleet",
  // A focused filter page names itself, so the shell's heading stands down:
  // the one-h1 sweep is what holds the two from both printing one.
  "filters/companies",
];

// The same sweep under the dark palette. The scheme is pinned, because an
// unpinned run inherits whichever palette the machine reports.
//
// It is a second block, not a second tag on the block above, because the two
// palettes are different subjects. tokens.css publishes dark as its own set of
// values: the accent lightens to the ADR-0040 mandate and the surfaces darken.
// So a screen can pass in one palette and fail in the other with the same markup.
test.describe("WCAG 2.2 AA (axe), dark palette", () => {
  test.use({ colorScheme: "dark" });

  // Both lists, not just the nav entries. A record header drawn as icon-only
  // squares sits on the accent-tinted grounds this palette lightens.
  // Accent-coloured text on an accent-tinted fill fails here and passes in light.
  for (const screen of [...CORE_SCREENS, ...ADDRESSED_VIEWS]) {
    test(`no AA violations on #/${screen} in dark`, async ({ page }) => {
      await page.goto(`/#/${screen}`);
      await page.waitForLoadState("networkidle");
      await expectShellRendered(page);
      await expectSettingsViewLanded(page, screen);
      await settleAnimations(page);
      await expectNoAaViolations(page, `${screen} (dark)`);
    });
  }
});

// One h1 per route, which axe does not answer: `page-has-heading-one` asks
// whether there is at least one. Two page titles in one document leave a
// reader who navigates by heading unable to tell which is the page.
//
// Across both route lists, not only the three routes AC-shell-1k names, so a
// new route inherits the check. That test keeps its own cases: it asserts
// which heading wins on a record, a claim about the identity block.
test.describe("one page heading per route", () => {
  for (const screen of [...CORE_SCREENS, ...ADDRESSED_VIEWS]) {
    test(`#/${screen} has exactly one h1`, async ({ page }) => {
      await page.goto(`/#/${screen}`);
      await page.waitForLoadState("networkidle");
      // A page that threw during render has almost no accessibility tree, so
      // "no h1" would read as a heading defect when it is a crash. Same guard,
      // and the same reason, as the axe sweeps below.
      await expectShellRendered(page);
      await expectSettingsViewLanded(page, screen);
      await expect(page.getByRole("heading", { level: 1 })).toHaveCount(1);
    });
  }
});

test.describe("WCAG 2.2 AA (axe)", () => {
  for (const screen of CORE_SCREENS) {
    test(`no AA violations on #/${screen}`, async ({ page }) => {
      await page.goto(`/#/${screen}`);
      await page.waitForLoadState("networkidle");
      // Before measuring anything: a page that threw during render has almost
      // no accessibility tree to fault, so a crash would read as a clean sweep.
      await expectShellRendered(page);
      await expectSettingsViewLanded(page, screen);
      await settleAnimations(page);
      await expectNoAaViolations(page, screen);
    });
  }

  // The results screen with hits on it, in the light palette. ADDRESSED_VIEWS
  // above reaches it in dark alone. This half judges an accent-coloured link
  // on a light card.
  test("no AA violations on the search results screen", async ({ page }) => {
    await page.goto("/#/search/brandt");
    await page.waitForLoadState("networkidle");
    await expectShellRendered(page);
    await expect(page.getByRole("heading", { name: "Produkte" })).toBeVisible();
    await settleAnimations(page);
    await expectNoAaViolations(page, "search/brandt");
  });

  // The narrowed screen is a different arrangement: one group and a pressed
  // pill. When the narrowing finds nothing, it shows an empty state under a
  // filter row that has to stay operable.
  test("no AA violations on a narrowed search", async ({ page }) => {
    await page.goto("/#/search/brandt");
    await page.waitForLoadState("networkidle");
    await expectShellRendered(page);
    await page.getByRole("button", { name: "Produkte", exact: true }).click();
    await expect(
      page.getByRole("button", { name: "Produkte", exact: true }),
    ).toHaveAttribute("aria-pressed", "true");
    await settleAnimations(page);
    await expectNoAaViolations(page, "search/brandt (narrowed to products)");
  });

  // The ⌘K palette is a modal, and every other axe pass in this file measures
  // a page with the dialog closed. A reader reaches it from any screen.
  test("no AA violations with the command palette open", async ({ page }) => {
    await page.goto("/#/home");
    await page.waitForLoadState("networkidle");
    await expectShellRendered(page);
    await page.keyboard.press("ControlOrMeta+k");
    await expect(
      page.getByRole("dialog", { name: "Befehlspalette" }),
    ).toBeVisible();
    await page.getByRole("combobox", { name: "Befehlspalette" }).fill("brandt");
    // Wait on the hits, not on a duration: the live arm is what adds the rows
    // this sweep exists to judge.
    // Anchored: a contact found through the company carries its name too. The
    // avatar's printed initials lead the name (WCAG 2.5.3).
    await expect(
      page.getByRole("option", { name: /^(BA\s*)?Brandt Automotive$/ }),
    ).toBeVisible();
    await settleAnimations(page);
    await expectNoAaViolations(page, "brief — the command palette open");
  });

  // The View panel is a popover on a desktop and a full-screen sheet at 390px.
  // The closed toolbar sweeps only its button, never what it opens.
  const VIEW_SIZES = [
    { name: "desktop", width: 1280, height: 800 },
    { name: "390px", width: 390, height: 844 },
  ];
  for (const screen of ["contacts", "companies", "deals", "leads"]) {
    for (const size of VIEW_SIZES) {
      for (const step of [null, "sort", "filters"]) {
        const opened = step
          ? `the View panel's ${step} step open`
          : "the View panel open";
        test(`no AA violations on #/${screen} with ${opened} at ${size.name}`, async ({
          page,
        }) => {
          await page.setViewportSize(size);
          await page.goto(`/#/${screen}`);
          await page.waitForLoadState("networkidle");
          await expectShellRendered(page);
          await page
            .getByRole("button", { name: viewMenuName("de") })
            .first()
            .click();
          const panel = page.locator(".lt-vpanel");
          await expect(panel).toBeVisible();
          if (step) {
            await panel.locator(`[data-view-row="${step}"]`).click();
            await expect(panel.locator(".lt-vstep")).toBeVisible();
          }
          // Off the panel first: a row under the pointer would start its hover
          // fade after the settle, and the scan would read it mid-fade.
          await page.mouse.move(0, 0);
          await settleAnimations(page);
          await expectNoAaViolations(
            page,
            `${screen} — View panel${step ? ` (${step})` : ""} at ${size.name}`,
          );
        });
      }
    }
  }

  // A report row's explain drawer, with the permission notice in it: the
  // closed page sweeps only the icon-only trigger, never what it opens.
  test("no AA violations with a report row's explain drawer open", async ({
    page,
  }) => {
    await page.goto("/#/analytics/pipeline");
    await page.waitForLoadState("networkidle");
    await expectShellRendered(page);
    await page
      .getByRole("button", {
        name: de["explain.cell"].replace("{figure}", "Qualify"),
      })
      .click();
    const drawer = page.getByRole("dialog");
    await expect(drawer.getByText(de["explain.excluded_one"])).toBeVisible();
    await settleAnimations(page);
    await expectNoAaViolations(page, "analytics — a row's explain drawer open");
  });

  // The Questions section with a question asked and one of its rows opened.
  // The builder's rows of pickers, the answer table and the drawer it opens
  // are three surfaces the closed tab never draws. The engine's three routes
  // are stubbed here because the shared mock answers no analytics question.
  test("no AA violations on an asked question with its drawer open", async ({
    page,
  }) => {
    const json = (body: unknown) => ({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(body),
    });
    await page.route("**/analytics/schema", (route) =>
      route.fulfill(
        json({
          version: "v1",
          entities: [
            {
              name: "deals-by-stage",
              group_by: ["currency", "stage_id", "status"],
              measures: ["amount_minor"],
            },
          ],
        }),
      ),
    );
    await page.route("**/analytics/query", (route) =>
      route.fulfill(
        json({
          columns: ["count"],
          rows: [{ count: 12, _withheld: false }],
          withheld: false,
          total_safe: true,
          schema_version: "v1",
        }),
      ),
    );
    await page.route("**/analytics/explain", (route) =>
      route.fulfill(
        json({
          columns: ["id", "status"],
          rows: [{ id: "d-fleet", status: "open" }],
          withheld: false,
          truncated: false,
        }),
      ),
    );
    await page.goto("/#/analytics/questions");
    await page.waitForLoadState("networkidle");
    await expectShellRendered(page);
    await page
      .getByRole("combobox", { name: de["analytics.q.population"] })
      .click();
    await page
      .getByRole("option", { name: de["analytics.reportDealsByStage"] })
      .click();
    await page.getByRole("button", { name: de["analytics.q.ask"] }).click();
    await page
      .getByRole("button", {
        name: de["explain.cell"].replace(
          "{figure}",
          de["analytics.q.allRecords"],
        ),
      })
      .click();
    await expect(page.getByRole("dialog")).toBeVisible();
    await settleAnimations(page);
    await expectNoAaViolations(
      page,
      "analytics — an asked question's drawer open",
    );
  });

  // The forecast's Shared links drawer with rows in it, and the confirmation
  // that a row's Close link stacks over it. The closed page sweeps only the trigger.
  // The mock's admin holds no forecast grant, so this reader is given one.
  test("no AA violations with the shared links drawer open", async ({
    page,
  }) => {
    await openSharedLinks(page);
    const drawer = page.getByRole("dialog", {
      name: de["analytics.share.listTitle"],
    });
    await settleAnimations(page);
    await expectNoAaViolations(page, "analytics/forecast — shared links open");

    // By the keyboard: a pointer left over the pressed row would start its
    // hover transition back as the confirmation covers it, mid-sweep.
    await drawer
      .getByRole("button", { name: de["analytics.share.revoke"] })
      .first()
      .focus();
    await page.keyboard.press("Enter");
    await expect(
      page.getByRole("dialog", { name: de["analytics.share.closeTitle"] }),
    ).toBeVisible();
    await settleAnimations(page);
    await expectNoAaViolations(page, "analytics/forecast — close link asked");
  });

  // A list header folds its verbs into one overflow menu below 1100px
  // (design-system/listsurface.tsx). The buttons are then inside a disclosure,
  // so the trigger owes a name and the panel owes a relationship to it. The
  // wide sweep above cannot see any of that.
  test("no AA violations on a list header folded into its menu", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1024, height: 800 });
    await page.goto("/#/contacts");
    await page.waitForLoadState("networkidle");
    await expectShellRendered(page);
    await settleAnimations(page);
    // Opened, because a closed disclosure hides the controls this exists to
    // check. `OverflowMenu` keeps them mounted and `hidden`, and axe skips a
    // hidden subtree as a reader does.
    //
    // German, like every other name this suite reaches for: the app under test
    // runs in it.
    await page.getByRole("button", { name: "Weitere Aktionen" }).click();
    await expect(
      page.getByRole("button", { name: "Neuer Kontakt" }),
    ).toBeVisible();
    await expectNoAaViolations(page, "contacts (folded header)");
  });

  // An address whose whole meaning is "the create form is open", at the width
  // where the verb that opens it has folded into a menu.
  //
  // A functional case, not an axe one. It sits here because this is the only
  // block that drives the folded arrangement. `CreateAction` reads its opening
  // state once, at mount, so the menu must mount its children before the
  // first press, or this route opens nothing below 1100px.
  test("#/deals/new opens the create form with the verbs folded", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1024, height: 800 });
    await page.goto("/#/deals/new");
    await page.waitForLoadState("networkidle");
    await expectShellRendered(page);
    // The dialog, without anything being pressed. The heading, not the
    // button: the route stands in for the button, and asserting on the button
    // would pass over a form that never opened.
    await expect(
      page.getByRole("dialog").getByRole("heading", { name: "Neuer Deal" }),
    ).toBeVisible();
  });

  for (const view of ADDRESSED_VIEWS) {
    test(`no AA violations on #/${view}`, async ({ page }) => {
      await page.goto(`/#/${view}`);
      await page.waitForLoadState("networkidle");
      await expectShellRendered(page);
      await settleAnimations(page);
      await expectNoAaViolations(page, view);
    });
  }

  // The company record route (#/companies/<id>) takes an id, and CORE_SCREENS
  // names only the fixed nav surface. So it gets its own test.
  //
  // This mock harness has no /companies/{id}/360 route. Its fallback answers
  // an empty page with 200, not 404. So the 360 read succeeds with a body that
  // carries none of the fields a 360 promises. The page renders its own failure
  // state, never a blank or a stuck spinner. That state shows the header, the
  // tab strip and each section's "could not be loaded" wording.
  // A wrong-shaped 200 is how a reader meets a server one release out of step.
  // A card that reads a promised field without a guard takes the page down.
  // This covers the shell around the record, not the loaded content.
  test("no AA violations on #/companies/<id>", async ({ page }) => {
    await page.goto("/#/companies/o-brandt");
    await page.waitForLoadState("networkidle");
    // The sweep is only meaningful once the record chrome is on screen: axe
    // finds nothing to complain about in an empty shell.
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await settleAnimations(page);
    await expectShellRendered(page);
    await expectNoAaViolations(page, "companies/o-brandt");
  });

  // The transient confirmation, swept while it is on screen.
  //
  // Every other case in this file sweeps a settled page. A toast arrives after
  // a write and takes itself away again, so it is swept where it appears, over
  // a record page. In scope: the contrast of its dark plate and the reach of
  // what it carries. The qualify confirmation carries a link to the new contact.
  // A control inside a message is where keyboard reach is worth testing.
  test("no AA violations while a confirmation is on screen", async ({
    page,
  }) => {
    await page.goto("/#/leads/l-1");
    await page.waitForLoadState("networkidle");
    // The header's own verb, by testid: the rail's deal slice offers the same
    // act, and the name alone no longer says which control was pressed.
    await page.getByTestId("lead-qualify").click();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: /^Qualifizieren/ })
      .click();

    // The toast's own region: the shell's page wait is a status beside it.
    const said = page.locator(".toast-region").getByRole("status");
    await expect(said).toBeVisible();

    // Keyboard, before the sweep: the link the message carries is reachable,
    // and reaching it stops the withdrawal. A control that leaves the focus
    // ring seconds after a reader tabbed to it cannot be used (WCAG 2.2.1).
    // `EntityRef` is an anchor over the app's own hash route, so the record
    // opens the ways a link does: a new tab, a bookmark, the keyboard.
    // It is focusable either way; this test checks a keyboard reader can reach
    // it and keep it.
    const carried = said.getByRole("link").first();
    await carried.focus();
    await expect(carried).toBeFocused();
    await page.waitForTimeout(4000);
    await expect(said).toBeVisible();

    // `settleAnimations` runs first here as in every other sweep in this file.
    // The toast rises into place on `.arrive`, and axe reading a half-faded
    // plate measures a colour nothing is ever drawn in.
    await settleAnimations(page);
    await expectNoAaViolations(page, "leads/l-1 (confirmation on screen)");

    // And the way out, from inside the message rather than from its own
    // buttons, since the message carries a control.
    await page.keyboard.press("Escape");
    await expect(said).toBeHidden();
  });

  // The connectivity banner stands above whatever page the reader is on, so it
  // is swept where it lands first. Axe needs no network, so offline is safe.
  test("no AA violations with the offline banner up", async ({
    page,
    context,
  }) => {
    await page.goto("/#/home");
    await page.waitForLoadState("networkidle");
    await expectShellRendered(page);
    await context.setOffline(true);
    await expect(
      page.getByText(de["connectivity.offline.title"]),
    ).toBeVisible();
    await settleAnimations(page);
    await expectNoAaViolations(page, "home (offline banner)");
  });

  // The lead record (#/leads/<id>), for the same reason as the company one
  // above: an id-bearing route CORE_SCREENS does not name. Here the harness
  // answers the read, so the sweep reaches the loaded page. That covers the
  // readings strip, the ladder with its refused steps and the rail's folded
  // score section. It also covers the details grid's hover-to-edit rows.
  test("no AA violations on #/leads/<id>", async ({ page }) => {
    await page.goto("/#/leads/l-1");
    await page.waitForLoadState("networkidle");
    // The sweep is only meaningful once the record chrome is on screen: axe
    // finds nothing to complain about in an empty shell.
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await settleAnimations(page);
    await expectShellRendered(page);
    await expectNoAaViolations(page, "leads/l-1");
  });
});
