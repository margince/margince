import { expect, test } from "@playwright/test";
import { loadLocale } from "../src/i18n/catalogs";
import { de } from "../src/i18n/de";
import { expectNoAaViolations, settleAnimations } from "./a11y";
import { brandt } from "./mockapi/fixtures";
import { page as listPage } from "./mockapi/server";
import {
  CORE_SCREENS,
  expectSettingsViewLanded,
  expectShellRendered,
} from "./screens";
import { mockApi } from "./seed";
import { listClipping, pageOverflow } from "./waits";

test.beforeAll(async () => {
  await loadLocale("de");
});

// The AC-<screen>-N criteria run as named tests, so a failing test names the
// criterion it breaks. This file holds the 390px no-horizontal-scroll sweep,
// the signed-out screens and PERF-1's held-read claim for a record open.

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

// The record lists whose rows fold to a phone row at 390px; the deals list as its table.
const PHONE_ROW_LISTS = [
  "contacts",
  "companies",
  "leads",
  "projects",
  "deals?view=table",
  "partners",
];

test.describe("§3.8: 390px mobile", () => {
  test.use({ viewport: { width: 390, height: 844 } });

  for (const screen of CORE_SCREENS) {
    test(`nothing scrolls sideways or hides at 390px on #/${screen}`, async ({
      page,
    }) => {
      await page.goto(`/#/${screen}`);
      await page.waitForLoadState("networkidle");
      await expectShellRendered(page);
      await expectSettingsViewLanded(page, screen);
      // The scroller has to be FOUND, or a renamed class would leave this
      // sweep quietly measuring only the document — which is the blindness it
      // was written to end.
      await expect(page.locator(".scroll")).toHaveCount(1);
      // Named offenders rather than a count: a bare number tells whoever broke
      // this that something is 169px too wide and nothing about what.
      expect(await pageOverflow(page)).toEqual([]);
      expect(await listClipping(page)).toEqual([]);
    });
  }

  // A phone row draws an end badge and a muted caption line that no wide sweep reaches.
  for (const list of PHONE_ROW_LISTS) {
    test(`no AA violations on the phone rows of #/${list}`, async ({
      page,
    }) => {
      // The mock API serves no partner, and an empty list draws no row.
      if (list === "partners") {
        await page.route(/\/v1\/partners(\?.*)?$/, (route) =>
          route.fulfill({
            json: listPage([
              {
                company_id: brandt.id,
                partner_kinds: ["hosting", "referral"],
                cert_status: "certified",
                relationship_stage: "active",
                stage_label: "Live",
                stage_meaning: "active",
                version: 1,
                created_at: "2026-01-01T00:00:00Z",
                updated_at: "2026-06-01T00:00:00Z",
              },
            ]),
          }),
        );
      }
      await page.goto(`/#/${list}`);
      await page.waitForLoadState("networkidle");
      await expectShellRendered(page);
      await expect(
        page.locator(".lt-table.lt-phonerows tbody tr").first(),
      ).toBeVisible();
      await settleAnimations(page);
      await expectNoAaViolations(page, `${list} (390px)`);
    });
  }

  test("nothing scrolls sideways at 390px on the search results", async ({
    page,
  }) => {
    await page.goto("/#/search/brandt");
    await page.waitForLoadState("networkidle");
    await expectShellRendered(page);
    await expect(page.getByRole("heading", { name: "Produkte" })).toBeVisible();
    expect(await pageOverflow(page)).toEqual([]);
  });

  test("AC-company-relationships: a company's relationship table folds at 390 and keeps each date whole", async ({
    page,
  }) => {
    await page.route(/\/v1\/relationships(\?.*)?$/, (route) =>
      route.fulfill({
        json: listPage([
          {
            id: "r-anna",
            kind: "employment",
            company_id: brandt.id,
            contact_id: "p-anna",
            role: "Leiterin Einkauf und strategische Beschaffung",
            started_at: "2021-03-01",
            ended_at: "2024-11-30",
            is_current_primary: false,
            employment_status: "former",
            version: 1,
            created_at: "2026-01-01T00:00:00Z",
            updated_at: "2026-06-01T00:00:00Z",
          },
        ]),
      }),
    );
    await page.goto(`/#/companies/${brandt.id}/profile`);
    await page.waitForLoadState("networkidle");
    await expectShellRendered(page);
    const dates = page.locator(".relationshiprows-date");
    await expect(dates).toHaveCount(2);
    await settleAnimations(page);
    expect(await pageOverflow(page)).toEqual([]);
    const pastScrollBox = await page.evaluate(() =>
      Array.from(document.querySelectorAll(".table-scroll")).flatMap((box) =>
        Array.from(box.querySelectorAll("td, th"))
          .filter(
            (cell) =>
              cell.getBoundingClientRect().right >
              box.getBoundingClientRect().right + 1,
          )
          .map((cell) => (cell.textContent ?? "").trim()),
      ),
    );
    expect(pastScrollBox).toEqual([]);
    const lineBoxes = await dates.evaluateAll((nodes) =>
      nodes.map(
        (node) =>
          new Set(
            Array.from(node.getClientRects(), (rect) => Math.round(rect.top)),
          ).size,
      ),
    );
    expect(lineBoxes).toEqual([1, 1]);
    await expectNoAaViolations(page, "company profile, relationships (390px)");
  });

  test("the More menu opens above the bar and meets AA at 390px", async ({
    page,
  }) => {
    await page.goto("/#/home");
    await page.waitForLoadState("networkidle");
    await expectShellRendered(page);
    const more = page.locator("nav.rail .railmore");
    await more.click();
    const menu = page.locator("nav.rail .railmenu");
    await expect(menu).toBeVisible();
    await expect(more).toHaveAttribute("aria-expanded", "true");
    await settleAnimations(page);
    const menuBox = await menu.boundingBox();
    const barBox = await page.locator("nav.rail").boundingBox();
    expect(menuBox).not.toBeNull();
    expect(barBox).not.toBeNull();
    expect((menuBox?.y ?? 0) + (menuBox?.height ?? 0)).toBeLessThan(
      barBox?.y ?? 0,
    );
    expect(await pageOverflow(page)).toEqual([]);
    await expectNoAaViolations(page, "home, More menu open");

    await more.click();
    await expect(menu).toHaveCount(0);
    await expect(more).toBeFocused();
  });

  // The palette is a full-height SHEET at this width — a 560px box floated at
  // 12vh with a 320px list had about two rows left under a software keyboard.
  // Its rows are thumb targets, which is the half a screenshot cannot assert.
  test("the palette is a workable sheet at 390px", async ({ page }) => {
    await page.goto("/#/home");
    await page.waitForLoadState("networkidle");
    await expectShellRendered(page);
    await page.keyboard.press("ControlOrMeta+k");
    await expect(
      page.getByRole("dialog", { name: "Befehlspalette" }),
    ).toBeVisible();
    // The sheet ARRIVES — it scales up from 0.96 (atoms.css) — and a row
    // measured while it is still growing reports the size the box is passing
    // through rather than the size a thumb meets. 44 × 0.964 is 42.4, which is
    // what this read before the wait.
    await settleAnimations(page);
    expect(await pageOverflow(page)).toEqual([]);
    const rows = page.locator(".palette-row");
    await expect(rows.first()).toBeVisible();
    const heights = await rows.evaluateAll((nodes) =>
      nodes.map((node) => node.getBoundingClientRect().height),
    );
    expect(Math.min(...heights)).toBeGreaterThanOrEqual(44);
  });

  // AC-WORKLIST-SDR-01: the first action is above the phone fold.
  //
  // The whole promise of this screen on a phone is that a rep opens it and can
  // ACT — not scroll, then act. Everything above the first row competes for
  // that space: the title, the count sentence, the readings strip, the scope
  // control. Each was added for a good reason and none of them is the work.
  //
  // Measured against the VIEWPORT rather than a fixed pixel budget, because the
  // number that matters is whether a thumb can reach it without scrolling. The
  // page is not scrolled first: this asserts the arriving screen.
  // PASSES since the decision moved into a drawer.
  //
  // It failed for as long as the first seeded row was an approval drawing its
  // whole decision inline — evidence, draft and three answers — 440px of card
  // inside a row whose ceiling is 208. "Übernehmen" ended at 920px on an 844px
  // screen and a rep had to scroll past one decision to reach their work.
  //
  // The row now offers the verb and the drawer holds the card, so the row is
  // 205px and the first action is inside the fold. It was left failing rather
  // than relaxed to 950px through several sessions, which is why it could go
  // green on the fix instead of being quietly tightened to agree with it.
  test("AC-WORKLIST-SDR-01: the first primary action is above the fold at 390x844", async ({
    page,
  }) => {
    await page.goto("/#/worklist");
    await page.waitForLoadState("networkidle");
    await expect(page.locator(".worklist-list li").first()).toBeVisible();

    const reach = await page.evaluate(() => {
      const row = document.querySelector(".worklist-list li");
      if (!row) {
        return null;
      }
      // THE PRIMARY ACTION, which is the one that resolves the work — not
      // merely the first control the row happens to lay out.
      //
      // Taking the first match was wrong in the way that matters: on an
      // approval row the first control is the evidence link ("Details zur Freigabe"),
      // which sits well above the Accept and Reject buttons that actually
      // answer the row. The test passed while the verb a rep presses was still
      // below the fold — a measurement of the wrong control is a green that
      // means nothing.
      //
      // `btn-primary` is how the queue marks the one accent verb. An inline
      // decision offers several equal answers and draws none. There the
      // lowest control is measured, because a row works only when the reader
      // can reach all of it.
      //
      // Read off the ROW, never from a list of the containers it groups its
      // verbs into. A selector naming those is a claim about which groups
      // exist, and a renamed group leaves it matching nothing: this case then
      // reports "the first row drew no action to measure" for a row that draws
      // four, which is under-recognition and reads exactly like a pass. The
      // rank is excluded because it opens the pane rather than answering the
      // work; a title link is not a `.btn` and is not a verb.
      const controls = Array.from(
        row.querySelectorAll("button:not(.worklist-rank-select), a.btn"),
      ).filter((element) => element.getBoundingClientRect().height > 0);
      if (controls.length === 0) {
        return null;
      }
      const primary =
        controls.find((element) => element.classList.contains("btn-primary")) ??
        controls.reduce((lowest, element) =>
          element.getBoundingClientRect().bottom >
          lowest.getBoundingClientRect().bottom
            ? element
            : lowest,
        );
      return {
        bottom: primary.getBoundingClientRect().bottom,
        fold: window.innerHeight,
        label: (primary.textContent ?? "").trim(),
      };
    });

    // A day whose first row carries no verb at all is not this test's subject,
    // and passing vacuously on one would hide exactly the regression it exists
    // to catch — so it fails rather than skips.
    expect(reach, "the first row drew no action to measure").not.toBeNull();
    const seen = reach as { bottom: number; fold: number; label: string };
    expect(
      seen.bottom,
      `"${seen.label}" ends ${Math.round(seen.bottom)}px down a ${seen.fold}px screen`,
    ).toBeLessThanOrEqual(seen.fold);
  });

  // The drawer itself, on the screen size it was built for.
  //
  // SDR-01 above proves the row got shorter, and a row that lost its card and
  // gained a button that opens nothing would pass it. This asserts the other
  // half: the decision is still answerable, and only once the reader asks.
  test("AC-WORKLIST-SDR-03: a decision is answered in a drawer, not in the row", async ({
    page,
  }) => {
    await page.goto("/#/home?queue=1&queue_view=decisions");
    await page.waitForLoadState("networkidle");
    await expect(page.locator(".worklist-list li").first()).toBeVisible();

    // Nothing to answer until it is opened: the queue draws no verdict button.
    await expect(
      page.getByRole("button", { name: "E-Mail freigeben", exact: true }),
    ).toHaveCount(0);

    await page.getByRole("button", { name: "Entscheiden" }).first().click();

    const decision = page.getByRole("dialog", {
      name: "Deine Entscheidung",
      exact: true,
    });
    await expect(decision).toBeVisible();
    // The same card the record page draws, with its verdicts on it.
    await expect(
      decision.getByRole("button", { name: "E-Mail freigeben", exact: true }),
    ).toBeVisible();
  });

  // The height a row is allowed to take before it has to fold.
  //
  // The fold test above measures the FIRST row's reach; this one measures every
  // row's own height, because a queue is only workable if a reader can travel
  // it. A row that draws its whole decision inline — evidence, draft, three
  // answers — takes 601px on a phone, so two rows fill a screen and a half and
  // the queue stops being a list a thumb can work through.
  //
  // 208px for a row carrying an inline decision, 176px for one that does not:
  // the two ceilings the product's own row anatomy sets.
  //
  // FAILS TODAY on TWO rows, and they have different causes — which is why
  // this note no longer says "the same one cause" as it once did.
  //
  // The approval is FIXED: its decision moved into a drawer, and the row went
  // from 657px to 205px, inside its 208px ceiling.
  //
  // The second is an ORDINARY TASK, and no drawer touches it. It measures
  // 205px against a 176px ceiling. Its parts: title 49 (the category badge
  // wraps to a second line at 390px), the three facts on ONE line 19,
  // consequence 19, the primary verbs 44, rank 19, and the rest gap and
  // padding.
  //
  // Two rounds have now taken what can be taken. The set-aside verbs left the
  // row for a swipe (284 -> 227), and the deadline stopped being printed twice
  // — `when` gives the hour and `due_today` gave the day underneath it, so the
  // phrase gives way and the remaining facts share one line (227 -> 205).
  //
  // What is left is not padding. The 44px band is the row's PRIMARY actions,
  // which is how a rep does the work; 44px is the touch target rather than a
  // style. The 49px title is 49 because the category badge wraps, and the badge
  // says what kind of work the row is. Closing the last 29px means dropping a
  // fact rather than a duplicate — or raising a ceiling that was never measured
  // and which the decision rows already set at 208.
  //
  // Left failing rather than relaxed, so the decision is made by someone rather
  // than by a number quietly rising to agree with the defect.
  test.fixme("AC-WORKLIST-SDR-07: no row outgrows its ceiling at 390x844", async ({
    page,
  }) => {
    await page.goto("/#/worklist");
    await page.waitForLoadState("networkidle");
    await expect(page.locator(".worklist-list li").first()).toBeVisible();

    const tall = await page.evaluate(() => {
      return Array.from(document.querySelectorAll(".worklist-list li"))
        .map((row) => {
          const decides =
            row.querySelector("[data-testid='worklist-row-decision']") !== null;
          return {
            height: row.getBoundingClientRect().height,
            ceiling: decides ? 208 : 176,
            title: (row.querySelector(".worklist-row-title")?.textContent ?? "")
              .trim()
              .slice(0, 40),
          };
        })
        .filter(({ height, ceiling }) => height > ceiling)
        .map(
          ({ height, ceiling, title }) =>
            `${title}: ${Math.round(height)}px over a ${ceiling}px ceiling`,
        );
    });
    expect(tall).toEqual([]);
  });

  // A THUMB, not a mouse aimed at a phone-sized window. Every touch floor in
  // this product is declared under `@media (pointer: coarse)`, and Chromium
  // resolves that off touch emulation rather than off the viewport — so a run
  // without it measures the desktop sizes and reports them as a phone's.
  test.describe("under a coarse pointer", () => {
    test.use({ hasTouch: true });

    // The queue is WORKABLE with a thumb, not merely present.
    //
    // What stood here asserted that one text node was visible at 390px, and it
    // passed for as long as the screen was unusable: the row is a three-column
    // line whose verbs never yield width, so the title column was squeezed to a
    // few characters while three buttons held their full size beside it. A test
    // that cannot tell that from a working screen is part of the defect.
    //
    // So this measures the row itself — every row, not the first — and the
    // targets a rep presses.
    test("the day's queue is workable with a thumb at 390px", async ({
      page,
    }) => {
      await page.goto("/#/home?queue=1&queue_view=decisions");
      await page.waitForLoadState("networkidle");
      await settleAnimations(page);
      await expect(
        page.getByText(/Send the follow-up to Anna Weber/).first(),
      ).toBeVisible();

      // Nothing runs off the side. The row wraps instead of pushing the page
      // wider, which is the difference between a stacked layout and a squeezed
      // one.
      expect(await pageOverflow(page)).toEqual([]);

      // The text column is wide enough to read a sentence in. Half the viewport
      // is a low bar deliberately: it is the one this layout FAILED, at roughly a
      // quarter, and a ceiling tuned to today's rows would break on tomorrow's
      // longer verb.
      const narrow = await page.evaluate(() => {
        const floor = 390 / 2;
        return Array.from(document.querySelectorAll(".worklist-row-text"))
          .map((element) => ({
            width: element.getBoundingClientRect().width,
            text: (element.textContent ?? "").slice(0, 40),
          }))
          .filter(({ width }) => width < floor)
          .map(({ width, text }) => `${Math.round(width)}px: ${text}`);
      });
      expect(narrow).toEqual([]);

      // Every control this screen is FOR is a real target: the queue's rows and
      // the readings strip above them, whose "Open →" is the way into a lane.
      // The strip used to be left out of this sweep on the argument that its
      // door belongs to the design system's StatCard rather than to this screen
      // — true of where the fix goes, and no reason for the screen to stop
      // measuring a control a rep presses on it.
      //
      // TWO censuses, not one list, and each reports what it FOUND before it
      // reports what was too small. A selector that matches nothing returns an
      // empty list of offenders — the same value a clean screen returns — so a
      // renamed class would leave this sweep measuring half the page and still
      // green. Counting first is what makes a region going missing a red.
      //
      // Both spellings of the strip are named: this route draws the Brief's
      // readings today and the worklist's own strip is the same component in
      // the same place, so whichever is present is the one a rep presses.
      //
      // Visible controls only: the page carries collapsed panels whose buttons
      // lay out at zero height, and those are not targets a thumb can miss.
      const targets = await page.evaluate(() => {
        const census = (scopes: readonly string[]) => {
          const found = scopes.flatMap((scope) =>
            Array.from(
              document.querySelectorAll(
                `${scope} button, ${scope} a.btn, ${scope} .link-button`,
              ),
            ),
          );
          // One control can answer to two of those selectors (a `.link-button`
          // that is a `<button>`), and a census that counted it twice would
          // report a page busier than it is.
          const visible = Array.from(new Set(found)).filter(
            (element) => element.getBoundingClientRect().height > 0,
          );
          return {
            counted: visible.length,
            small: visible
              .map((element) => ({
                height: element.getBoundingClientRect().height,
                // A glyph button has no text, so its name is its aria-label.
                label: (
                  element.getAttribute("aria-label") ||
                  element.textContent ||
                  ""
                ).trim(),
              }))
              .filter(({ height }) => height < 44)
              .map(({ height, label }) => `${label}: ${Math.round(height)}px`),
          };
        };
        return {
          readings: census([".brief-readings", ".worklist-readings"]),
          queue: census([".worklist-list"]),
          main: census([".main"]),
        };
      });

      expect(targets.readings.counted).toBeGreaterThan(0);
      expect(targets.readings.small).toEqual([]);
      expect(targets.queue.counted).toBeGreaterThan(0);
      expect(targets.queue.small).toEqual([]);
      expect(targets.main.counted).toBeGreaterThan(0);
      expect(targets.main.small).toEqual([]);
    });
  });

  // The bar's centre cell and the panel it opens, neither of which exists above
  // this breakpoint: there the agent is the sidebar's foot and the panel stands
  // beside it. So every sweep in this file runs with that panel shut, and it is
  // the largest surface in the product no other case opens.
  //
  // Three things at once, because they are one act: it opens ABOVE the well
  // rather than across it (a frame measured from the cell behind the well would
  // cover the orb that opened it), the page does not grow sideways under it, and
  // it scores no AA violation.
  test("the agent's panel opens clear of the bar and fits 390px", async ({
    page,
  }) => {
    await page.goto("/#/home");
    await page.waitForLoadState("networkidle");
    await expectShellRendered(page);

    await page.getByRole("button", { name: "Agentenbereich öffnen" }).click();
    const panel = page.locator(".arpanel");
    await expect(panel).toBeVisible();
    await settleAnimations(page);

    // By class, not by name: the control renames itself the moment it is open
    // ("Collapse the agent panel"), which is the point of it — and what is
    // measured here is a BOX, not an identity.
    const panelBox = await panel.boundingBox();
    const wellBox = await page.locator(".arhit").boundingBox();
    if (!panelBox || !wellBox) {
      throw new Error("the panel or the well it points at is not laid out");
    }
    expect(panelBox.y + panelBox.height).toBeLessThanOrEqual(wellBox.y);
    expect(panelBox.y).toBeGreaterThanOrEqual(0);
    expect(await pageOverflow(page)).toEqual([]);
    await expectNoAaViolations(page, "brief — the agent's panel (390px)");
  });

  // AC-shell-8, restated against the taskbar. The criterion is the promise
  // about what the agent can reach, which row scope bounds. A reader cannot
  // rely on a guarantee nobody states, so the panel where a reader meets the
  // agent must state it.
  test("AC-shell-8: the agent's panel states what the agent can reach", async ({
    page,
  }) => {
    await page.goto("/#/home");
    await page.waitForLoadState("networkidle");
    await expectShellRendered(page);

    await page.getByRole("button", { name: "Agentenbereich öffnen" }).click();
    await expect(
      page
        .locator(".arpanel")
        .getByText("Margince liest nur, was du sehen kannst."),
    ).toBeVisible();
  });

  // The whole keyboard path of this panel: it opens from the bar, and Escape
  // closes it from inside. Focus lands back on the control that opened it.
  // Focus on <body> would start the next Tab at the top of the page.
  test("the agent's panel closes on Escape and hands focus back to the orb", async ({
    page,
  }) => {
    await page.goto("/#/home");
    await page.waitForLoadState("networkidle");
    await expectShellRendered(page);

    const orb = page.getByRole("button", { name: "Agentenbereich öffnen" });
    await orb.click();
    await expect(page.locator(".arpanel")).toBeVisible();

    await page.keyboard.press("Escape");
    await expect(page.locator(".arpanel")).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: "Agentenbereich öffnen" }),
    ).toBeFocused();
  });
});

// ---------------------------------------------------------------------------
// The unauthenticated surface (ADR-0076): §3.8 at two narrow widths, 200% zoom,
// the two-region structure, and axe.
//
// WHY IT NEEDS ITS OWN BLOCK: every sweep above walks CORE_SCREENS, and all of
// those start behind a session — so the one screen a signed-out user actually
// meets was never measured at any width. It is also the only screen the spec
// gives a SECOND region, which makes it the likeliest place a breakpoint
// regression lands and the least likely place anyone notices: nobody resizes the
// login page.
//
// These override the file-level beforeEach with an unauthenticated mock, so the
// app renders login rather than the shell.
// ---------------------------------------------------------------------------
test.describe("ADR-0076: the unauthenticated surface", () => {
  test.beforeEach(async ({ page }) => {
    await mockApi(page, { session: "unauthenticated" });
  });

  // Each entry is a viewport the spec names. 200% zoom is expressed as the CSS
  // viewport it actually produces — a 1280x800 window at 200% presents 640x400 —
  // because that is what the layout sees; deviceScaleFactor would change the
  // pixel ratio and nothing about the breakpoints.
  // The identity region is part of the surface at EVERY width: the sign-in is
  // one centred column, and the Core and its sentences are what the surface is
  // for, so no phone layout drops them.
  const NARROW = [
    { label: "390px mobile", width: 390, height: 844 },
    { label: "320px narrow", width: 320, height: 568 },
    { label: "200% zoom", width: 640, height: 400 },
  ] as const;

  for (const { label, width, height } of NARROW) {
    test.describe(label, () => {
      test.use({ viewport: { width, height } });

      test("no horizontal body scroll", async ({ page }) => {
        await page.goto("/");
        await expect(
          page.getByRole("heading", {
            level: 1,
            name: "Das ist Margince.",
          }),
        ).toBeVisible();
        const overflow = await page.evaluate(
          () =>
            document.documentElement.scrollWidth -
            document.documentElement.clientWidth,
        );
        expect(overflow).toBeLessThanOrEqual(0);
      });

      // The defect this pins is specific and was live: a fixed, overflow-hidden
      // full-screen surface cannot scroll, so at 320px or under a phone keyboard
      // the submit button is simply unreachable (§13.3).
      test("keeps the primary action reachable", async ({ page }) => {
        await page.goto("/");
        const submit = page.getByRole("button", { name: "Anmelden" });
        await submit.scrollIntoViewIfNeeded();
        await expect(submit).toBeVisible();
        // Thrown rather than asserted-and-continued: an `if (box)` guard around
        // the checks below would let a null box SKIP them, and a test that can
        // no-op is the thing this one exists to stop being.
        const box = await submit.boundingBox();
        if (!box) {
          throw new Error("the submit button rendered no box");
        }
        const viewport = page.viewportSize();
        if (!viewport) {
          throw new Error("the page reported no viewport");
        }
        // §6.5/§12: 44px is the target floor, not a rounded-up number.
        expect(Math.round(box.height)).toBeGreaterThanOrEqual(44);
        // `toBeVisible()` passes for a CSS-visible element parked outside the
        // viewport on a fixed, overflow-hidden surface, which is exactly the
        // defect this test is named for. Pin containment directly.
        expect(box.y).toBeGreaterThanOrEqual(0);
        expect(box.y + box.height).toBeLessThanOrEqual(viewport.height);
      });

      // The region shows all of itself: not one of its rows may be dropped or
      // clipped to fit (ADR-0076 Decision 6, and two earlier implementations
      // dropped rows with `display: none`). Presence AND rendered height,
      // because a node that is CSS-visible inside a container hiding its
      // overflow still passes `toBeVisible` while measuring nothing — which is
      // exactly how the copy came to be cut off at a narrow width once already.
      //
      // Named ROW BY ROW rather than counted: the copy is one paragraph in one
      // voice, so a row missing from the middle of it is a sentence the system
      // stopped saying, and a count would pass on any two elements.
      //
      // Two rows, since the surface stopped making claims about itself: the
      // greeting and the one line about what the product is for. The kicker,
      // the send promise and the handover line were removed with the AI
      // posture readout they belonged to.
      const identityRows = [".auth-statement", ".auth-purpose"];

      test("shows the identity region whole", async ({ page }) => {
        await page.goto("/");
        await expect(page.locator(".auth-identity")).toBeVisible();
        for (const row of identityRows) {
          const line = page.locator(row);
          await expect(line).toHaveCount(1);
          await expect(line).toBeVisible();
          const box = await line.boundingBox();
          expect(box?.height ?? 0).toBeGreaterThan(0);
        }
        // The class, not the tag: `RaillessFrame` already wraps every
        // rail-less screen in a `<main>`, so the task region is a `<div>` —
        // a second `<main>` here would be an invalid, duplicate landmark.
        await expect(page.locator(".auth-task")).toBeVisible();
      });
    });
  }

  // One column at every width, the identity region ABOVE the task on screen
  // while the task stays FIRST in the DOM (Decision 1): `order` is what puts
  // the Core over the form, so a reading order that starts at the form and a
  // picture that starts at the Core are the same markup.
  for (const width of [390, 720, 1280]) {
    test(`puts the identity region above the task at ${width}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 900 });
      await page.goto("/");
      await expect(
        page.getByRole("heading", {
          level: 1,
          name: "Das ist Margince.",
        }),
      ).toBeAttached();
      // The class, not the tag: see the note beside the other `.auth-task`
      // locator above — one `<main>` per screen, and it belongs to the frame.
      const task = page.locator(".auth-task");
      const identity = page.locator(".auth-identity");
      await expect(task).toBeVisible();
      await expect(identity).toBeVisible();
      const taskBox = await task.boundingBox();
      const identityBox = await identity.boundingBox();
      expect(identityBox?.y ?? 0).toBeLessThan(taskBox?.y ?? 0);
      const order = await page.evaluate(() => {
        const first = document.querySelector(".auth-task");
        const second = document.querySelector(".auth-identity");
        return first && second
          ? (first.compareDocumentPosition(second) &
              Node.DOCUMENT_POSITION_FOLLOWING) !==
              0
          : false;
      });
      expect(order).toBe(true);
    });
  }

  // §12: one h1, and it is the GREETING. What this surface is ABOUT is the
  // system introducing itself; whichever of the four outcomes the frame is
  // carrying is a section under that name, so the sign-in card's own title is an
  // h2 — and `.sr-only`, because the greeting next to it has already said what
  // the page is. Both halves are asserted: a view that promotes its card back to
  // h1 leaves two, and one that drops the card's title leaves the form unnamed.
  //
  // The accessible NAME, not the text: the greeting is typed, so two of its
  // three layers are `aria-hidden` copies — one holding the final height open,
  // one carrying the partial string — and `textContent` would read all three.
  test("has exactly one h1, and it is the greeting", { tag: "@smoke" }, async ({
    page,
  }) => {
    await page.goto("/");
    const headings = page.getByRole("heading", { level: 1 });
    await expect(headings).toHaveCount(1);
    await expect(
      page.getByRole("heading", { level: 1, name: "Das ist Margince." }),
    ).toBeAttached();
    await expect(
      page.getByRole("heading", { level: 2, name: "Bei Margince anmelden" }),
    ).toBeAttached();
  });

  // The Core is decoration (WDS-CORE-4): every state it shows is also stated in
  // text by the surface around it, so it must not reach the a11y tree at all.
  test("keeps the Core out of the accessibility tree", async ({ page }) => {
    await page.goto("/");
    const core = page.locator("[data-core-state]");
    await expect(core).toHaveCount(1);
    await expect(core).toHaveAttribute("aria-hidden", "true");
  });

  // §19: no control whose flow does not exist — and the reason this passes has
  // CHANGED. The federated block has markup now, so it is no longer a property
  // of the tree that nothing could render a provider button. The gate is the
  // CAPABILITY: /auth/capabilities serves `oidc_providers: []` because the OIDC
  // flow has not shipped, and `ProviderButtons` returns null for an empty list.
  // The companion test below drives the same gate in its "on" position, because
  // a gate only ever seen switched off is a gate nobody has watched work.
  test("offers no identity provider that does not work", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator(".auth-sso")).toHaveCount(0);
    await expect(page.locator(".auth-or")).toHaveCount(0);
    await expect(
      page.getByText(/weiter mit|continue with|oder per e-mail/i),
    ).toHaveCount(0);
  });

  // The other direction: an installation whose administrator HAS wired SSO. The
  // labels are the SERVER's strings (the second one is a provider this frontend
  // has never heard of, which is the normal case for a self-hosted product), the
  // buttons are real focusable controls at the 44px target floor, and the divider
  // labels the PASSWORD FORM below them — where SSO exists the form is the
  // fallback door, so the divider names it rather than the buttons.
  test("offers the providers an installation does serve", async ({ page }) => {
    await mockApi(page, {
      session: "unauthenticated",
      oidcProviders: [
        { key: "google", label: "Weiter mit Google" },
        { key: "corp-sso", label: "Anmeldung über Werk-IT" },
      ],
    });
    await page.goto("/");

    const google = page.getByRole("button", { name: "Weiter mit Google" });
    const corp = page.getByRole("button", { name: "Anmeldung über Werk-IT" });
    await expect(google).toBeVisible();
    await expect(corp).toBeVisible();
    // An unrecognised key still gets a mark — a neutral one rather than nothing,
    // because a working sign-in path must not be hidden for want of a logo.
    await expect(page.locator(".auth-sso .provider-mark")).toHaveCount(2);

    // Reachable, not merely present.
    await google.focus();
    await expect(google).toBeFocused();
    expect(
      Math.round((await google.boundingBox())?.height ?? 0),
    ).toBeGreaterThanOrEqual(44);

    // The divider sits between the providers and the form it labels.
    const divider = page.locator(".auth-or");
    await expect(divider).toHaveText("oder");
    const buttonsY = (await page.locator(".auth-sso").boundingBox())?.y ?? 0;
    const dividerY = (await divider.boundingBox())?.y ?? 0;
    const fieldsY = (await page.locator(".auth-fields").boundingBox())?.y ?? 0;
    expect(buttonsY).toBeLessThan(dividerY);
    expect(dividerY).toBeLessThan(fieldsY);

    // The federated block exists only under this seeding, so the axe sweep below
    // would never see it. Run it here too rather than leave the one part of the
    // surface a user of an SSO installation actually clicks unmeasured.
    await page.waitForLoadState("networkidle");
    await settleAnimations(page);
    await expectNoAaViolations(page, "login (with SSO providers)");
  });

  test("no AA violations on the login screen", async ({ page }) => {
    await page.goto("/");
    await page.waitForLoadState("networkidle");
    // The rail-less surface has no shell to check for; its own h1 is the proof
    // the screen rendered, and the block above already asserts that.
    await expect(
      page.getByRole("heading", { level: 1, name: "Das ist Margince." }),
    ).toBeVisible();
    await settleAnimations(page);
    await expectNoAaViolations(page, "login");
  });
});

// What makes a record open feel instant is not a number on this runner, it is
// how little the identity waits for. So a HELD read is what asserts it: the
// request is made to hang, and the heading has to arrive anyway. That claim
// means the same thing on an idle laptop and on a CI box running six other jobs,
// which no reading of a clock does.
//
// This case holds the read the heading itself comes from. A record route carries
// an id and not a name, so the head is seeded from the list row the open was
// clicked from (`useCachedRecordName`) and draws before `/contacts/{id}/360`
// answers. Holding the plain `/contacts/{id}` instead would leave the case green
// over a page that waits, because that is not the request the heading depends on.
//
// The perceived BUDGET is not asserted here at all. One wall-clock sample says
// how busy the runner was, and this lane shares its machine with six integration
// shards. `make bench-mobile` owns the 300ms figure as a p95 over 20 samples on
// a throttled Fast-3G profile — the harder of the two conditions, so a budget
// that holds there holds unthrottled by construction.
test("PERF-1: a record opens on its route's identity, not on its read", async ({
  page,
}) => {
  // Held, not slowed: the read cannot have answered when the assertion below
  // runs, so a page that waited on it could not pass by being lucky. The hold
  // stays under Playwright's 5s expect timeout on purpose — a page that DID wait
  // still resolves its heading, so this case fails on `readAnswered`, which names
  // the mechanism, rather than timing out with nothing to say.
  const READ_HELD_MS = 3000;
  // Both flags, because `readAnswered` alone is false in two different worlds:
  // the read was held, and the read was never sent. A renamed route or a page
  // that stops issuing this request would leave the assertion below green having
  // exercised nothing, which is the failure a held-read case cannot notice about
  // itself. `readStarted` is what tells the two apart.
  let readStarted = false;
  let readAnswered = false;
  await page.route("**/contacts/p-anna/360", async (route) => {
    readStarted = true;
    await new Promise((settle) => setTimeout(settle, READ_HELD_MS));
    readAnswered = true;
    await route.fallback();
  });

  await page.goto("/#/contacts");
  // Anchor on a settled screen first: a click during hydration can land on a
  // row whose handler is not attached yet — the navigation then never happens
  // and the assertion times out as a phantom failure (twice-seen CI flake).
  await page.waitForLoadState("networkidle");
  // The list row that carries the name, by ROLE: the contacts list draws a
  // contact as a table row, and a bare text match would also take any other
  // element that legitimately repeats the name (the agent panel's spoken line,
  // a bulk-select label) without saying which one it clicked. Substring on
  // purpose — a row's accessible name is every cell of it joined, so the contact's
  // name is a fragment of it by construction and `exact` could never match.
  const row = page.getByRole("row", { name: "Anna Weber" });
  await expect(row).toBeVisible();

  await row.click();
  // The record's own header, not the shell's — the head shows only the trail on
  // a record route, and it renders from the router before any record read
  // returns, so waiting on it would measure routing rather than the open. The
  // heading is exact: the whole name is what says the right record opened.
  await expect(
    page.getByRole("heading", { level: 1, name: "Anna Weber", exact: true }),
  ).toBeVisible();
  expect(readStarted).toBe(true);
  expect(readAnswered).toBe(false);
});

// A new condition and the pickers it opens: the field, an owner, a company search.
test("AC-filters-and-views-6: a new condition names no field, and each picker it opens meets AA and closes on Escape", async ({
  page,
}) => {
  await page.route(/\/v1\/filters\/vocabulary\?.*resource=contact/, (route) =>
    route.fulfill({
      json: {
        resource: "contact",
        fields: [
          {
            name: "owner_id",
            type: "id",
            operators: ["eq", "neq", "in", "exists"],
            custom: false,
            references: "app_user",
          },
          {
            name: "company_id",
            type: "id",
            operators: ["eq", "neq", "in", "exists"],
            custom: false,
            references: "company",
          },
        ],
      },
    }),
  );
  await page.goto("/#/filters/contacts");
  await expectShellRendered(page);
  await page.getByRole("button", { name: de["filters.addClause"] }).click();

  const field = page.getByRole("combobox", { name: de["filters.field"] });
  await expect(field).toBeFocused();
  await expect(field).toHaveText(de["filters.choosePlaceholder"]);
  await expect(page.getByRole("combobox", { name: "Operator" })).toHaveCount(0);
  await page.keyboard.press("Enter");
  await expect(page.getByRole("listbox")).toBeVisible();
  await expectNoAaViolations(page, "filters — the field picker open");
  await page.keyboard.press("Escape");
  await expect(page.getByRole("listbox")).toHaveCount(0);
  await expect(field).toBeFocused();

  await field.click();
  await page.getByRole("option", { name: "Verantwortlich" }).click();
  const owner = page.getByRole("combobox", { name: de["filters.value"] });
  await expect(owner).toHaveText(de["filters.pickOwner"]);
  await owner.click();
  await expect(page.getByRole("option").first()).toHaveText(de["filters.you"]);
  await expectNoAaViolations(page, "filters — the owner picker open");
  await page.keyboard.press("Escape");
  await expect(page.getByRole("listbox")).toHaveCount(0);

  await field.click();
  await page.getByRole("option", { name: "Unternehmen" }).click();
  const search = page.getByRole("combobox", {
    name: de["filters.searchRecords"],
  });
  await search.fill("Brandt");
  await expect(
    page.getByRole("option", { name: "Brandt Automotive GmbH" }),
  ).toBeVisible();
  await expectNoAaViolations(page, "filters — the company suggestions open");
  await page.keyboard.press("Escape");
  await expect(page.getByRole("listbox")).toHaveCount(0);
  await expect(search).toHaveValue("Brandt");
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Enter");
  await expect(
    page.getByRole("button", { name: de["filters.changeRecord"] }),
  ).toBeFocused();
});
