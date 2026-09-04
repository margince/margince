import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";
import { de } from "../src/i18n/de";
import type { MessageKey } from "../src/i18n/en";
import { mockApi } from "./seed";

/**
 * Settle the page's motion before measuring the colours it paints.
 *
 * A contrast check reads the colours that are painted, and an element mid-fade
 * paints a blend: the login screen's staggered entry made axe see the runtime
 * line at #bfc3c1 on #fafbfa (1.71:1) instead of its settled #36433d on #eef1f0
 * (8.9:1), so the sweep failed or passed depending on how fast the machine got
 * to `networkidle`.
 *
 * WAITING for `getAnimations()` to report every finite animation finished
 * cannot settle a page. That set is empty both BEFORE the first entrance starts
 * and AFTER the last one ends, and `[].every()` is true of an empty set, so the
 * wait returns on a page whose content has not begun arriving. On a loaded
 * runner that is the ordinary outcome rather than a rare one: `.wrap > *` holds
 * each block back by up to five steps of `--stagger-enter` before fading it in
 * over `--dur-enter`, and a whole screen of half-painted text is what such a
 * wait hands to axe.
 *
 * Reduced motion settles it instead. The design system answers that with
 * `animation: none` on every arrival, which lands the content already mounted
 * AND the content still to mount at its resting colour, leaving no window to
 * race. The assertions are the half that cannot pass vacuously: an arrival that
 * outlives the guard fails here, naming itself, rather than being measured
 * mid-flight.
 */
async function settleAnimations(page: Page) {
  await page.emulateMedia({ reducedMotion: "reduce" });
  const motion = await page.evaluate(() => {
    const describe = (element: Element) =>
      `${element.tagName.toLowerCase()}.${element.className}`;
    return {
      running: document
        .getAnimations()
        .filter((animation) => animation.playState === "running")
        .map((animation) => {
          const effect = animation.effect;
          return effect instanceof KeyframeEffect &&
            effect.target instanceof Element
            ? describe(effect.target)
            : "an element the effect does not name";
        }),
      // The guard itself, read off the cascade rather than off the clock: an
      // arrival that still names an animation here will fade whenever it
      // mounts, including after this check has run. Timing cannot see that one;
      // the computed style can.
      stillArriving: [...document.querySelectorAll(".arrive, .wrap > *")]
        .filter((element) => getComputedStyle(element).animationName !== "none")
        .map(describe),
    };
  });
  expect(
    motion.stillArriving,
    "an arrival outlived the reduced-motion guard in enter.css, so content fades in whenever it mounts and axe reads the blend rather than the colours a reader sees",
  ).toEqual([]);
  expect(
    motion.running,
    "an animation kept running under prefers-reduced-motion, so axe would read a frame mid-flight",
  ).toEqual([]);
}

// B-EP09.22a/b: the AC-<screen>-N criteria as named tests — a failing test
// names the criterion it breaks. Includes the cross-cutting invariants
// (rail + ⌘K present, 🟡 confirm-first, provenance rendered), the 390px
// no-horizontal-scroll sweep (§3.8), the WCAG 2.2 AA axe gate (B-EP09.21)
// and PERF-1's held-read claim for a record open.

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

const CORE_SCREENS = [
  "home",
  "contacts",
  "companies",
  "deals",
  // The lead queue. It was in neither sweep while it grew a bulk-selection
  // bar, a board and a toolbar of its own — and it is the one record list
  // whose primary surface a reader can swap under the same address, so the
  // 390px pass has two layouts to find a horizontal scroll in rather than one.
  "leads",
  // Filters & views. It is a rail destination AC-shell-1 already asserts by
  // name, and it was in neither sweep — so the one screen in the product whose
  // body is a nested editor grew clause rows, an operator control and a preview
  // table with nobody measuring them at 390px or against axe. It is also the
  // screen most able to be WIDE: a clause is a row of three controls, and a
  // second level of nesting indents it again.
  "filters",
  "worklist",
  // Projects and Partners. Both are rail-or-off-rail list screens that now
  // print the page's name inside their table header, and both were in neither
  // sweep — so the two screens where that header is NOT measured were the two
  // whose header this change rewrote. The sweep follows the surface.
  "projects",
  "partners",
  "analytics",
  "settings",
  // The automations editor is configuration on the AI settings page now, not a
  // destination of its own. Sweeping `#/automations` after the route retired
  // would measure the fallback screen and report it as coverage, so the sweeps
  // follow the surface to where it actually lives.
  "settings/ai",
  // Settings is FIFTEEN pages behind one route (SETTINGS_TABS in
  // screens/settings.tsx), and a bare `settings` resolves to Account — the
  // shortest of them. Sweeping that alone and calling settings covered is how
  // the widest page in the product went unmeasured: `data-model` carries two
  // full list surfaces with their own toolbars, `integrations` four
  // installation-wide cards, `people` a roster of rows that each end in two
  // buttons. These are where a narrow viewport actually breaks.
  //
  // ALL fifteen. The list below is written out rather than derived, and that is
  // the weakness to know about: it went stale once already, claiming twelve
  // while three pages — capture-activity, knowledge, license — were in neither
  // sweep and nothing failed to say so. A census that can fall short reports
  // PASS on the smaller tree. The fix is to read the ids from SETTINGS_TABS,
  // which this file cannot import today without pulling the screen's whole
  // module graph through Playwright's transform.
  "settings/data-model",
  "settings/integrations",
  "settings/users",
  "settings/voice",
  "settings/agents",
  "settings/connections",
  "settings/general",
  "settings/capture",
  "settings/privacy",
  "settings/maintenance",
  "settings/capture-activity",
  "settings/knowledge",
  "settings/license",
];

/**
 * The settings page the address named is the one on screen.
 *
 * `useVisibleSettingsTabs` falls back to the first tab a principal can see, so
 * an address naming a tab this mock's grants do not cover lands on Account and
 * renders perfectly — clean axe, no overflow, and the census one page longer
 * than the tree it actually read. Two of the three pages this list gained were
 * doing exactly that. The active row in the settings level carries the tab it
 * points at, which is the cheapest thing on screen that can tell the two apart.
 */
async function expectSettingsViewLanded(page: Page, view: string) {
  if (!view.startsWith("settings/")) {
    return;
  }
  const tab = view.slice("settings/".length);
  // The trail rather than the settings nav level, because the nav level is not
  // there at 390px — the rail becomes a phone bar and the tabs go with it, and
  // an assertion that reads the rail passes vacuously on the one viewport this
  // sweep exists for. The trail's current crumb names the tab at every width.
  //
  // The expected wording comes from the catalog under the SAME key the nav
  // builds its label from, so this is not a second copy of the tab names.
  await expect(
    page.locator(".crumbs-current"),
    `#/${view} did not land on the ${tab} tab — the address fell back, so this sweep measured a page it never opened`,
  ).toHaveText(de[`settings.tab.${tab}` as MessageKey]);
}

/**
 * The page rendered, rather than the app error boundary standing in for it.
 *
 * Both sweeps below measure what is on screen, and a page that threw during
 * render puts almost nothing there: `settings/maintenance` crashed on a
 * malformed `/admin/job-health` payload and scored zero axe violations and zero
 * overflow — a dead page passing every check. The rail is the cheapest proof
 * the shell survived, because the boundary is mounted ABOVE it (main.tsx): if
 * the fallback is showing, the rail is gone.
 */
async function expectShellRendered(page: Page) {
  await expect(page.locator("nav.rail")).toBeVisible();
}

/**
 * How far the PAGE scrolls sideways, and which scroller does it.
 *
 * NOT `document.body.scrollWidth`, which is what this sweep used to read. The
 * shell pins `.main` to `overflow: hidden` (shell.css) and gives `.scroll` an
 * `overflow-y: auto` that computes `overflow-x` to `auto` with it, so wide
 * content scrolls INSIDE the page's own scroller and the body never grows a
 * pixel. Measured across all twelve settings tabs the body reported 0 while
 * `.scroll` itself overflowed by up to 273px — the assertion was structurally
 * incapable of failing, whatever the layout did.
 *
 * So this reads the two elements that actually scroll the page: the document,
 * and the shell's content column. Anything a screen spills — a header row, a
 * card, a table — spills into one of them, and it is the reader panning THOSE
 * that §3.8 forbids.
 *
 * A component that declares a horizontal scroll region of its own is not this
 * and is deliberately not measured: `.table-scroll` around a table too wide for
 * a phone (atoms.css) is the sanctioned answer to wide content, and it is
 * bounded by its card, so the page around it never moves.
 */
async function pageOverflow(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const scrollers: { name: string; element: Element }[] = [
      { name: "the document", element: document.documentElement },
      ...Array.from(document.querySelectorAll(".scroll")).map((element) => ({
        name: "the shell content scroller (.scroll)",
        element,
      })),
    ];
    return scrollers
      .map(({ name, element }) => ({
        name,
        overflow: element.scrollWidth - element.clientWidth,
      }))
      .filter(({ overflow }) => overflow > 0)
      .map(({ name, overflow }) => `${name}: ${overflow}px past the viewport`);
  });
}

/**
 * The one account affordance, at the foot of the sidebar.
 *
 * Scoped to the top bar's trailing cluster rather than found by name alone:
 * WHERE it is is half of what the restructure promises — the person sits at the
 * end of the session strip, opposite the trail that says where you are — and a
 * second account control appearing anywhere else would still satisfy a bare
 * name lookup. The name itself is deliberately a substring: the trigger carries
 * who is signed in into its accessible name (WCAG 2.5.3), so it reads
 * "<person> — Konto".
 */
function accountTrigger(page: Page) {
  return page.locator(".topbar .topbar-trail").getByRole("button", {
    name: "Konto",
  });
}

// The canonical ten, in order: Home alone, then records / work / intelligence.
// Not upstream's set: Automations is not a destination here (it is set-and-forget
// configuration on Settings → AI). One label differs from its route id on
// purpose — `deals` presents as Pipeline — so this asserts what a person reads,
// not what the router matches.
//
// The count and the list are both spelled out on purpose. NAV_GROUPS in
// src/app/nav.ts is the source of the rail; deriving this from it would assert
// only that the rail renders itself, so a destination added there is meant to
// fail here until somebody says what a person now reads and where.
//
// Heute LEADS the work group and is the only door to the work that waits on a
// person: decisions to answer, tasks to finish and duplicates to merge are lanes
// inside it rather than rows of their own.
test("AC-shell-1: the rail renders the canonical 10 items in order", async ({
  page,
}) => {
  await page.goto("/#/home");
  // evaluateAll never waits — anchor on the rendered count first, or the
  // read races the auth splash and sees an empty rail.
  // Scoped to the level the panel is showing: the DESTINATIONS are its rows,
  // while the foot's Settings door rides the same `.navitem` geometry without
  // being one of them.
  await expect(page.locator("nav.rail .navlevel a.navitem")).toHaveCount(10);
  const labels = await page
    .locator("nav.rail .navlevel a.navitem")
    .evaluateAll((links) =>
      links.map((link) => link.getAttribute("aria-label")),
    );
  expect(labels).toEqual([
    "Briefing",
    "Kontakte",
    "Firmen",
    "Leads",
    "Filter & Ansichten",
    "Arbeitsliste",
    "Pipeline",
    "Projekte",
    "Analytics",
    "Margince fragen",
  ]);
});

test("AC-shell-2: exactly one rail item is active and tracks the route", async ({
  page,
}) => {
  await page.goto("/#/deals");
  await expect(page.locator("nav.rail a.navitem.active")).toHaveCount(1);
  await expect(page.locator("nav.rail a.navitem.active")).toHaveAttribute(
    "aria-label",
    "Pipeline",
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
  await expect(page.locator(".record-head h1")).toHaveText("Anna Weber");
  // The trail that leads back to the list stands in the top bar, where it is
  // true of the page rather than part of the document the reader is reading.
  await expect(page.locator(".topbar .crumbs a").last()).toHaveText("Kontakte");
  await expect(page.locator('.topbar [aria-current="page"]')).toHaveText(
    "Anna Weber",
  );

  // An id segment that names no record is the screen's own state, so it is named
  // in WORDS and never as the slug it is addressed by: #/settings/privacy is the
  // privacy surface, and the sidebar's level beside it carries "Settings" — the
  // page said that word twice while naming the surface never. "privacy" itself
  // is a route slug no reader should ever be shown.
  await page.goto("/#/settings/privacy");
  const settingsHeading = page.getByRole("heading", { level: 1 });
  await expect(settingsHeading).toHaveCount(1);
  await expect(settingsHeading).toHaveText("Datenschutz & Audit");
  await expect(page.locator(".rail .navtitle")).toHaveText("Einstellungen");
  await expect(page.locator("main")).not.toContainText("privacy");
});

test("AC-shell-3/4/5: ⌘K opens focused+empty, filters, Enter navigates", async ({
  page,
}) => {
  await page.goto("/#/home");
  await page.locator("body").click();
  await page.keyboard.press("ControlOrMeta+k");
  const input = page.getByRole("searchbox", { name: "Befehlspalette" });
  await expect(input).toBeFocused();
  // "Deals" is the route id, not the label the rail shows (Pipeline) — typing the
  // domain word still has to land on the screen, or a relabeled destination
  // becomes unreachable for everyone who knows it by its old name.
  await input.fill("Deals");
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/#\/deals$/);
});

test("AC-shell-7: the top bar's search opens the palette", async ({ page }) => {
  await page.goto("/#/home");
  const topbar = page.locator(".topbar");
  // One search affordance in the product, and it is the centre of the session
  // strip. A BUTTON, never a field: the palette owns the query, and a second
  // input taking one here is exactly what this criterion forbids.
  await expect(topbar.locator("input")).toHaveCount(0);
  await topbar.locator(".topbar-search").click();
  await expect(
    page.getByRole("searchbox", { name: "Befehlspalette" }),
  ).toBeVisible();
  // And it is not a destination of its own — the links AC-shell-1 counts are
  // unchanged by search leaving the sidebar.
  await expect(page.locator("nav.rail .navlevel a.navitem")).toHaveCount(10);
});

// The account menu carries what belongs to the PERSON rather than to the page:
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
    menu.getByRole("menuitem", { name: "Einstellungen" }),
  ).toHaveAttribute("href", "#/settings");
  // ONE row in here navigates. Counted as anchors rather than by the link role:
  // inside a `role="menu"` every row carries `role="menuitem"`, which is what a
  // menu's keyboard contract needs and what replaces the implicit link role.
  await expect(menu.locator("a[href]")).toHaveCount(1);
  await expect(menu.getByRole("menuitem", { name: "Abmelden" })).toBeVisible();

  // Appearance is a submenu, not a control sitting open in the menu: three
  // choices spelled out flat would out-weigh the two destinations either side.
  const theme = menu.getByRole("menuitem", { name: "Design" });
  await expect(theme).toHaveAttribute("aria-expanded", "false");
  await theme.click();
  const choices = page.locator(".accountsub");
  await expect(choices.getByRole("menuitemradio")).toHaveCount(3);
  await choices.getByRole("menuitemradio", { name: "Dunkel" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await expect(
    choices.getByRole("menuitemradio", { name: "Dunkel" }),
  ).toHaveAttribute("aria-checked", "true");
});

test("features/10 §7: the locale switch flips the chrome DE↔EN", async ({
  page,
}) => {
  await page.goto("/#/home");
  await expect(page.locator('nav.rail a[aria-label="Kontakte"]')).toBeVisible();
  // The language is a preference of this person rather than a destination, so it
  // lives on Settings → Account; the account block at the sidebar foot carries
  // the three places it can take you and nothing that changes a setting. Three
  // locales ship, so the control is a list rather than a toggle — a toggle
  // cannot say where the next click lands.
  await page.goto("/#/settings/account");
  // The card the language row sits in: password, sign-off and language are one
  // account card now rather than a Preferences card of their own.
  await expect(page.getByRole("heading", { name: "Ihr Konto" })).toBeVisible();
  await page.getByRole("combobox", { name: "Sprache" }).click();
  await page.getByRole("option", { name: "English" }).click();
  // The surface around the control follows the choice, not just the control's
  // own face: every word on it is rendered from the catalog that just changed.
  await expect(
    page.getByRole("heading", { name: "Your account" }),
  ).toBeVisible();
  await expect(page.getByRole("combobox", { name: "Language" })).toBeVisible();
});

// Appearance is chosen from the account menu now — it is the setting a reader
// changes most often, and from wherever they happen to be standing. The account
// card keeps the preferences that are not appearance, so this asserts what the
// page LOST rather than that the card went away: the language control beside it
// has to still be there, or an account card that failed to render would pass.
test("features/10 §7: Settings → Account keeps language and offers no theme control", async ({
  page,
}) => {
  await page.goto("/#/settings/account");
  await expect(page.getByRole("heading", { name: "Ihr Konto" })).toBeVisible();
  await expect(page.getByRole("combobox", { name: "Sprache" })).toBeVisible();
  for (const name of ["Hell", "Dunkel", "System", "Design"]) {
    await expect(page.getByRole("button", { name, exact: true })).toHaveCount(
      0,
    );
    await expect(page.getByRole("group", { name, exact: true })).toHaveCount(0);
  }
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

test("AC-pipeline-7: board↔table swaps views preserving the deal set", async ({
  page,
}) => {
  await page.goto("/#/deals");
  // Both views draw a deal as a LINK now — a card that opens a record is an
  // anchor, so middle-click and open-in-new-tab work on it — which is why the
  // board side is identified by the card's own `data-deal` and not by its role.
  // Role used to tell the two views apart, and it cannot any more; asserting it
  // alone would pass on either view and stop testing the swap at all.
  //
  // Neither side is asserted on text alone: text says nothing about which
  // element it found, and it goes ambiguous the moment anything else on the page
  // legitimately repeats the name — the table's visually-hidden "<name>
  // auswählen" bulk-select label does, and so does the agent panel's spoken
  // status line ("Reading the <name> deal").
  //
  // The card is matched by substring on purpose: a board card's accessible name
  // is the whole card read out — name, company, value, age, badges — so the
  // deal's name is a fragment of it by construction, and the assertion is "a
  // card for this deal is on the board", not "this text is the card".
  const boardCard = page.locator('[data-deal="d-fleet"]');
  await expect(boardCard).toBeVisible();
  // The card IS the link — `data-deal` sits on the anchor — and the role is
  // asserted rather than assumed, because that is the behaviour the change
  // bought: a middle-click and an open-in-new-tab on a deal card.
  await expect(boardCard).toHaveRole("link");
  await expect(boardCard).toContainText("Fleet retrofit");
  await page.getByRole("button", { name: "Tabelle" }).click();
  // The board is gone, so its card locator is the proof the view swapped
  // rather than drew both at once.
  await expect(boardCard).toHaveCount(0);
  await expect(
    page.getByRole("link", { name: "Fleet retrofit" }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Service contract" }),
  ).toBeVisible();
});

test("AC-deal-6: a terminal-stage drop is a 🟡 confirm — nothing runs before Confirm", async ({
  page,
}) => {
  await page.goto("/#/deals");
  // Waited for by the locator the drag actually uses, so the wait and the
  // action cannot disagree about which element the test is about.
  const card = page.locator('[data-deal="d-fleet"]');
  await expect(card).toBeVisible();
  const won = page.locator('[data-stage="s4"]');
  await card.dragTo(won);
  await expect(page.getByText("Nach Won verschieben?")).toBeVisible();

  // The first Confirm is REFUSED, and that is the criterion rather than a
  // detour: this deal carries no signed contract, and a win without paper has
  // to say how it was won (deals/deal_advance.go's ensureWinEvidence). The
  // reason panel is deliberately not shown before the refusal — a win the
  // paperwork already explains stays one click, because a field every rep must
  // fill is a field every rep fills with the same lie.
  await page.getByRole("button", { name: "Bestätigen" }).click();
  await expect(page.getByText("Wie wurde er gewonnen?")).toBeVisible();
  // Still nothing has run: the drop is not applied while the dialog is open.
  await expect(page.getByText("Nach Won verschoben")).toHaveCount(0);

  await page.getByRole("combobox", { name: "Wie wurde er gewonnen?" }).click();
  await page.getByRole("option", { name: "Per Bestellung" }).click();
  await page.getByRole("button", { name: "Bestätigen" }).click();
  await expect(page.getByText("Nach Won verschoben")).toBeVisible();
});

// The decision queue lives on Today now, one decision at a time: the focus lane
// draws the staged proposal through the same ApprovalRow the retired Decisions
// screen used, so the verb a person presses is unchanged and this asserts it
// where a person now finds it.
test("AC-inbox: the staged decision is on the day's queue", async ({
  page,
}) => {
  await page.goto("/#/worklist");
  // The queue shows the decision by the sentence the SERVER composed for it,
  // not by its kind: the old lane drew "E-Mail senden" from the kind label,
  // and a queue that has to rank things across producers names each one by
  // what it actually is. Answering it stays the approvals surface's own job,
  // so the verbs are asserted where they live rather than here.
  // The sentence appears twice on purpose: once as the row, and once on the
  // card the row opens to answer it. That the decision is ANSWERABLE here is
  // the claim worth making, so this asserts the verb rather than the text.
  await expect(
    page.getByText(/Send the follow-up to Anna Weber/).first(),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Übernehmen" })).toBeVisible();
});

test("AC-book: the booking page renders rail-less with live slots", async ({
  page,
}) => {
  await page.goto("/#/book");
  await expect(page.locator("nav.rail")).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: /06\.07\.2026/ }).first(),
  ).toBeVisible();
});

test("AC-automations-1 (B-EP09.15): create from the catalog arrives paused; enable is the deliberate second step", async ({
  page,
}) => {
  // Automations are configuration, not a destination: the editor is one BODY of
  // the AI settings page, reached by its tab, so every assertion below is scoped
  // to that section rather than to a page that also carries routing, provider
  // credentials, spend and the call trace.
  await page.goto("/#/settings/ai");
  await page.getByRole("button", { name: "Automatisierungen" }).click();
  const automations = page.locator("[data-automations-admin]");
  await expect(automations.getByText("Stillstands-Erinnerung")).toBeVisible();
  await automations
    .getByRole("button", { name: "Vorlage verwenden" })
    .first()
    .click();
  // Name and parameters are one form behind the library's verb, so they are
  // asserted in the dialog it opens rather than in the region it opened from.
  const form = page.getByRole("dialog");
  // the schema default arrives in the one parameter field
  await expect(
    form.getByRole("spinbutton", { name: "due_in_days" }),
  ).toHaveValue("3");
  await form.getByRole("button", { name: "Anlegen" }).click();
  // The outcome lands on the CARD: by the time it is true the dialog that
  // produced it is gone.
  await expect(
    page.getByText("Pausiert angelegt — es läuft nichts, bis du aktivierst."),
  ).toBeVisible();
  const row = page.locator('[data-automation="au-2"]');
  // The row states its status on the control that changes it, rather than on a
  // badge beside a button whose label named the OTHER state. So the criterion
  // reads the switch: arrives off, one deliberate flip turns it on.
  const active = row.getByRole("switch", { name: "Aktiv" });
  await expect(active).toHaveAttribute("aria-checked", "false");
  await active.click();
  await expect(active).toHaveAttribute("aria-checked", "true");
});

test("AC-automations-2 (features/10 §1): anti-DSL — no free-form rule body, no user-defined trigger", async ({
  page,
}) => {
  // The anti-DSL claim is about the automations surface, so it is asserted over
  // that surface: the editor is one tab of a settings page whose other bodies
  // carry inputs with nothing to do with rule authoring, and counting those in
  // would say something else entirely.
  await page.goto("/#/settings/ai");
  await page.getByRole("button", { name: "Automatisierungen" }).click();
  const automations = page.locator("[data-automations-admin]");
  await expect(automations.getByText("Stillstands-Erinnerung")).toBeVisible();
  await automations
    .getByRole("button", { name: "Vorlage verwenden" })
    .first()
    .click();
  // The authoring form is the dialog the verb opens. The claim is about what a
  // reader can WRITE, so it is counted where the inputs are — and counted in
  // the region too, so a rule body that reappeared beside the library rather
  // than inside the dialog would still fail this.
  const form = page.getByRole("dialog");
  await expect(form.locator("textarea")).toHaveCount(0);
  await expect(automations.locator("textarea")).toHaveCount(0);
  // exactly the instance name plus the schema-derived parameter
  await expect(form.getByRole("textbox")).toHaveCount(1);
  await expect(form.getByRole("spinbutton")).toHaveCount(1);
});

test("AC-settings-16: the audit log renders attributed entries, filters live, and loads more", async ({
  page,
}) => {
  // The audit log is the last card on the Privacy & audit entry — the trail
  // that proves the consent, retention and DSR surfaces above it were honoured
  // — and it names the PERSON behind each entry (AuditEntryLine, PD-002): the
  // signed-in human reads "Du", and a machine acting under someone's authority
  // reads as THAT PERSON with the tool as a qualifier. An agent's own id is
  // never the label — attribution exists so somebody can be asked about a
  // change, and an identifier cannot be asked anything.
  await page.goto("/#/settings/privacy");
  await expect(page.getByText("Du", { exact: true })).toBeVisible();
  await expect(page.getByText("Marcus Brandt", { exact: true })).toBeVisible();
  await expect(page.getByText("über einen Agenten")).toBeVisible();
  // The agent's own identifier is not shown at all when a human stands behind it.
  await expect(page.getByText("agent:runner", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "Mehr laden" }).click();
  // A bare connector presented no grant, so there is no human to name and no
  // gap to claim — it shows what acted.
  await expect(
    page.getByText("connector:gmail", { exact: true }),
  ).toBeVisible();
  // The dials are the card's SECONDARY half — a reader arrives to read what
  // happened and narrows it second — so they sit in a disclosure closed on
  // arrival and the filter has to be opened before it can be typed in.
  await page
    .locator("details")
    .filter({ has: page.getByText("Filter", { exact: true }) })
    .locator("summary")
    .click();
  // The actor filter still speaks the API's `type:id` vocabulary, which is the
  // spelling the column itself carries.
  await page.getByRole("textbox", { name: "Akteur" }).fill("agent:runner");
  // The matching row stays AND both non-matching rows go. Asserting only that
  // the agent row is still visible would pass on a filter that did nothing —
  // it was already on screen before the filter was typed.
  await expect(page.getByText("Marcus Brandt", { exact: true })).toBeVisible();
  await expect(page.getByText("Du", { exact: true })).toHaveCount(0);
  await expect(page.getByText("connector:gmail", { exact: true })).toHaveCount(
    0,
  );
});

test("AC-settings: the passport list is metadata-only and strikes revoked rows", async ({
  page,
}) => {
  // Agent passports are a credential the PERSON holds, so they live on the
  // "Your agents" entry beside autonomy and the tool catalog — not on the
  // organization's AI page, which is spend, model prices and automations.
  await page.goto("/#/settings/agents");
  await expect(page.getByText("Marcus' Claude", { exact: true })).toBeVisible();
  const revoked = page.locator('[data-passport="pp-2"]');
  await expect(revoked.getByText("widerrufen")).toBeVisible();
  // Struck on the NAME rather than across the whole row: the row also carries
  // the dates and the standing, and a line drawn through those made the one
  // part a reader needs hardest to read.
  await expect(revoked.getByText("Alter Runner")).toHaveCSS(
    "text-decoration-line",
    "line-through",
  );
  // no token is ever re-disclosed on this surface
  await expect(page.getByText(/mgp_/)).toHaveCount(0);
});

test("AC-book-public (B-EP09.14): consent gates booking and the policy passes through verbatim", async ({
  page,
}) => {
  await page.goto("/#/book/host-1");
  await expect(page.locator("nav.rail")).toHaveCount(0);
  const slot = page.getByRole("button", { name: /06\.07\.2026/ }).first();
  await expect(slot).toBeDisabled();
  await page.getByRole("textbox", { name: "Dein Name" }).fill("Jonas Beispiel");
  await page
    .getByRole("textbox", { name: "Deine E-Mail" })
    .fill("jonas@beispiel.example");
  await expect(slot).toBeDisabled();
  await page.getByRole("checkbox").check();
  await expect(slot).toBeEnabled();
  const shownWording = await page
    .locator("[data-consent-wording]")
    .textContent();
  const requestPromise = page.waitForRequest(
    (request) =>
      request.method() === "POST" &&
      request.url().includes("/public/booking/host-1"),
  );
  await slot.click();
  const request = await requestPromise;
  const body = request.postDataJSON();
  // the wording the visitor SAW is byte-for-byte what was submitted
  expect(body.consent.wording).toBe(shownWording);
  expect(body.consent.purpose_id).toBeTruthy();
  expect(body.consent.policy_version).toBeTruthy();
  await expect(
    page.getByText("Gebucht. Die Einladung ist unterwegs."),
  ).toBeVisible();
});

test("AC-book-public-409: a taken slot degrades honestly — no fabricated confirmation", async ({
  page,
}) => {
  await page.goto("/#/book/host-1");
  await page.getByRole("textbox", { name: "Dein Name" }).fill("Jonas Beispiel");
  await page
    .getByRole("textbox", { name: "Deine E-Mail" })
    .fill("jonas@beispiel.example");
  await page.getByRole("checkbox").check();
  await page.getByRole("button", { name: /12:00/ }).click();
  await expect(
    page.getByText(
      "Die Buchung ging nicht durch — es wurde nichts eingetragen.",
    ),
  ).toBeVisible();
  await expect(page.getByText("slot no longer available")).toBeVisible();
  await expect(
    page.getByText("Gebucht. Die Einladung ist unterwegs."),
  ).toHaveCount(0);
});

test("AC-onboarding-1: onboarding is the rail-less conversational shell", async ({
  page,
}) => {
  // The onboarding wizard/stepper was replaced by the conversational shell
  // (#217): onboarding is a focused, rail-less flow whose journey is a
  // conversation thread, not a stepper.
  //
  // The flow now opens on the gate: one question, centred, before any thread
  // exists — nobody should meet the whole tool on their first screen. So the AC
  // asserts what it always meant (rail-less, no stepper) against the surface
  // that is actually first, and that the single question is the whole ask.
  await page.goto("/#/onboarding");
  await expect(page.locator("nav.rail")).toHaveCount(0);
  await expect(page.locator(".stepper")).toHaveCount(0);
  await expect(page.getByLabel("Deine Website-Adresse")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Meine Website lesen" }),
  ).toBeVisible();
  // The thread belongs to the working view, not the gate.
  await expect(
    page.getByRole("log", { name: "Einrichtungsgespräch" }),
  ).toHaveCount(0);
});

test("AC-create-1: a contact is created from the list and lands on its 360", async ({
  page,
}) => {
  await page.goto("/#/contacts");
  await page.getByRole("button", { name: "Neuer Kontakt" }).click();
  await page.getByLabel("Vollständiger Name").fill("Peter Neu");
  // Email is now a repeatable row group (P-15): add a row, then fill it.
  await page.getByRole("button", { name: "E-Mail hinzufügen" }).click();
  await page.getByLabel("E-Mail *").fill("peter@neu.example");
  await page.getByRole("button", { name: "Anlegen" }).click();
  await expect(page).toHaveURL(/#\/contacts\/p-new$/);
});

test("AC-create-2: the palette's New-deal action opens the create form; only open stages offered", async ({
  page,
}) => {
  await page.goto("/#/deals/new");
  // Scope to the create dialog: the deals list now also renders a stage FILTER
  // select (bespoke, over ALL stages) whose accessible name likewise contains
  // "Phase", so a page-wide getByLabel would ambiguously match it. The create
  // form's stage select — the subject of this AC — lives inside the modal and
  // still offers open stages only.
  const stageSelect = page.getByRole("dialog").getByLabel("Phase");
  await expect(stageSelect).toBeVisible();
  // The choices exist only while the popup is open, and the popup is portalled
  // to the body — so it is located from the page, not from inside the dialog.
  await stageSelect.click();
  const stageNames = await page
    .locator('[role="listbox"]')
    .getByRole("option")
    .allTextContents();
  expect(stageNames.filter(Boolean)).toEqual([
    "Qualify",
    "Proposal",
    "Negotiation",
  ]);
  // Looking is all this AC needs, so the list is put away the way it was
  // opened — a second press on the trigger. NOT Escape: the surrounding Modal
  // listens for it too and would take the whole create form down with it.
  await stageSelect.click();
  await expect(page.locator('[role="listbox"]')).toHaveCount(0);
  await page.getByLabel("Deal-Name").fill("Neuer Deal");
  await page.getByLabel("Wert").fill("480");
  await page.getByRole("button", { name: "Anlegen" }).click();
  await expect(page).toHaveURL(/#\/deals\/d-new$/);
});

// B-EP09.23: the mock-overlay lane — proving the system-of-record mode swap
// end to end against `mockApi(page, { sor: "overlay" })` rather than a real
// HubSpot account. Each test re-seeds on top of the global (native)
// beforeEach — Playwright resolves the most-recently-registered route first,
// so the overlay routes take over for that test only.
test.describe("B-EP09.23: overlay mode", () => {
  test("AC-overlay-1: the mode chip marks an overlay installation (and is absent under the native seed)", async ({
    page,
  }) => {
    // The native seed (this file's global beforeEach) never renders it.
    await page.goto("/#/home");
    await expect(page.locator(".badge-accent")).toHaveCount(0);

    // Same route both times, so a plain goto would be a same-document hash
    // navigation the SPA never reloads for — reload forces the fresh /me
    // read that actually picks up the newly-registered overlay routes.
    await mockApi(page, { sor: "overlay" });
    await page.reload();
    const chip = page.getByRole("link", {
      name: "Diese Installation liest Datensätze aus einem HubSpot-Spiegel statt aus nativen Tabellen. Öffne Einstellungen → Integrationen, um die Verbindung zu verwalten.",
    });
    await expect(chip).toBeVisible();
    await expect(chip).toHaveText("Liest aus HubSpot");
    // Integrations lives under the admin segment, which is the address the
    // chip has to mint: the personal Connections entry now holds only a
    // reader's own mailbox and network.
    await expect(chip).toHaveAttribute("href", "#/settings/admin/integrations");
  });

  test("AC-overlay-2: the card shows connection, sync rows and budget band", async ({
    page,
  }) => {
    await mockApi(page, { sor: "overlay" });
    await page.goto("/#/settings/integrations");
    await expect(page.getByText("Verbunden", { exact: true })).toBeVisible();
    await expect(page.getByText(/eu1/)).toBeVisible();
    // Per-object sync rows: person + organization landed fresh; deal is still
    // catching up — three distinct rows, not a collapsed summary.
    await expect(page.getByText("person", { exact: true })).toBeVisible();
    await expect(page.getByText("organization", { exact: true })).toBeVisible();
    await expect(page.getByText("deal", { exact: true })).toBeVisible();
    await expect(page.getByText("Aktuell")).toHaveCount(2);
    await expect(page.getByText("Sync ausstehend")).toBeVisible();
    // "Gesund" bands twice: the REST budget window and the per-second Search
    // window, both seeded "ok".
    await expect(page.getByText("Gesund")).toHaveCount(2);
    // The server's own "can't attribute a share" sentinel prints verbatim —
    // never a computed substitute.
    await expect(page.getByText(/~unknown/)).toBeVisible();
  });

  test("AC-overlay-3: an ordinary edit succeeds in overlay mode — the mirror write-back seam accepts it", async ({
    page,
  }) => {
    // Update writes back through the incumbent seam and succeeds
    // (overlay/provider_writes.go) — so the deal 360's Edit affordance
    // renders in overlay too (deals.tsx's DealBadges) and this drives it for
    // real: click Edit, change the name, save, and see the 360 render the
    // saved value — the same click path AC-deal-* exercises in native mode.
    await mockApi(page, { sor: "overlay" });
    await page.goto("/#/deals/d-fleet");
    await page.getByTestId("edit-record").click();
    const name = page.getByLabel("Deal-Name *");
    // Wait for the modal's own prefill to land before typing over it — the
    // form seeds its fields from the fetched record on open, and typing
    // into it before that commits races the prefill, not the write-back
    // this test is about.
    await expect(name).toHaveValue("Fleet retrofit");
    await name.fill("Fleet retrofit — expanded scope");
    await page.getByRole("button", { name: "Speichern" }).click();
    // The record's own heading, by ROLE — which is what "the edit landed"
    // means, and what stays true when a panel, a toast or a breadcrumb also
    // carries the name. The agent line in the rail already does: it says what
    // it is reading, so a bare text match finds the saved name twice and
    // cannot say which of them is the 360 rendering the write. A record page
    // has exactly one level-1 heading and it is the record's own (AC-shell-1k).
    //
    // `exact`, because the assertion is about the WHOLE name: `name` matches by
    // substring otherwise, and a heading still reading the pre-edit name would
    // pass every renaming assertion whose new name merely extends the old one.
    await expect(
      page.getByRole("heading", {
        level: 1,
        name: "Fleet retrofit — expanded scope",
        exact: true,
      }),
    ).toBeVisible();
  });

  test("AC-overlay-4: an unsupported verb explains itself rather than failing", async ({
    page,
  }) => {
    // Every refusable write affordance (advance/edit/merge/promote/
    // disqualify/create/log-activity) is deliberately HIDDEN once the SPA
    // knows it's in overlay mode — so there is no click path to a refused
    // write verb in a freshly-loaded overlay session; a naive "click it and
    // assert the copy" test is unwritable, and forcing one (or reading the
    // raw response body off a direct fetch) would only prove the mock
    // answers 422, not that the SPA does anything with it.
    //
    // The copy exists for exactly one real scenario: the stale-["me"]-cache
    // race. A screen mounts while the installation is still native (its
    // write affordances render, since the overlay gate reads the cached
    // ["me"].system_of_record.mode); another process then flips the
    // installation to overlay server-side. The SPA's own ["me"] read has a
    // 5-minute staleTime and nothing here triggers a refetch of it, so the
    // board still renders as native and the drag is still live — but the
    // request now lands on a server that refuses it. That's reproduced here:
    // load the board under the native seed (global beforeEach), THEN layer
    // the overlay mock on top with no intervening navigation/reload/
    // invalidate, so only the SERVER side (this mock's route table) has
    // flipped — the mounted screen's own state has not.
    await page.goto("/#/deals");
    await expect(page.locator('[data-deal="d-fleet"]')).toBeVisible();
    await mockApi(page, { sor: "overlay" });

    // d-fleet (stage s2, "Proposal") → s3 ("Negotiation"), both open-semantic
    // stages: an immediate advance, no confirm modal in the way (AC-deal-6
    // covers the terminal-stage confirm path separately).
    const card = page.locator('[data-deal="d-fleet"]');
    const target = page.locator('[data-stage="s3"]');
    await card.dragTo(target);

    // The board never refetched ["me"] — the Advance affordance was real,
    // still native as far as the SPA knew — but the request the server
    // actually received hit the (now overlay) mock's refused
    // POST /deals/{id}/advance, and the SPA renders the localized refusal
    // (overlay.refused), not the raw sentinel and not a generic failure.
    await expect(
      page.getByText(
        "Beim Lesen aus HubSpot nicht verfügbar — der Spiegel kann diesen Schreibvorgang nicht ausführen.",
      ),
    ).toBeVisible();
    // The deal never actually moved (the mutation errored, so nothing
    // invalidated the deals list) — the card is still in its origin column,
    // not silently accepted into a state the mirror never agreed to.
    await expect(
      page.locator('[data-stage="s2"] [data-deal="d-fleet"]'),
    ).toBeVisible();
  });

  test("overlay mode: an unsupported READ dial (list sort/filter) explains itself rather than failing", async ({
    page,
  }) => {
    // Distinct from AC-overlay-4 above: sort/filter is a refused READ dial
    // (unsupported_in_overlay_mode, compose/overlayread.go), not a refused
    // write verb — a different server code path with its own copy
    // (list.overlayReadOnly / t("overlay.filterUnsupported")), so it gets its
    // own test rather than being folded into "an unsupported verb". The list
    // toolbar never offers the controls in overlay (so the user never gets
    // to click one that can only fail); it explains the gap in place
    // instead. Search and the archived toggle are honestly still served, so
    // only the sort/filter half disappears.
    await mockApi(page, { sor: "overlay" });
    await page.goto("/#/contacts");
    await expect(
      page.getByText("Sortierung und Filter laufen über HubSpot"),
    ).toBeVisible();
    // A PREFIX, because the dial has two names: it reads "Sortieren" with no
    // order in force and "Sortierung: <Spalte>" with one. An assertion that no
    // dial is offered has to be unable to miss either, or it passes by failing
    // to look — which is what an equality test on the bare verb started doing
    // the day the dial began naming the order it holds.
    await expect(page.getByRole("combobox", { name: /^Sortier/ })).toHaveCount(
      0,
    );
    await expect(page.getByRole("searchbox")).toBeVisible();
    await expect(
      page.getByRole("checkbox", { name: "Archivierte anzeigen" }),
    ).toBeVisible();
  });

  test("AC-overlay-5: sync now reports a queued sweep", async ({ page }) => {
    await mockApi(page, { sor: "overlay" });
    await page.goto("/#/settings/integrations");
    await page.getByRole("button", { name: "Jetzt synchronisieren" }).click();
    await expect(page.getByText(/Abgleich eingereiht/)).toBeVisible();
    // Distinct from the per-object "Backfill abgeschlossen" copy already on
    // this page — this is specifically checking the sweep itself never
    // claims to be finished.
    await expect(page.getByText("Abgleich abgeschlossen")).toHaveCount(0);
  });

  test("AC-overlay-6: disconnect names the purge and the app returns to native", async ({
    page,
  }) => {
    await mockApi(page, { sor: "overlay" });
    await page.goto("/#/settings/integrations");
    // The chip is the only accent badge that is a link; the mapping card on
    // this tab wears the same badge on the row for the signed-in user, so an
    // unqualified `.badge-accent` would be counting two different things.
    const chip = page.locator("a.badge-accent");
    await expect(chip).toBeVisible();
    await page.getByRole("button", { name: "Trennen" }).click();
    await expect(
      page.getByText(
        "Dies löscht die gespiegelten Daten und schaltet die Organisation zurück auf native Datensätze.",
        { exact: false },
      ),
    ).toBeVisible();
    // Two buttons now share the label (the card's own trigger, already
    // clicked, and the modal's confirm) — the modal's is the last in the DOM,
    // the same convention overlay.test.tsx's disconnect test uses.
    const confirms = page.getByRole("button", { name: "Trennen" });
    await confirms.last().click();
    // The whole cache is invalidated on success (/me included) — the chip
    // (driven purely off /me) disappears once the app re-reads native.
    await expect(chip).toHaveCount(0);
    // The connection row survives disconnect (revoked, never deleted —
    // backend/internal/modules/overlay/teardown.go's revokeConnection), so
    // the card's own re-read must show that, not vanish or revert to
    // "active": the revoked badge plus a working Reconnect affordance.
    await expect(page.getByText("Widerrufen")).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Erneut verbinden" }),
    ).toBeVisible();
  });

  test("AC-overlay-8: an admin unmaps a user and maps them back, and each write moves the card", async ({
    page,
  }) => {
    // The whole round trip, not just the request: the seed's mapping handlers
    // mutate their own state, so each assertion below is about what the write
    // DID. A mock answering a bare 200 would let this pass having changed
    // nothing, which is the one way a mapping workflow must not be able to
    // look correct.
    await mockApi(page, { sor: "overlay" });
    await page.goto("/#/settings/integrations");

    // Seeded state: the admin's own seat, matched to a HubSpot owner by email.
    await expect(page.getByText("Über E-Mail zugeordnet")).toBeVisible();

    await page.getByRole("button", { name: "Zuordnung aufheben" }).click();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "Zuordnung aufheben" })
      .click();

    // Unmapping is confirm-first and its own standing decision: the row now
    // reports the admin's block, and the email match it replaced is gone.
    await expect(page.getByText("Von Admin aufgehoben")).toBeVisible();
    await expect(page.getByText("Über E-Mail zugeordnet")).toHaveCount(0);

    await page.getByRole("button", { name: "Zuordnen" }).click();
    await page.getByLabel("HubSpot-Nutzer suchen").fill("Lars");
    await page
      .getByRole("button", { name: "Lars Brandt · lars@brandt.example" })
      .click();

    // Mapping back is the admin's manual override, never a re-derived email
    // match — the card has to say which of the two it is.
    await expect(page.getByText("Manuell gesetzt")).toBeVisible();
    await expect(page.getByText("Von Admin aufgehoben")).toHaveCount(0);
  });

  test("AC-overlay-7: every 360 panel renders its unavailable state, never an error box", async ({
    page,
  }) => {
    await mockApi(page, { sor: "overlay" });
    const unavailable = "In der HubSpot-Ansicht nicht verfügbar";
    const errorBox = "Konnten diese Ansicht nicht laden.";

    // Person 360 (overview tab, the default): timeline, relationship
    // strength, the who-knows-them card, and the related-records context panel
    // each read a native capability the mirror doesn't hold. The interaction
    // projection is folded from natively captured participants, so an overlay
    // workspace has none — and "nobody knows them" would be a lie rather than
    // an empty answer.
    await page.goto("/#/contacts/p-anna");
    // The RECORD's own identity block, which is the page's one h1 — the shell's
    // page head yields to it on a record route and prints the trail instead.
    // `exact`, because the whole name is the assertion: `name` matches by
    // substring, so without it a heading carrying the name plus anything else
    // would pass as the record this navigation asked for.
    await expect(
      page.getByRole("heading", { level: 1, name: "Anna Weber", exact: true }),
    ).toBeVisible();
    // The person page V2 states a withheld section in its own vocabulary rather
    // than the SoR-specific copy the other 360s use, so what is asserted here is
    // what it actually promises today: the page renders, and no panel degrades
    // into an error box. That it cannot yet say "HubSpot does not carry this" —
    // a different fact from "you may not see this" — is issue #882.
    await expect(page.getByTestId("person-readings")).toBeVisible();
    await expect(page.getByText(errorBox)).toHaveCount(0);

    // Deal 360: timeline, coverage, offers, the context panel, and the buying
    // committee map. Coverage joins the interaction projection too, so it is
    // unavailable for the same reason rather than reporting a clean deal.
    //
    // FIVE. Stakeholders is not a panel of its own — the seats and the findings
    // about them are one card, and that card states the overlay case itself.
    // The fifth is the committee MAP, which draws how the deal is threaded and
    // where cover is missing: a working surface rather than a second listing of
    // the same seats, and it refuses under overlay in the same words for the
    // same reason. A picture of who is missing cannot be drawn from a store
    // that does not carry the seats.
    await page.goto("/#/deals/d-fleet");
    // `exact`, and this is the case that shows why: another test in this file
    // renames the same deal to "Fleet retrofit — expanded scope", and a
    // substring match would accept that heading as this one.
    await expect(
      page.getByRole("heading", {
        level: 1,
        name: "Fleet retrofit",
        exact: true,
      }),
    ).toBeVisible();
    // The stakeholder card is in the record's details column, which starts
    // folded. Open it before counting that card alongside the four overview
    // refusals; a closed pane is intentionally absent from the accessibility
    // tree. The switch carries the word Details rather than a bare glyph.
    await page.getByRole("button", { name: "Details" }).click();
    await expect(page.getByText(unavailable)).toHaveCount(5);
    await expect(page.getByText(errorBox)).toHaveCount(0);
  });
});

test.describe("§3.8: 390px mobile", () => {
  test.use({ viewport: { width: 390, height: 844 } });

  for (const screen of CORE_SCREENS) {
    test(`nothing scrolls sideways at 390px on #/${screen}`, async ({
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
  // FAILS TODAY, and is left failing on purpose.
  //
  // The queue's first seeded row is an approval that draws its whole decision
  // inline — evidence, draft and three answers — so its primary verb ends at
  // 864px on an 844px screen. Nothing above the queue is the cause any more:
  // the readings moved below it and the header is down to 174px. The remaining
  // 20px is the row's own height.
  //
  // The fix is to take long decisions out of the row and into the shared
  // drawer, which is the Review work rather than this screen's. Marked `fixme`
  // rather than relaxed to 900px: a ceiling tuned to today's failure is a test
  // that agrees with the defect, and this one has to go GREEN when the drawer
  // lands rather than have to be tightened by somebody who remembers to.
  test.fixme("AC-WORKLIST-SDR-01: the first primary action is above the fold at 390x844", async ({
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
      // approval row the first control is the evidence link ("Freigabe-Detail"),
      // which sits well above the Accept and Reject buttons that actually
      // answer the row. The test passed while the verb a rep presses was still
      // below the fold — a measurement of the wrong control is a green that
      // means nothing.
      //
      // `btn-primary` is how the queue marks the one emerald verb. Where a row
      // draws none — an inline decision offers several equal answers — the
      // LOWEST control is measured instead, because a row is only workable
      // when the reader can reach all of it.
      const controls = Array.from(
        row.querySelectorAll(
          ".worklist-row-verbs button, .worklist-row-verbs a, .worklist-row-dispositions button, .worklist-row-decision button",
        ),
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
  // FAILS TODAY for the same one cause, and left failing for the same reason.
  // An approval row draws 601px against a 208px ceiling.
  test.fixme("AC-WORKLIST-SDR-07: no row outgrows its ceiling at 390x844", async ({
    page,
  }) => {
    await page.goto("/#/worklist");
    await page.waitForLoadState("networkidle");
    await expect(page.locator(".worklist-list li").first()).toBeVisible();

    const tall = await page.evaluate(() => {
      return Array.from(document.querySelectorAll(".worklist-list li"))
        .map((row) => {
          const decides = row.querySelector(".worklist-row-decision") !== null;
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
  test("S-E11.2: the day's queue is workable with a thumb at 390px", async ({
    page,
  }) => {
    await page.goto("/#/worklist");
    await page.waitForLoadState("networkidle");
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

    // Every verb in the QUEUE is a real target — the rows and the focus card
    // above them, which is the work this screen exists for. The focus CTA is a
    // full-size `.btn` and already clears the floor through `--control-h`; it
    // is measured anyway, because a rule that holds only where somebody
    // remembered to look is not a floor.
    //
    // The readings strip above is deliberately NOT measured. Its "open this
    // lane" link is a 24px target on a phone and genuinely too small, but it
    // belongs to the design system's StatCard rather than to this screen —
    // fixing it here would size every reading card in the product from the
    // worklist's stylesheet. Filed as #3961.
    //
    // Visible controls only: the page carries collapsed panels whose buttons
    // lay out at zero height, and those are not targets a thumb can miss.
    const small = await page.evaluate(() => {
      const controls = document.querySelectorAll(
        ".worklist-list button, .worklist-list a.btn, .worklist-list .link-button",
      );
      return Array.from(controls)
        .filter((element) => element.getBoundingClientRect().height > 0)
        .map((element) => ({
          height: element.getBoundingClientRect().height,
          label: (element.textContent ?? "").trim(),
        }))
        .filter(({ height }) => height < 44)
        .map(({ height, label }) => `${label}: ${Math.round(height)}px`);
    });
    expect(small).toEqual([]);
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

    await page.getByRole("button", { name: "Expand the agent panel" }).click();
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
    await expectNoAaViolations(page, "home — the agent's panel (390px)");
  });

  // The whole keyboard path this surface has: it opens from the bar, Escape
  // closes it from inside, and focus lands back on the control that opened it
  // rather than on <body> — from where the next Tab starts at the top of a page
  // the reader had not left.
  test("the agent's panel closes on Escape and hands focus back to the orb", async ({
    page,
  }) => {
    await page.goto("/#/home");
    await page.waitForLoadState("networkidle");
    await expectShellRendered(page);

    const orb = page.getByRole("button", { name: "Expand the agent panel" });
    await orb.click();
    await expect(page.locator(".arpanel")).toBeVisible();

    await page.keyboard.press("Escape");
    await expect(page.locator(".arpanel")).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: "Expand the agent panel" }),
    ).toBeFocused();
  });
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
    await page.getByRole("button", { name: "Expand the agent panel" }).click();
    await expect(page.locator(".arpanel")).toBeVisible();
    await settleAnimations(page);
    await expectNoAaViolations(page, "home — the agent's panel (390px, dark)");
  });
});

/**
 * The AA sweep of one page: assert what axe DECIDED, report what it could not.
 *
 * `violations` is the gate, and it stays the gate. `incomplete` is deliberately
 * reported rather than asserted, and the reason is what the word means: axe
 * puts a node there when it cannot reach a verdict at all — a colour over a
 * gradient or an image, a pseudo-element, or (the case in this tree) a `.cf-count`
 * whose "content [is] too short to determine if it is actual text content".
 * None of those become decidable by fixing the page, so a blanket
 * `expect(incomplete).toEqual([])` could only be satisfied by deleting the
 * assertion again — and a gate that gets weakened once teaches everyone that
 * gates get weakened.
 *
 * So the undecided findings are PRINTED, with axe's own reason attached, on a
 * line a human reading the run can see. That is a real gap and it is stated as
 * one: an incomplete `color-contrast` is a colour nobody has verified, and the
 * only thing standing behind those today is the token law in tokens.css — meta
 * text takes `--textMeta`, and `--textTertiary` is for marks.
 */
// What a colour-contrast check carries when it could MEASURE the pair. Axe
// types `data` as unknown and fills it per check, so a rule that is not
// colour-contrast carries something else entirely and colour-contrast itself
// omits these when it could not resolve a background — the "partially overlaps
// other elements" findings are exactly that shape.
//
// All four are required together rather than each defaulting, because a
// half-filled reading is what this reporting exists to prevent: "#5e6c65 on
// undefined = undefined:1" reads like a measurement and is not one, and it
// would be believed. A check that cannot fill them falls back to axe's own
// sentence, which at least says why.
type ContrastReading = Readonly<{
  fgColor: string;
  bgColor: string;
  contrastRatio: number;
  expectedContrastRatio: string;
}>;

function isContrastReading(data: unknown): data is ContrastReading {
  if (typeof data !== "object" || data === null) {
    return false;
  }
  return (
    "fgColor" in data &&
    typeof data.fgColor === "string" &&
    "bgColor" in data &&
    typeof data.bgColor === "string" &&
    "contrastRatio" in data &&
    typeof data.contrastRatio === "number" &&
    "expectedContrastRatio" in data &&
    typeof data.expectedContrastRatio === "string"
  );
}

// HEADING_RULES are run BESIDE the WCAG tags rather than inside them, because
// axe tags both `best-practice` only. Neither carries a WCAG tag, so the tag
// list below ran neither on any route: the sweep read a smaller rule set than
// "the axe sweep is green" implies, with no failing assertion to say so.
//
// By id rather than by adding the `best-practice` tag, which would widen the
// sweep well past headings in one step. These two are the measured gap;
// adopting the rest of best-practice is a separate decision with its own tail.
const HEADING_RULES = ["heading-order", "page-has-heading-one"];

// TWO ANALYSES, and it has to be two.
//
// `AxeBuilder.withRules` does not ADD to `withTags` — it sets
// `runOnly: {type: "rule"}`, replacing the tag selection outright, and the
// library's own doc says the two cannot be combined. Chaining them narrows the
// sweep from every WCAG rule to these two, which is a worse version of the
// defect this exists to fix and would have reported PASS the whole way.
//
// So the tags run, the rules run, and the violations are read together.
async function axeFindings(page: Page) {
  const wcag = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
    .analyze();
  const headings = await new AxeBuilder({ page })
    .withRules(HEADING_RULES)
    .analyze();
  return {
    violations: [...wcag.violations, ...headings.violations],
    incomplete: [...wcag.incomplete, ...headings.incomplete],
  };
}

async function expectNoAaViolations(page: Page, screen: string) {
  const results = await axeFindings(page);
  const undecided = results.incomplete.flatMap((check) =>
    check.nodes.map(
      (node) =>
        `${check.id}: ${node.target.join(" ")} — ${node.any
          .map((result) => result.message)
          .join("; ")}`,
    ),
  );
  if (undecided.length > 0) {
    console.warn(
      `axe could not decide ${undecided.length} finding(s) on #/${screen}; a human has to:\n  ${undecided.join("\n  ")}`,
    );
  }
  // A violation names the SELECTOR and nothing else, which is unactionable for
  // the one class of failure this sweep actually produces: a contrast finding
  // that appears on CI and on no developer's machine. Four runs named four
  // different screens and every finding was small meta text, and none of them
  // said what colours were measured — so there was nothing to compare against
  // the tokens, and the investigation ran on hypotheses instead of readings.
  //
  // Axe already computed the pair. Reporting it costs nothing on a green run
  // and turns a red one into evidence: the resolved theme, the two colours and
  // the ratio, beside the threshold that rejected them.
  const failures = results.violations.flatMap((violation) =>
    violation.nodes.map((node) => {
      const detail = node.any
        .map((check) =>
          isContrastReading(check.data)
            ? `${check.data.fgColor} on ${check.data.bgColor} = ${check.data.contrastRatio}:1, needs ${check.data.expectedContrastRatio}`
            : check.message,
        )
        .join("; ");
      return `${violation.id}: ${node.target.join(" ")} — ${detail}`;
    }),
  );
  if (failures.length > 0) {
    // The document's own theme, not the emulated scheme: a stamped
    // `data-theme` outranks `prefers-color-scheme`, so the scheme a test asked
    // for is not proof of the palette it got.
    const painted = await page.evaluate(() => ({
      dataTheme: document.documentElement.getAttribute("data-theme"),
      prefersDark: window.matchMedia("(prefers-color-scheme: dark)").matches,
      textMeta: getComputedStyle(document.documentElement)
        .getPropertyValue("--textMeta")
        .trim(),
      bgPage: getComputedStyle(document.documentElement)
        .getPropertyValue("--bgPage")
        .trim(),
    }));
    throw new Error(
      `axe found ${failures.length} AA violation(s) on #/${screen}\n` +
        `  painted as: ${JSON.stringify(painted)}\n  ` +
        failures.join("\n  "),
    );
  }
}

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
    await expect(
      page.getByRole("button", { name: /Brandt Automotive/ }),
    ).toBeVisible();
    await settleAnimations(page);
    await expectNoAaViolations(page, "home — the command palette open");
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
  // This mock harness has no /organizations/{id}/360 route, and its fallback
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
    await page
      .getByRole("button", { name: "Qualifizieren", exact: true })
      .click();
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
    // A BUTTON, not a link: `EntityRef` opens the record through the app's own
    // hash router rather than by navigating, so what the message carries is a
    // control. Which is the point — it is focusable either way.
    const carried = said.getByRole("button").first();
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
  // `identity` is whether the identity REGION is part of the surface at that
  // width. Below 561px it is not: the phone layout is the task alone — the
  // region goes, and the Core goes with it (see the ≤560 block in auth.css),
  // because on a phone the form is the only thing the screen is for. That is a
  // deliberate reversal of Decision 1 for phones only, and this suite is the
  // record of it: pinned here rather than left to drift, because the alternative
  // is a suite that still forbids the shipped design.
  const NARROW = [
    { label: "390px mobile", width: 390, height: 844, identity: false },
    { label: "320px narrow", width: 320, height: 568, identity: false },
    { label: "200% zoom", width: 640, height: 400, identity: true },
  ] as const;

  for (const { label, width, height, identity } of NARROW) {
    test.describe(label, () => {
      test.use({ viewport: { width, height } });

      test("no horizontal body scroll", async ({ page }) => {
        await page.goto("/");
        await expect(
          page.getByRole("heading", {
            level: 1,
            name: "Bei Margince anmelden",
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

      // Where the region IS part of the surface, it shows all of itself: not one
      // of its rows may be dropped or clipped to fit (ADR-0076 Decision 6, and
      // two earlier implementations dropped rows with `display: none`). Presence
      // AND rendered height, because a node that is CSS-visible inside a
      // container hiding its overflow still passes `toBeVisible` while measuring
      // nothing — which is exactly how the copy came to be cut off at this width
      // once already.
      //
      // Where it is NOT — the phone layout — the region is absent by design and
      // what remains is the task alone: no aside, no sphere, and no second copy
      // of the region's copy either. The phone surface is the form (founder
      // ruling, 2026-08-07): the disclosure the aside makes is a property of the
      // region, and at this width the region is not on the screen for it to be a
      // property of.
      //
      // Named ROW BY ROW rather than counted: the copy is one paragraph in one
      // voice, so a row missing from the middle of it is a sentence the system
      // stopped saying, and a count would pass on any six elements.
      const identityRows = [
        ".auth-kicker",
        ".auth-statement",
        ".auth-purpose",
        ".auth-scope",
        ".auth-promise",
        ".auth-handover",
      ];

      test("shows the identity region whole, or not at all", async ({
        page,
      }) => {
        await page.goto("/");
        const region = page.locator("aside.auth-identity");
        if (identity) {
          await expect(region).toBeVisible();
          for (const row of identityRows) {
            const line = page.locator(row);
            await expect(line).toHaveCount(1);
            await expect(line).toBeVisible();
            const box = await line.boundingBox();
            expect(box?.height ?? 0).toBeGreaterThan(0);
          }
        } else {
          await expect(region).toBeHidden();
          // Hidden, not merely present: the sphere is drawn by the same markup at
          // every width, so a count would pass on a Core the phone layout still
          // shows — which is the thing this width is supposed to be free of.
          await expect(page.locator("[data-core-state]")).toBeHidden();
          // NONE of the region's copy survives either, named part by part: the
          // region hides as one box, so a future rule that lifted a line of it
          // back into the task column would leave this width claiming to be the
          // form alone while a sentence about the AI sat above the fields.
          for (const row of identityRows) {
            await expect(page.locator(row)).toBeHidden();
          }
          // The class, not the tag: `RaillessFrame` already wraps every
          // rail-less screen in a `<main>`, so the task region is a `<div>` —
          // a second `<main>` here would be an invalid, duplicate landmark.
          await expect(page.locator(".auth-task")).toBeVisible();
        }
      });
    });
  }

  // Stacked below 960px, and the task comes FIRST — visually as well as in the
  // DOM, because at this width there is no second column for `order` to move.
  test("stacks the task region above the identity region below 960px", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 720, height: 900 });
    await page.goto("/");
    await expect(
      page.getByRole("heading", { level: 1, name: "Bei Margince anmelden" }),
    ).toBeVisible();
    // The class, not the tag: see the note beside the other `.auth-task`
    // locator above — one `<main>` per screen, and it belongs to the frame.
    const task = await page.locator(".auth-task").boundingBox();
    const identity = await page.locator("aside.auth-identity").boundingBox();
    expect(task).not.toBeNull();
    expect(identity).not.toBeNull();
    expect(task?.y ?? 0).toBeLessThan(identity?.y ?? 0);
  });

  // §6.4 / §12: one h1, and it is the TASK. A surface whose h1 is the system
  // talking and whose h2 is "sign in" has inverted its own hierarchy — and the
  // identity region's statement is set large enough that promoting it to a
  // heading is a tempting mistake.
  test("has exactly one h1, and it is the task", async ({ page }) => {
    await page.goto("/");
    const headings = page.getByRole("heading", { level: 1 });
    await expect(headings).toHaveCount(1);
    await expect(headings).toHaveText("Bei Margince anmelden");
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
      page.getByRole("heading", { level: 1, name: "Bei Margince anmelden" }),
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
// This case bounds `GET /people/{id}`, and the title says so because that is the
// read it holds. The heading itself comes from `/people/{id}/360` — a record
// head that draws before ITS own read returns is the wider claim, and #2864
// carries it, product half first.
//
// The perceived BUDGET is not asserted here at all. One wall-clock sample says
// how busy the runner was, and this lane shares its machine with six integration
// shards. `make bench-mobile` owns the 300ms figure as a p95 over 20 samples on
// a throttled Fast-3G profile — the harder of the two conditions, so a budget
// that holds there holds unthrottled by construction.
test("PERF-1: a record's heading does not wait on GET /people/{id}", async ({
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
  await page.route("**/people/p-anna", async (route) => {
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
  // person as a table row, and a bare text match would also take any other
  // element that legitimately repeats the name (the agent panel's spoken line,
  // a bulk-select label) without saying which one it clicked. Substring on
  // purpose — a row's accessible name is every cell of it joined, so the person's
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

// AC-filters-and-views: authoring a dynamic list's filter in the product.
//
// The screen shipped without entering either sweep and with no criterion of its
// own asserted, which is why #1468 could read as open against a surface that had
// been live for weeks: nothing in this file mentioned it, so nothing here
// contradicted the report. Adding `filters` to CORE_SCREENS above puts it in the
// 390px and axe passes — which found a 257px overflow and two 1.5:1 captions on
// their first run. These four say what the screen is FOR.
//
// Four rather than eight. 2, 6 and 7 are cited nowhere in this tree, and a test
// naming a criterion whose text nobody here can read would assert whatever I
// guessed it said.
test.describe("filters and views", () => {
  // Every clause below is authored the way a person authors one — through the
  // picker — rather than by seeding a tree in code. "A human can build this" is
  // the claim, and a tree set in code is one no human went through.
  async function authorIndustryIs(page: Page) {
    await page.getByRole("button", { name: "Bedingung hinzufügen" }).click();
    await page.getByRole("combobox", { name: "Feld" }).click();
    await page.getByRole("option", { name: "industry" }).click();
    await page.getByRole("combobox", { name: "Operator" }).click();
    await page.getByRole("option", { name: "ist", exact: true }).click();
    await page.getByRole("textbox", { name: "Wert" }).fill("automotive");
  }

  test("AC-filters-and-views-3: a clause is a field, an operator its type admits, and a value", async ({
    page,
  }) => {
    await page.goto("/#/filters/companies");
    await expectShellRendered(page);

    // Before anything is authored the count says so, rather than showing a zero
    // that would read as "no companies match".
    await expect(
      page.getByText("Bedingung hinzufügen, um die Treffer zu sehen"),
    ).toBeVisible();

    await page.getByRole("button", { name: "Bedingung hinzufügen" }).click();

    // The field picker is the SERVER's vocabulary, not a list this screen keeps:
    // `industry` and `lifecycle` are organization fields, `tag` is the leaf that
    // is an EXISTS over a join rather than a column, and none of them is
    // spelled anywhere in the frontend.
    await page.getByRole("combobox", { name: "Feld" }).click();
    expect(await page.getByRole("option").allTextContents()).toEqual([
      "owner id",
      "industry",
      "lifecycle",
      "tag",
      // The custom field, and it sorts after the core ones — a reader scanning
      // for a column they added finds it in one place rather than interleaved.
      "fleet size",
    ]);
    await page.getByRole("option", { name: "industry" }).click();

    // And the operator set is the FIELD's. `industry` is text, so `enthält` is
    // offered; the tag clause in AC-4 proves the narrowing by its absence.
    await page.getByRole("combobox", { name: "Operator" }).click();
    expect(await page.getByRole("option").allTextContents()).toEqual([
      "ist",
      "ist nicht",
      "ist eines von",
      "enthält",
      "hat einen Wert",
    ]);
    await page.getByRole("option", { name: "ist", exact: true }).click();

    await page.getByRole("textbox", { name: "Wert" }).fill("automotive");

    // The count is the SERVER's — 812 is the fixture's match_count, a number no
    // arithmetic on the two returned rows could produce.
    await expect(page.getByText("812 Firmen treffen zu")).toBeVisible();
  });

  // #1286 made custom fields and tags selectable beside core fields, and the
  // issue's own "done when" names that: a picker that offered only core fields
  // would satisfy every other case here. The badge is the half a reader needs —
  // `cf_fleet_size` and a core column look identical without it.
  test("AC-filters-and-views-2: a custom field is offered beside the core ones, and says it is one", async ({
    page,
  }) => {
    await page.goto("/#/filters/companies");
    await expectShellRendered(page);
    await page.getByRole("button", { name: "Bedingung hinzufügen" }).click();
    await page.getByRole("combobox", { name: "Feld" }).click();
    await page.getByRole("option", { name: "fleet size" }).click();

    // The badge is beside the picker rather than inside the option, which is
    // where it has to be: a chosen field's option is no longer on screen, and
    // "this is a column somebody here added" is the fact a reader needs while
    // reading the clause, not while opening the menu.
    await expect(page.getByText("eigenes Feld")).toBeVisible();

    // Its operators are its TYPE's — number, so the comparisons appear, and
    // `enthält` does not. A picker that offered a fixed set here would produce
    // a 422 the reader could not interpret.
    await page.getByRole("combobox", { name: "Operator" }).click();
    expect(await page.getByRole("option").allTextContents()).toEqual([
      "ist",
      "ist nicht",
      "ist größer als",
      "ist mindestens",
      "ist kleiner als",
      "ist höchstens",
      "ist eines von",
      "hat einen Wert",
    ]);
  });

  test("AC-filters-and-views-4: clauses combine, the group names the join, and a linked field is narrowed", async ({
    page,
  }) => {
    await page.goto("/#/filters/companies");
    await expectShellRendered(page);

    // The join control is present before a second clause exists, because it is a
    // property of the GROUP rather than of having two of anything.
    const joins = page.getByRole("group", {
      name: "Wie diese Gruppe ihre Bedingungen verknüpft",
    });
    await expect(joins).toHaveCount(1);
    await expect(
      joins.getByRole("button", { name: "ALLE · UND", pressed: true }),
    ).toBeVisible();
    await joins.getByRole("button", { name: "BELIEBIGE · ODER" }).click();
    await expect(
      joins.getByRole("button", { name: "BELIEBIGE · ODER", pressed: true }),
    ).toBeVisible();

    await page.getByRole("button", { name: "Bedingung hinzufügen" }).click();
    await page.getByRole("button", { name: "Bedingung hinzufügen" }).click();
    await expect(page.getByRole("combobox", { name: "Feld" })).toHaveCount(2);

    // A tag leaf is reached through a join, so the engine narrows it to the link
    // operators — `enthält` is gone, and a picker that offered it would produce
    // a 422 the reader could not interpret.
    await page.getByRole("combobox", { name: "Feld" }).first().click();
    await page.getByRole("option", { name: "tag" }).click();
    await page.getByRole("combobox", { name: "Operator" }).first().click();
    expect(await page.getByRole("option").allTextContents()).toEqual([
      "ist",
      "ist nicht",
      "ist eines von",
      "hat einen Wert",
    ]);
    await page.keyboard.press("Escape");

    // And nesting: a group inside a group is what mixing AND with OR needs.
    await page.getByRole("button", { name: "Gruppe hinzufügen" }).click();
    await expect(joins).toHaveCount(2);
  });

  test("AC-filters-and-views-5: the preview shows matching records, and says it is a page of them", async ({
    page,
  }) => {
    await page.goto("/#/filters/companies");
    await expectShellRendered(page);
    await authorIndustryIs(page);

    await expect(
      page.getByRole("heading", { level: 2, name: "Passende Datensätze" }),
    ).toBeVisible();
    // By CELL: the shell's own workspace strip carries the same company name,
    // and a bare text match would pass on the chrome while the table was empty.
    await expect(
      page.getByRole("cell", { name: "Brandt Automotive", exact: true }),
    ).toBeVisible();
    // EVERY row satisfies the filter, and that is asserted rather than assumed.
    // A preview is what a reader checks a filter against before saving it as a
    // list, so a row the predicate excludes is the one thing it must never
    // show — and a test naming only the row it expected would pass over exactly
    // that.
    await expect(
      page.getByRole("cell", { name: "Kessler Fahrzeugbau", exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("cell", { name: "automotive", exact: true }),
    ).toHaveCount(2);
    await expect(page.getByRole("cell", { name: "logistics" })).toHaveCount(0);
    // The caption that keeps the page honest: 812 match and two are shown, so
    // the reader is told this is a sample rather than the selection.
    await expect(
      page.getByText(
        "Die erste Seite der Treffer — genug, um den Filter zu prüfen, nicht die gesamte Auswahl.",
      ),
    ).toBeVisible();
  });

  test("AC-filters-and-views-1: the object tab is part of the address", async ({
    page,
  }) => {
    await page.goto("/#/filters/deals");
    await expectShellRendered(page);
    const objects = page.getByRole("group", {
      name: "Welche Datensätze gefiltert werden",
    });
    await expect(
      objects.getByRole("button", { name: "Geschäfte", pressed: true }),
    ).toBeVisible();

    await objects.getByRole("button", { name: "Kontakte" }).click();
    await expect(page).toHaveURL(/#\/filters\/contacts$/);

    // Reloaded, not just navigated: the tab is where you ARE, so a shared link
    // and a refresh have to land on the same object.
    await page.reload();
    await expect(
      objects.getByRole("button", { name: "Kontakte", pressed: true }),
    ).toBeVisible();
  });
});
