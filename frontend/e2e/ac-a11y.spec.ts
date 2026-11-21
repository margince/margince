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

// The same panel under the dark palette. It is the one surface where the
// stylesheet paints a fill of its own at this width — the well's ground and the
// notch's — and tokens.css publishes dark as its own set of values, so passing
// in light says nothing about it.
test.describe("B-EP09.21: WCAG 2.2 AA (axe), the agent's panel at 390px in dark", () => {
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

// A phone at 200% text holds a question taller than the room, and the board is
// the one box that scrolls: a keyboard reader has to be able to land on it.
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
    // The Core's row is what a room this short against its text gives up —
    // folded, never unmounted, or its WebGL loop restarts on the way back.
    await expect(page.locator(".ob-stage-core")).toHaveCount(1);
    await expect(page.locator(".ob-stage-core")).toBeHidden();
    // The ignition is the widest step: the sealed badge and the capability
    // lines are the lines that outgrew the board.
    await page.getByRole("button", { name: de["firstRun.continue"] }).click();
    await expect(page.locator(".ob-ig-can li")).toHaveCount(3);
    await expectBoardReachable(page);
  });

  // The same window at the text size it was built for keeps the Core: the fold
  // is measured against the reader's text, not against the phone.
  test("keeps the Core at the default text size", async ({ page }) => {
    await mockApi(page, { journey: "unconfigured" });
    await page.goto("/#/onboarding");
    await expect(page.locator(".ob-stage-title")).toBeVisible();
    await expect(page.locator(".ob-stage-core")).toBeVisible();
  });
});

// The addresses this product answers that carry a VIEW as well as a
// destination: a narrowed list, a record's tab, the decided queue, one report.
// They are swept separately from CORE_SCREENS because each is a state a reader
// reaches rather than a nav entry, and each draws chrome the bare screen does
// not — the chips the filter fills in, the tab strip's pressed control, the
// pager once a page is named.
const ADDRESSED_VIEWS = [
  "companies?q=brandt&sort=name",
  "companies/o-brandt/tasks",
  "analytics/forecast",
  "analytics/pipeline",
  "analytics/questions",
  // The three record headers whose verbs are icon-only: the name a sighted
  // reader gets on hover is not the name axe checks, so what is swept here is
  // the other half — that every square carries an accessible name at all, and
  // that a header of squares still meets AA on its own ground. A glyph with no
  // name is the one way this pattern fails silently, and it fails for exactly
  // the readers who cannot see the glyph.
  "contacts/p-anna",
  "deals/d-fleet",
  // The results screen with hits on it. It was in NEITHER sweep while it grew
  // groups, a type filter and links per row — and it is the surface a reader
  // arrives at by searching, so it is one of the few nobody navigates TO on
  // purpose and everybody lands on.
  "search/brandt",
  "projects/pr-fleet",
  // A focused filter page names itself, so the shell's heading stands down:
  // the one-h1 sweep is what holds the two from both printing one.
  "filters/companies",
];

// The SAME sweep under the dark palette, which the suite measured by accident
// for as long as the scheme went unpinned: a run inherited whichever palette
// the machine reported, so one afternoon read light locally and dark on CI and
// the lane presented as flaky. It was two palettes.
//
// It is a second block rather than a second tag on the block above because the
// palettes are not the same subject. tokens.css publishes dark as its own set
// of values — the accent lightens to the ADR-0040 mandate and the surfaces
// darken — so a screen can pass in one and fail in the other with no markup
// between them different. Eight findings across six screens were live here
// when this block was written, every one of them accent-coloured text on an
// accent-tinted fill, and none of them visible to the light sweep.
test.describe("B-EP09.21: WCAG 2.2 AA (axe), dark palette", () => {
  test.use({ colorScheme: "dark" });

  // Both lists, not just the nav entries. A record header drawn as icon-only
  // squares sits on the accent-tinted grounds this palette lightens, and the
  // eight findings this block was written for were every one of them
  // accent-coloured text on an accent-tinted fill — invisible to the light
  // sweep, with no markup between the two different.
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

// EXACTLY one h1 per route, which axe does not answer: `page-has-heading-one`
// asks whether there is AT LEAST one, and two page titles in one document is
// the failure a reader navigating by heading actually meets — they cannot tell
// which is the page.
//
// Across both route lists rather than the three routes AC-shell-1k names, so a
// new route inherits the check instead of needing somebody to remember it.
// That test keeps its own cases: it asserts WHICH heading wins on a record,
// which is a claim about the identity block rather than about the count.
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

test.describe("B-EP09.21: WCAG 2.2 AA (axe)", () => {
  for (const screen of CORE_SCREENS) {
    test(`no AA violations on #/${screen}`, async ({ page }) => {
      await page.goto(`/#/${screen}`);
      await page.waitForLoadState("networkidle");
      // Before measuring anything: a page that threw during render has almost
      // no accessibility tree to fault, so a crash used to read as a clean
      // sweep. #/settings/maintenance scored zero violations while showing the
      // app error boundary.
      await expectShellRendered(page);
      await expectSettingsViewLanded(page, screen);
      await settleAnimations(page);
      await expectNoAaViolations(page, screen);
    });
  }

  // The results screen with hits on it, in the light palette. ADDRESSED_VIEWS
  // above reaches it in dark alone, and the two sweeps disagree by design —
  // this is the half where an accent-coloured link on a light card is judged.
  test("no AA violations on the search results screen", async ({ page }) => {
    await page.goto("/#/search/brandt");
    await page.waitForLoadState("networkidle");
    await expectShellRendered(page);
    await expect(page.getByRole("heading", { name: "Produkte" })).toBeVisible();
    await settleAnimations(page);
    await expectNoAaViolations(page, "search/brandt");
  });

  // The narrowed screen is a DIFFERENT arrangement, not the same one shorter:
  // one group, a pressed pill, and — when the narrowing finds nothing — an
  // empty state under a filter row that has to stay operable.
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

  // The ⌘K palette is a modal, and no sweep in this file had ever opened one:
  // every axe pass above measures a page with the dialog closed, so the
  // surface a reader reaches from any screen in the product was unmeasured.
  test("no AA violations with the command palette open", async ({ page }) => {
    await page.goto("/#/home");
    await page.waitForLoadState("networkidle");
    await expectShellRendered(page);
    await page.keyboard.press("ControlOrMeta+k");
    await expect(
      page.getByRole("dialog", { name: "Befehlspalette" }),
    ).toBeVisible();
    await page
      .getByRole("searchbox", { name: "Befehlspalette" })
      .fill("brandt");
    // Wait on the hits, not on a duration: the live arm is what adds the rows
    // this sweep exists to judge.
    // Anchored: a contact found through the company carries its name too. The
    // avatar's printed initials lead the name (WCAG 2.5.3).
    await expect(
      page.getByRole("button", { name: /^(BA\s*)?Brandt Automotive$/ }),
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

  // The Questions section with a question asked and one of its rows opened:
  // the builder's rows of pickers, the answer table and the drawer it opens
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

  // The forecast's Shared links drawer with rows in it, and the confirmation a
  // row's Close link stacks over it: the closed page sweeps only the trigger.
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

  // A list header FOLDS its verbs into one overflow menu below 1100px
  // (design-system/listsurface.tsx), which is a different arrangement rather
  // than the same one narrower: the buttons are inside a disclosure, so the
  // trigger owes a name and the panel owes a relationship to it. The wide sweep
  // above cannot see any of that.
  test("no AA violations on a list header folded into its menu", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1024, height: 800 });
    await page.goto("/#/contacts");
    await page.waitForLoadState("networkidle");
    await expectShellRendered(page);
    await settleAnimations(page);
    // Opened, because a closed disclosure hides the controls this exists to
    // check — `OverflowMenu` keeps them mounted and `hidden`, and axe skips a
    // hidden subtree exactly as a reader does.
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
  // Not an axe case — a FUNCTIONAL one, and it sits here because this is the
  // only block that drives the folded arrangement. `CreateAction` reads its
  // opening state once, at mount, and the menu used to defer its children to
  // the first press: so this route opened nothing at all below 1100px, and
  // pressing the menu then opened a form nobody had asked for. Invisible in a
  // screenshot and green in every other lane.
  test("#/deals/new opens the create form with the verbs folded", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1024, height: 800 });
    await page.goto("/#/deals/new");
    await page.waitForLoadState("networkidle");
    await expectShellRendered(page);
    // The dialog, without anything being pressed. The heading rather than the
    // button: the button is what the ROUTE stands in for, and asserting on it
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

  // The company record route (#/companies/<id>) is not one of CORE_SCREENS —
  // it takes an id rather than a bare nav destination, and CORE_SCREENS names
  // the fixed nav surface — so it gets its own test rather than reshaping that
  // list for one parameterised route.
  //
  // This mock harness has no /companies/{id}/360 route, and its fallback
  // answers an empty PAGE with 200 rather than 404 — so the read the record
  // depends on for its strip, tabs bodies and rail succeeds with a body that
  // carries none of the fields a 360 promises. The page renders in its own
  // honest failure state throughout: the header, the tab strip and every
  // section's "could not be loaded" wording, never a blank or a spinner stuck
  // mid-load. That is still real chrome worth a sweep, and the wrong-shaped
  // 200 is worth MORE than the 404 this comment used to claim — it is how a
  // reader meets a server one release out of step, and it caught a card that
  // read a promised field without guarding it and took the whole page down.
  // This covers the shell around the record, not the loaded content a live
  // run would need to reach.
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

  // The transient confirmation, swept while it is ON SCREEN.
  //
  // Every other case in this file sweeps a settled page, and a toast is by
  // definition not on one — it arrives after a write and takes itself away
  // again. So it is swept where it actually appears, over a record page, with
  // the contrast of its own dark plate and the reachability of what it carries
  // both in scope. The qualify confirmation is the one to use because its body
  // carries a LINK to the contact that was just created: a control inside a
  // message is the case where "can a keyboard reader get to it, and back out of
  // it" has an answer worth having.
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

    const said = page.getByRole("status");
    await expect(said).toBeVisible();

    // Keyboard, before the sweep: the link the message carries is reachable,
    // and reaching it STOPS the withdrawal — a control that walks out from
    // under the focus ring three and a half seconds after a reader tabbed to it
    // is a control they cannot use (WCAG 2.2.1).
    // A LINK: `EntityRef` is an anchor over the app's own hash route, so the
    // record opens the ways a link does — a new tab, a bookmark, the keyboard.
    // Which is the point here — it is focusable either way, and this test is
    // about whether a keyboard reader can reach it and keep it.
    const carried = said.getByRole("link").first();
    await carried.focus();
    await expect(carried).toBeFocused();
    await page.waitForTimeout(4000);
    await expect(said).toBeVisible();

    // `settleAnimations` is what every other sweep in this file runs first, and
    // it applies here too: the toast rises into place on `.arrive`, and axe
    // reading a half-faded plate measures a colour nothing is ever drawn in.
    await settleAnimations(page);
    await expectNoAaViolations(page, "leads/l-1 (confirmation on screen)");

    // And the way out, from inside the message rather than from its own
    // buttons — the distinction a body-carried control is the whole reason for.
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

  // The lead record (#/leads/<id>), for the same reason and on the same terms
  // as the company one above: an id-bearing route CORE_SCREENS does not name.
  // Unlike that one the harness answers this read properly, so the sweep
  // reaches the loaded page — the readings strip, the ladder with its refused
  // steps, the rail's folded score section and the details grid's hover-to-edit
  // rows, which is where this page keeps its interactive controls.
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
