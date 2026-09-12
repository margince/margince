/** @vitest-environment jsdom */
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { displayVersion, narrowVersion } from "./release";
import { navigate, type Route } from "./router";
import { Shell, WorkspaceRail } from "./shell";
import {
  fixtureSection,
  ignoreSearch,
  newClient,
  render,
  renderWith,
  stubPhoneViewport,
} from "./testing/shellharness";

// A composed installation, because the vanilla registry is empty by
// construction: the case below that stands on a composed unit's screen is
// asserting what the rail does when the DESCRIPTOR resolves, and against an
// empty registry it would silently be the uncomposed case instead. The
// descriptor shape is the generator's, the same one App.extscreen.test.tsx hands
// the same lookup.
vi.mock("@composition/extensions", () => ({
  extensions: [
    {
      name: "notes",
      verbs: [
        {
          operationId: "notesList",
          route: "/ext/notes/list",
          method: "POST",
          title: "List demo notes",
          version: "1.0.0",
          rbacObject: "ext_notes_note",
        },
      ],
    },
  ],
}));

// B-EP09.4 acceptance, for the SIDEBAR — the left-hand panel and nothing else.
//
// It is destinations only: the canonical 13-item nav in order (AC-shell-1b —
// Automations left it for Settings → AI while the dedupe queue and the filter
// builder took rows, which is a UI divergence on the founder's back-fill list),
// at most one active
// item tracking the route (AC-shell-2), badges only on the attention screens and
// only from live counts (AC-shell-1e), collapsed rows on the expanded rows' own
// geometry with a dismissible tooltip (AC-shell-1d), and the phone bar with its
// More sheet. It carries no search row, no collapse control, no Settings door
// and no account block any more — each of those moved to the top bar
// (topbar.test.tsx).
//
// The second suite is the panel's own DEPTH: a section route replaces the
// destinations with that section's entries, one level at a time, with the way
// back up in the panel. A few of those cases drive the whole Shell, because
// where the reader walked down FROM is remembered above the rail and a rail on
// its own always answers "brief".
//
// What the shell composes around this panel — the page title, the top bar's
// mounting, sign-out — is shell.test.tsx's.

// Only what a level hides needs the shell's real stylesheet in the document
// (see mountShellStyles); it outlives cleanup(), so it is taken down here.
let shellStyles: HTMLStyleElement | undefined;

afterEach(() => {
  cleanup();
  shellStyles?.remove();
  shellStyles = undefined;
  window.location.hash = "";
  vi.unstubAllGlobals();
  // The sidebar remembers its width in localStorage, which jsdom keeps for the
  // whole file: without this a case that collapses the sidebar hands the next
  // one a collapsed shell, and the next one's failure names a control that is
  // simply in its other state. Cleared after the unstub, so it clears the real
  // storage rather than a stub that is about to be thrown away.
  window.localStorage.clear();
});

// The words a reader sees, in the order the sidebar shows them: the four record
// types first, then the surfaces that work over them, then what reads them back.
// A label is not a route id — `ai` presents as Ask Margince, and no assertion
// here may be satisfied by a route id that happens to match.
const CANONICAL_ORDER = [
  "Brief",
  "Contacts",
  "Companies",
  "Leads",
  "Deals",
  "Worklist",
  "Projects",
  "Filters & views",
  "Analytics",
  "Ask Margince",
];

// The rows of whatever level the panel is showing — the destinations, or a
// section's entries. Scoped to `.navlevel` rather than to the whole nav, because
// the brand above the level and the More control below it are not places the
// level leads: a claim about the level's inventory must not move when either of
// them does.
const levelLabels = () => {
  const level = document.querySelector<HTMLElement>(".navlevel");
  if (!level) {
    throw new Error("the rail rendered no navigation level at all");
  }
  return within(level)
    .getAllByRole("link")
    .map((link) => link.getAttribute("aria-label"));
};

// What a level does to the rail's head is CSS, and nothing applies a stylesheet
// in this environment unless it is in the document. It is the SHELL's own
// stylesheet that goes in — a rule copied into the test would prove only that
// the copy says what the test says. The phone block does not apply: these
// queries are widths, and the window here is wider than 700px.
const SHELL_CSS = readFileSync(
  join(dirname(fileURLToPath(import.meta.url)), "shell.css"),
  "utf8",
);

function mountShellStyles(): HTMLStyleElement {
  const style = document.createElement("style");
  style.textContent = SHELL_CSS;
  document.head.append(style);
  return style;
}

// A row that is not in the rail at all is not the same thing as a hidden one,
// and must not read as one: the head keeps its elements and the level takes
// their space.
function railDisplay(container: HTMLElement, selector: string): string {
  const node = container.querySelector(selector);
  if (!node) {
    throw new Error(`${selector} is missing from the rail entirely`);
  }
  return getComputedStyle(node).display;
}

describe("WorkspaceRail (AC-shell-1/2)", () => {
  it("renders the canonical 13 items in order, logomark → brief", () => {
    render(<WorkspaceRail route={{ screen: "deals" }} />);
    const brand = within(
      screen.getByRole("navigation", { name: "Primary navigation" }),
    ).getByRole("link", {
      name: "Margince",
    });
    expect(brand.getAttribute("href")).toBe("#/brief");
    // The DESTINATIONS are the level's own rows, so they are counted there: the
    // brand above it and the More control below it are neither of them, and each
    // is asserted where it belongs.
    expect(levelLabels()).toEqual(CANONICAL_ORDER);
    // The mark leads them, which is what "logomark → brief" means.
    const brief = screen.getByRole("link", { name: "Brief" });
    expect(
      brand.compareDocumentPosition(brief) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeGreaterThan(0);
  });

  it("groups the items under Records / Work / Intelligence when expanded", () => {
    render(<WorkspaceRail route={{ screen: "brief" }} />);
    const headings = screen
      .getAllByRole("heading", { level: 2 })
      .map((heading) => heading.textContent);
    expect(headings).toEqual(["Records", "Work", "Intelligence"]);
  });

  // Collapsed, a group heading has no word to show — 56px carries a glyph and
  // not a label — so it is not drawn, and the break between two groups is what
  // says where one ends. It stays in the DOCUMENT either way: the outline a
  // screen reader walks does not depend on how wide the reader left the panel.
  //
  // Asserted as a COMPARISON of the two states rather than as one absolute, for
  // the reason the brand head's own pair gives: two independent expectations
  // would both pass against a heading that had quietly stopped being drawn in
  // either state.
  it("draws the group headings expanded and not collapsed", () => {
    shellStyles = mountShellStyles();
    const expanded = render(<WorkspaceRail route={{ screen: "brief" }} />);
    const collapsed = render(
      <WorkspaceRail route={{ screen: "brief" }} collapsed />,
    );
    expect(railDisplay(expanded.container, ".navheading")).not.toBe("none");
    expect(railDisplay(collapsed.container, ".navheading")).toBe("none");
    // Both panels still publish all three, whichever width they are at.
    for (const rail of [expanded, collapsed]) {
      expect(
        [...rail.container.querySelectorAll(".navheading")].map(
          (heading) => heading.textContent,
        ),
      ).toEqual(["Records", "Work", "Intelligence"]);
    }
  });

  it("marks exactly one item active, matching the route", () => {
    render(<WorkspaceRail route={{ screen: "deals" }} />);
    const active = screen
      .getAllByRole("link")
      .filter((link) => link.getAttribute("aria-current") === "page");
    expect(active).toHaveLength(1);
    expect(active[0].getAttribute("aria-label")).toBe("Deals");
  });

  // Settings is not a destination and no longer has a door here, so a route the
  // destinations do not hold — the settings screen itself, a composed unit's screen —
  // leaves every row quiet rather than lighting one that does not lead there. It
  // is the top bar's trail that says where the reader is on those routes
  // (topbar.test.tsx); what the rail promises is that it never claims a page it
  // is not showing. A RENDER assertion on purpose: the route→activeId half is a
  // string that resolves to a rendered element or to nothing at all, and a
  // data-level test reads identically either way.
  it.each([
    ["a settings route", { screen: "settings" }],
    ["a composed unit's screen", { screen: "ext", id: "notes" }],
  ] satisfies readonly (readonly [string, Route])[])(
    "claims no destination on %s",
    (_case, route) => {
      render(<WorkspaceRail route={route} />);
      expect(levelLabels()).toEqual(CANONICAL_ORDER);
      expect(document.querySelectorAll('[aria-current="page"]')).toHaveLength(
        0,
      );
    },
  );

  // AC-shell-1e: a badge counts only what wants attention, and the primary level
  // declares no such row today (BADGE_SCREENS is empty — the queues that had
  // counts are lanes inside Today, which reports them on the page). A count for
  // ANY destination must therefore draw nothing: this bites if BADGE_SCREENS is
  // dropped and every screen starts rendering ambient totals again.
  it("draws no badge on the primary level, whatever counts it is given", () => {
    const { container } = render(
      <WorkspaceRail
        route={{ screen: "brief" }}
        counts={{ today: 4, deals: 13, leads: 7, contacts: 248 }}
      />,
    );
    expect(container.querySelectorAll(".count")).toHaveLength(0);
  });

  // AC-shell-1c/1d: collapsed items are icon-only, so the label must reach a
  // screen reader via aria-label in BOTH states, and the visible tooltip must
  // appear on keyboard focus (not hover alone) and be dismissible with Escape.
  it("keeps the accessible name when collapsed and shows a dismissible tooltip on focus", async () => {
    const user = userEvent.setup();
    render(<WorkspaceRail route={{ screen: "brief" }} collapsed />);
    const deals = screen.getByRole("link", { name: "Deals" });
    expect(screen.queryByRole("tooltip")).toBeNull();

    deals.focus();
    const tip = await screen.findByRole("tooltip");
    expect(tip.textContent).toBe("Deals");

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("tooltip")).toBeNull();
    // Escape dismisses the tooltip without moving focus (WCAG 1.4.13).
    expect(document.activeElement).toBe(deals);
  });

  // WCAG 1.4.13 also requires the tooltip be HOVERABLE: reaching for it must not
  // dismiss it. What makes that true is CONTAINMENT — the tooltip is a descendant
  // of the row it belongs to, so a pointer moving onto it never leaves the row —
  // and containment is what this asserts. A sibling tooltip would vanish under
  // the cursor and no assertion on its text would notice.
  //
  // The hover itself is deliberately NOT simulated: user-event dispatches
  // `mouseleave` on the row when the pointer moves to a child of it, which a
  // browser does not do, so a pass there would measure the simulator and a
  // failure would report a defect that is not in the product.
  it("nests the collapsed tooltip inside its own row so hovering it cannot dismiss it", async () => {
    const user = userEvent.setup();
    render(<WorkspaceRail route={{ screen: "brief" }} collapsed />);
    const deals = screen.getByRole("link", { name: "Deals" });

    await user.hover(deals);
    const tip = await screen.findByRole("tooltip");
    expect(deals.contains(tip)).toBe(true);
    expect(tip.parentElement).toBe(deals);
  });

  // On a phone the four bar tabs are the only rows rendered, so a route living
  // in the More sheet has nothing to carry the current-destination state. More
  // carries it instead, or the bar shows no active tab at all.
  it("marks More as the active tab for a destination the phone bar hides", () => {
    const sheeted = render(<WorkspaceRail route={{ screen: "analytics" }} />);
    const more = sheeted.container.querySelector(".railmore.active");
    expect(more).not.toBeNull();
    // Announced, not merely tinted: the hidden route's own link is out of the
    // accessibility tree at phone width, so More has to report the current page.
    expect(more?.getAttribute("aria-current")).toBe("page");
    cleanup();

    const onBar = render(<WorkspaceRail route={{ screen: "contacts" }} />);
    const inactive = onBar.container.querySelector(".railmore");
    expect(inactive?.className).not.toContain("active");
    expect(inactive?.getAttribute("aria-current")).toBeNull();
  });

  // Open, the sheet renders the real row for that route, which carries
  // aria-current itself. Two elements claiming the current page is worse than
  // the visual-only state this replaced.
  it("hands the current-page claim back to the real row once the sheet is open", async () => {
    const user = userEvent.setup();
    // The sheet exists only at phone width — the control that opens it is not
    // rendered above the breakpoint, and the rail closes any sheet it finds
    // itself holding there.
    stubPhoneViewport();
    const { container } = render(
      <WorkspaceRail route={{ screen: "analytics" }} />,
    );
    await user.click(screen.getByRole("button", { name: "More" }));
    expect(
      container.querySelector(".railmore")?.getAttribute("aria-current"),
    ).toBeNull();
    expect(container.querySelectorAll('[aria-current="page"]')).toHaveLength(1);
  });

  // The half that is easy to break: the marker has to survive the collapse. At
  // 56px the rail drops every label it has, so a build badge that rode the
  // wordmark would be gone exactly where the product is hardest to identify —
  // and it is the one thing here a reader may need to read back to us.
  //
  // Read from `release.ts` rather than typed out: the badge and the version are
  // one answer, and a literal here would go on passing after a release changed
  // it. The narrow form is the same string with one word abbreviated, so the two
  // cannot drift into naming different builds.
  it("stamps the build at both rail widths, one glyph narrower collapsed", () => {
    const expanded = render(<WorkspaceRail route={{ screen: "brief" }} />);
    expect(expanded.container.querySelector(".ws-alpha")?.textContent).toBe(
      displayVersion(),
    );
    cleanup();

    const collapsed = render(
      <WorkspaceRail route={{ screen: "brief" }} collapsed />,
    );
    expect(collapsed.container.querySelector(".ws-alpha")?.textContent).toBe(
      narrowVersion(),
    );
    // The abbreviation is what the 56px column buys, said once here so a
    // `narrowVersion` that stopped abbreviating fails rather than passing
    // against itself.
    expect(narrowVersion()).not.toBe(displayVersion());
  });

  // The badge is not inside the link. A fact about the build sitting in an
  // anchor is a fact a press carries the reader away from, and the head's one
  // target is the mark.
  it("keeps the build badge out of the brand's link", () => {
    const { container } = render(<WorkspaceRail route={{ screen: "brief" }} />);
    const badge = container.querySelector(".ws-alpha");
    expect(badge).toBeTruthy();
    expect(badge?.closest("a")).toBeNull();
    // And the marker the foot used to carry is gone with it.
    expect(container.querySelector(".railversion")).toBeNull();
  });

  // The bar is five cells and only three of them are destinations. The agent is
  // the middle one — it reports rather than navigates, so it is not filed among
  // the places to go — and it stands in the ROW STREAM rather than beside it,
  // because a cell placed by a grid column alone is third to the eye and last to
  // the Tab key. What this asserts is that ONE order describes both.
  it("puts the agent in the middle of the phone bar, in the order a thumb reads", () => {
    stubPhoneViewport();
    const { container } = render(<WorkspaceRail route={{ screen: "brief" }} />);
    const nav = container.querySelector(".rail");
    const cells = [
      ...(nav?.querySelectorAll(".navwrap.primary, .arblock, .railmore") ?? []),
    ].map((cell) =>
      cell.classList.contains("navwrap")
        ? (cell.querySelector(".navitem")?.getAttribute("aria-label") ?? "")
        : cell.classList.contains("arblock")
          ? "agent"
          : "more",
    );
    expect(cells).toEqual(["Brief", "Contacts", "agent", "Deals", "more"]);
  });

  // The Worklist is the destination the bar gave up for that cell. Off the bar
  // is not gone: it is a row in the sheet like every other destination the bar
  // cannot carry, and More reports it as the current page while it is open.
  it("keeps the Worklist off the bar and in the sheet", async () => {
    const user = userEvent.setup();
    stubPhoneViewport();
    const { container } = render(
      <WorkspaceRail route={{ screen: "worklist" }} />,
    );
    expect(container.querySelectorAll(".navwrap.primary")).toHaveLength(3);
    expect(
      container.querySelector(".railmore.active")?.getAttribute("aria-current"),
    ).toBe("page");

    await user.click(screen.getByRole("button", { name: "More" }));
    expect(levelLabels()).toContain("Worklist");
  });

  // The agent is a cell of the BAR, and the bar is not on screen while the sheet
  // is. It is not re-parented into the sheet either: a row for it in a list of
  // ten places would file the agent as an eleventh place to go, which is what
  // taking it out of that list was for. One block, still mounted, still one Core.
  it("keeps exactly one agent when the sheet takes the bar's place", async () => {
    const user = userEvent.setup();
    stubPhoneViewport();
    const { container } = render(<WorkspaceRail route={{ screen: "brief" }} />);
    expect(container.querySelectorAll(".arblock")).toHaveLength(1);

    await user.click(screen.getByRole("button", { name: "More" }));
    expect(container.querySelectorAll(".arblock")).toHaveLength(1);
  });

  // The sheet is the phone's whole sidebar, and it is navigation and nothing
  // else: the account menu lives in the top bar, which is on screen at this
  // width too. A second copy of it down here would be two account affordances
  // one scroll apart, which is what the sidebar's own foot used to be.
  it("opens a sheet of destinations and no second account affordance", async () => {
    const user = userEvent.setup();
    stubPhoneViewport();
    const { container } = render(<WorkspaceRail route={{ screen: "brief" }} />);

    await user.click(screen.getByRole("button", { name: "More" }));
    expect(levelLabels()).toEqual(CANONICAL_ORDER);
    expect(container.querySelector(".accountrows")).toBeNull();
    expect(screen.queryByRole("button", { name: /Account$/ })).toBeNull();
    expect(screen.queryByRole("button", { name: "Sign out" })).toBeNull();
  });

  // The sheet takes focus when it opens, so dismissing it from INSIDE (Escape, a
  // click outside) leaves focus on a row that is about to be gone. It goes back
  // to the control that opened it rather than onto <body> — and only then: a rail
  // that merely mounts with a row focused must keep that focus where it is.
  it("hands focus back to More when the sheet is dismissed from inside it", async () => {
    const user = userEvent.setup();
    stubPhoneViewport();
    render(<WorkspaceRail route={{ screen: "brief" }} />);
    await user.click(screen.getByRole("button", { name: "More" }));
    expect(document.activeElement).toBe(
      screen.getByRole("link", { name: "Brief" }),
    );

    await user.keyboard("{Escape}");
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "More" }),
    );
  });
});

// The sidebar's SECOND level (and its third). A section route replaces the
// destinations with the section's entries rather than hanging them off it: one
// level at a time, with the way back up in the panel. The depth is read off the
// DATA, which is what the synthetic third level in `fixtureSection` is for —
// settings, the only real section the app ships, is two levels deep.
describe("Rail levels (a section's entries as the second level)", () => {
  it("replaces the destinations with the section's entries, one current", () => {
    render(
      <WorkspaceRail
        route={{ screen: "settings", id: "account" }}
        section={fixtureSection("account")}
      />,
    );
    // The destinations are GONE, not pushed below a second list: 56px cannot
    // carry two levels and 224px carrying both is a list of twenty places to go.
    expect(screen.queryByRole("link", { name: "Deals" })).toBeNull();
    expect(levelLabels()).toEqual(["Account", "Privacy & retention"]);
    expect(
      screen
        .getByRole("link", { name: "Privacy & retention" })
        .getAttribute("href"),
    ).toBe("#/settings/deep");
    // Exactly one row claims the current page, and it is the entry the SECTION
    // resolved — the screen owns that answer, fallbacks included.
    const current = document.querySelectorAll('[aria-current="page"]');
    expect(current).toHaveLength(1);
    expect(current[0].getAttribute("aria-label")).toBe("Account");
  });

  // A level draws no name of its own: it is named by the heading over its first
  // group, so a level's groups stand at the same heading level the
  // destinations' do and the outline never gains a rung with the depth. The one
  // navigation landmark names the navigation; the headings inside it name the
  // groups, and nothing between them names the panel twice.
  it("gives a level's groups the destinations' own heading level", () => {
    render(
      <WorkspaceRail
        route={{ screen: "settings", id: "account" }}
        section={fixtureSection("account")}
      />,
    );
    expect(
      screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent),
    ).toEqual(["You", "Governance"]);
    expect(screen.queryAllByRole("heading", { level: 3 })).toHaveLength(0);
  });

  // A section belongs to ONE screen. Without this the fixture's entries would
  // leak onto every route, which is exactly what the canonical-order assertions
  // above would then be lying about.
  it("ignores a section that belongs to another screen", () => {
    render(
      <WorkspaceRail
        route={{ screen: "brief" }}
        section={fixtureSection("deep")}
      />,
    );
    expect(levelLabels()).toEqual(CANONICAL_ORDER);
  });

  // Out of the section the control names what it returns to, and names it in
  // full: the level above is the whole product, which is not a list a reader
  // can be assumed to have walked down from — a deep link into settings is the
  // common way in, and "Back" alone told that reader nothing about where it led.
  it("reads and is named Back to app at the section's own level", () => {
    render(
      <WorkspaceRail
        route={{ screen: "settings", id: "account" }}
        section={fixtureSection("account")}
      />,
    );
    const back = screen.getByRole("button", { name: "Back to app" });
    expect(back.querySelector(".navlabel")?.textContent).toBe("Back to app");
  });

  // Deeper it is one step between two lists of the same section, and the word
  // for that step is the same at every depth — so the control READS "Back"
  // while its accessible name says which list it leads to. WCAG 2.5.3 holds
  // because "Back" is contained in "Back to Settings".
  it("reads Back and is named for the level it leads to below the section", () => {
    render(
      <WorkspaceRail
        route={{ screen: "settings", id: "deep", id2: "deeper" }}
        section={fixtureSection("deep")}
      />,
    );
    const back = screen.getByRole("button", { name: "Back to Settings" });
    expect(back.querySelector(".navlabel")?.textContent).toBe("Back");
  });

  // Walking back changes the ADDRESS. Climbing in the panel's own state left the
  // reader on `#/settings/<tab>` with the destinations on screen, and the only
  // way back into the section was the address they were already standing on — so
  // nothing re-rendered and the level could not be reached again.
  //
  // Through the whole SHELL, because that is the only thing that can prove it:
  // the rail on a section route is a different component (SettingsRail), mounted
  // on the way into the level and gone again on the way out, so where the reader
  // came from is remembered above it. A rail on its own always answers "brief",
  // which is the case below and would hide this one.
  it("walks out of the section to the route the reader came from", async () => {
    const user = userEvent.setup();
    window.location.hash = "#/analytics";
    render(<Shell onOpenSearch={ignoreSearch}>{null}</Shell>);
    expect(screen.getByRole("link", { name: "Analytics" })).toBeTruthy();

    navigate({ screen: "settings", id: "account" });
    await user.click(
      await screen.findByRole("button", { name: "Back to app" }),
    );
    expect(window.location.hash).toBe("#/analytics");
    // The panel is derived from the address, so the destinations arrive with it —
    // and they take the focus the level's own rows gave up rather than leaving
    // the document on <body>.
    await waitFor(() => expect(levelLabels()).toEqual(CANONICAL_ORDER));
    expect(document.activeElement).toBe(
      screen.getByRole("link", { name: "Brief" }),
    );
  });

  // The walk asks for focus at the address it is GOING to, and the route it is
  // going to arrives on a hashchange that lands a task later. Anything that
  // re-renders the section's own rail in that gap — a query settling, a popover
  // closing — must not be handed the focus meant for the destinations: it would
  // spend it on a row it is about to unmount, and the document would end on
  // <body> with the sidebar lost. The re-render below is that gap, made
  // deterministic.
  it("keeps the arriving level's focus when the section re-renders mid-walk", async () => {
    window.location.hash = "#/analytics";
    render(<Shell onOpenSearch={ignoreSearch}>{null}</Shell>);
    navigate({ screen: "settings", id: "account" });
    const back = await screen.findByRole("button", {
      name: "Back to app",
    });

    back.click();
    // The gap: the address has changed but the hashchange has not been delivered,
    // and the section's rail re-renders for a reason of its own (a pointer landing
    // on a row is the cheapest one to stage).
    fireEvent.mouseEnter(screen.getByRole("link", { name: "Account" }));
    // Still inside the section: its own first row must not have taken the focus.
    expect(document.activeElement).not.toBe(
      screen.getByRole("link", { name: "Account" }),
    );

    await waitFor(() => expect(levelLabels()).toEqual(CANONICAL_ORDER));
    expect(document.activeElement).toBe(
      screen.getByRole("link", { name: "Brief" }),
    );
  });

  // A reader who typed the address, or followed a link into it, walked down from
  // nowhere — there is no origin to return them to, and the Brief is the one place
  // the app can honestly send them.
  it("falls back to the Brief when the reader deep-linked into the section", async () => {
    const user = userEvent.setup();
    window.location.hash = "#/settings/account";
    render(<Shell onOpenSearch={ignoreSearch}>{null}</Shell>);
    await user.click(
      await screen.findByRole("button", { name: "Back to app" }),
    );
    expect(window.location.hash).toBe("#/brief");
  });

  // The head belongs to the SIDEBAR and not to the list under it: a level is the
  // same column showing a different set of rows, so every part of the head is
  // drawn exactly as it is on a route with no level. Asserted as a COMPARISON
  // and not only as "visible": the defect this guards is a rule that fires on
  // the level alone, and two absolute expectations would pass a head that had
  // quietly changed in both places. The plain rail is checked to be drawn at all,
  // so a head hidden everywhere cannot satisfy the comparison either.
  it("draws the product's own head the same on a level as off one", () => {
    shellStyles = mountShellStyles();
    const parts = [".ws-chip", ".ws-name"];
    const plain = render(<WorkspaceRail route={{ screen: "brief" }} />);
    const leveled = render(
      <WorkspaceRail
        route={{ screen: "settings", id: "account" }}
        section={fixtureSection("account")}
      />,
    );
    for (const part of parts) {
      expect(railDisplay(plain.container, part)).not.toBe("none");
      expect(railDisplay(leveled.container, part)).toBe(
        railDisplay(plain.container, part),
      );
    }
  });

  // The other half of the same head, and the half a real installation actually
  // wears: the company's own mark with "Powered by Margince" and the build badge
  // under it. That line used to go with the brand's words on a level, so the one
  // thing in the chrome saying what the product IS was absent from every
  // settings address.
  it("draws an installation's own head the same on a level as off one", () => {
    shellStyles = mountShellStyles();
    const client = newClient();
    client.setQueryData(["company"], TWO_MARKS);
    const parts = [".company-logo", ".ws-company-text", ".ws-alpha"];
    const plain = renderWith(
      client,
      <WorkspaceRail route={{ screen: "brief" }} />,
    );
    const leveled = renderWith(
      client,
      <WorkspaceRail
        route={{ screen: "settings", id: "account" }}
        section={fixtureSection("account")}
      />,
    );
    for (const part of parts) {
      expect(railDisplay(plain.container, part)).not.toBe("none");
      expect(railDisplay(leveled.container, part)).toBe(
        railDisplay(plain.container, part),
      );
    }
  });

  // Whose product this is, above whose product it runs on. Every reader of a
  // live installation is in this case — the cases above are the pre-onboarding
  // one — so the heading is the company they work for and the product is the
  // line beneath it.
  it("heads the rail with the full company logo and puts the product under it", () => {
    const client = newClient();
    client.setQueryData(["company"], {
      company_id: "11111111-1111-4111-8111-111111111111",
      display_name: "Demo GmbH",
      logo_url: "/v1/companies/11111111-1111-4111-8111-111111111111/logo",
    });
    renderWith(client, <WorkspaceRail route={{ screen: "brief" }} />);
    const brand = screen.getByRole("link", {
      name: "Demo GmbH home, powered by Margince",
    });
    const logo = within(brand).getByRole("img", { name: "Demo GmbH" });
    const image = logo.querySelector("img");
    if (!image) throw new Error("the company logo image was not rendered");
    fireEvent.load(image);
    expect(logo.classList.contains("company-logo")).toBe(true);
    // Beside the link rather than inside it: the attribution and the build badge
    // are one row of the head, and neither is somewhere a press should lead.
    const attribution = document.querySelector(".ws-company");
    expect(attribution?.textContent).toContain("Powered by");
    expect(attribution?.textContent).toContain("Margince");
    expect(within(brand).queryByText("Powered by")).toBeNull();
    expect(brand.getAttribute("href")).toBe("#/brief");
  });

  // The settings card writes a chosen mark straight into this cache entry. The
  // rail has to be OBSERVING the entry rather than peeking at it once: a peek
  // left the old face in the rail until something unrelated re-rendered the
  // shell, so a contact who had just uploaded a mark saw the monogram stay.
  it("re-draws the head when a new mark is written into the company entry", async () => {
    const client = newClient();
    const profile = {
      company_id: "44444444-4444-4444-8444-444444444444",
      display_name: "Demo GmbH",
    };
    client.setQueryData(["company"], profile);
    const { container } = renderWith(
      client,
      <WorkspaceRail route={{ screen: "brief" }} />,
    );
    expect(container.querySelector(".company-logo img")).toBeNull();

    act(() => {
      client.setQueryData(["company"], {
        ...profile,
        logo_url: "/v1/companies/44444444-4444-4444-8444-444444444444/logo",
      });
    });
    await waitFor(() =>
      expect(
        container.querySelector(".company-logo img")?.getAttribute("src"),
      ).toBe("/v1/companies/44444444-4444-4444-8444-444444444444/logo"),
    );
  });

  // The mark is the company's own, drawn from the site the onboarding read
  // resolved it from — not the product's, and not a monogram standing in for a
  // logo the installation actually has.
  it("draws the company's resolved logo as the rail's mark", () => {
    const client = newClient();
    client.setQueryData(["company"], {
      company_id: "22222222-2222-4222-8222-222222222222",
      display_name: "Demo GmbH",
      logo_url: "/v1/companies/22222222-2222-4222-8222-222222222222/logo",
    });
    const { container } = renderWith(
      client,
      <WorkspaceRail route={{ screen: "brief" }} />,
    );
    expect(
      container.querySelector(".company-logo img")?.getAttribute("src"),
    ).toBe("/v1/companies/22222222-2222-4222-8222-222222222222/logo");
  });

  // A company whose site declared no icon has a face rather than a gap: the
  // monogram is the floor under the mark, so an absent logo_url is a rendering
  // decision and never an empty slot.
  it("draws the company's monogram when no logo resolved", () => {
    const client = newClient();
    client.setQueryData(["company"], {
      company_id: "33333333-3333-4333-8333-333333333333",
      display_name: "Demo GmbH",
    });
    const { container } = renderWith(
      client,
      <WorkspaceRail route={{ screen: "brief" }} />,
    );
    expect(container.querySelector(".ws-chip img")).toBeNull();
    expect(container.querySelector(".ws-chip .avatar")?.textContent).toBe("DG");
  });

  // A company has two marks because the panel has two widths, and the whole
  // point of the square one is that it is the one drawn at 56px. The wide mark
  // is still what stands there when no icon was uploaded — a real logo squeezed
  // small reads as that company, where initials would not — so both directions
  // are asserted rather than only the interesting one.
  const TWO_MARKS = {
    company_id: "55555555-5555-4555-8555-555555555555",
    display_name: "Demo GmbH",
    logo_url: "/v1/companies/55555555-5555-4555-8555-555555555555/logo",
    logo_icon_url:
      "/v1/companies/55555555-5555-4555-8555-555555555555/logo/icon",
  };

  // Every brand branch carries it — the product's own mark, an installation's
  // logo, an installation with only a monogram. The marker is a fact about the
  // BUILD, so it does not depend on whether there is a company name above it,
  // and the branch with nothing to attribute is the one a first-run reader sees.
  it.each([
    ["the product's own mark", undefined],
    ["an installation's logo", TWO_MARKS],
    [
      "an installation's monogram",
      { ...TWO_MARKS, logo_url: undefined, logo_icon_url: undefined },
    ],
  ])("stamps the build on the head with %s", (_name, company) => {
    const client = newClient();
    if (company) {
      client.setQueryData(["company"], company);
    }
    const { container } = renderWith(
      client,
      <WorkspaceRail route={{ screen: "brief" }} />,
    );
    expect(container.querySelectorAll(".ws-alpha")).toHaveLength(1);
    expect(container.querySelector(".ws-alpha")?.textContent).toBe(
      displayVersion(),
    );
  });

  it("draws the square icon when the panel is collapsed", () => {
    const client = newClient();
    client.setQueryData(["company"], TWO_MARKS);
    const { container } = renderWith(
      client,
      <WorkspaceRail route={{ screen: "brief" }} collapsed />,
    );
    expect(
      container.querySelector(".company-logo img")?.getAttribute("src"),
    ).toBe(TWO_MARKS.logo_icon_url);
  });

  it("draws the wide mark when the panel is expanded", () => {
    const client = newClient();
    client.setQueryData(["company"], TWO_MARKS);
    const { container } = renderWith(
      client,
      <WorkspaceRail route={{ screen: "brief" }} />,
    );
    expect(
      container.querySelector(".company-logo img")?.getAttribute("src"),
    ).toBe(TWO_MARKS.logo_url);
  });

  it("keeps the wide mark in the collapsed panel when no icon was uploaded", () => {
    const client = newClient();
    client.setQueryData(["company"], {
      ...TWO_MARKS,
      logo_icon_url: undefined,
    });
    const { container } = renderWith(
      client,
      <WorkspaceRail route={{ screen: "brief" }} collapsed />,
    );
    expect(
      container.querySelector(".company-logo img")?.getAttribute("src"),
    ).toBe(TWO_MARKS.logo_url);
  });

  // An entry that HAS children opens them: standing on it, the panel shows the
  // level it leads to rather than the list it came from. Nothing carries the
  // current page there until a child is addressed — the same as any list a
  // reader has just been handed.
  it("opens an entry's children as soon as the route stands on that entry", () => {
    render(
      <WorkspaceRail
        route={{ screen: "settings", id: "deep" }}
        section={fixtureSection("deep")}
      />,
    );
    expect(levelLabels()).toEqual(["Data model"]);
    expect(document.querySelectorAll('[aria-current="page"]')).toHaveLength(0);
    expect(
      screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent),
    ).toEqual(["Privacy & retention"]);
  });

  it("renders a third level from the data, addressed under the entry that opens it", () => {
    render(
      <WorkspaceRail
        route={{ screen: "settings", id: "deep", id2: "deeper" }}
        section={fixtureSection("deep")}
      />,
    );
    // The child level: only the entry's children, addressed under it.
    expect(levelLabels()).toEqual(["Data model"]);
    expect(
      screen.getByRole("link", { name: "Data model" }).getAttribute("href"),
    ).toBe("#/settings/deep/deeper");
    expect(
      screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent),
    ).toEqual(["Privacy & retention"]);
  });

  // One step at a time, and the step is an ADDRESS: below the section's own
  // level the way back leads to the entry the reader drilled through, whose own
  // address is what names the level above. It is also what the control is named
  // for — the section's list, not the entry whose children are on screen.
  it("lands on the parent entry's own address from a level below the section", async () => {
    const user = userEvent.setup();
    render(
      <WorkspaceRail
        route={{ screen: "settings", id: "deep", id2: "deeper" }}
        section={fixtureSection("deep")}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Back to Settings" }));
    expect(window.location.hash).toBe("#/settings/deep");
  });

  // AC-shell-1d holds at every depth, and there is ONE tooltip in the sidebar:
  // moving between two entries of a level must not leave the first one open.
  it("shows one tooltip at a time on the collapsed level", async () => {
    const user = userEvent.setup();
    render(
      <WorkspaceRail
        route={{ screen: "settings", id: "account" }}
        section={fixtureSection("account")}
        collapsed
      />,
    );
    const account = screen.getByRole("link", { name: "Account" });
    const privacyEntry = screen.getByRole("link", {
      name: "Privacy & retention",
    });
    expect(screen.queryByRole("tooltip")).toBeNull();

    account.focus();
    // waitFor on the TEXT, not findAllByRole on the role: a tooltip left over
    // from the previous row satisfies the role query on its first poll, so a
    // "one tooltip" assertion could pass while showing the wrong one.
    await waitFor(() =>
      expect(screen.getByRole("tooltip").textContent).toBe("Account"),
    );

    privacyEntry.focus();
    await waitFor(() =>
      expect(screen.getByRole("tooltip").textContent).toBe(
        "Privacy & retention",
      ),
    );
    expect(screen.getAllByRole("tooltip")).toHaveLength(1);

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("tooltip")).toBeNull();
    // Escape dismisses the tooltip without moving focus (WCAG 1.4.13).
    expect(document.activeElement).toBe(privacyEntry);
  });

  // At phone width the panel is a bar of four destinations, and it keeps them on
  // a section route: handing the bar to a section lost every destination, made
  // switching entries More → scroll → tap, and left a bar holding two controls.
  // The section is reached from the page title's own switcher there instead.
  it("keeps the destinations on the phone bar, even on a section route", async () => {
    const user = userEvent.setup();
    stubPhoneViewport();
    render(
      <WorkspaceRail
        route={{ screen: "settings", id: "account" }}
        section={fixtureSection("account")}
      />,
    );
    expect(levelLabels()).toEqual(CANONICAL_ORDER);
    // No level at all: no entries, no way back up, and no `leveled` arrangement
    // for the bar to be rearranged by.
    expect(
      screen.queryByRole("link", { name: "Privacy & retention" }),
    ).toBeNull();
    expect(screen.queryByRole("button", { name: /^Back/ })).toBeNull();
    expect(
      screen.getByRole("navigation", { name: "Primary navigation" }).className,
    ).not.toContain("leveled");

    // And the sheet is what it was before levels existed: every destination, and
    // still no entry of the section the reader is standing in.
    await user.click(screen.getByRole("button", { name: "More" }));
    expect(levelLabels()).toEqual(CANONICAL_ORDER);
    expect(
      screen.queryByRole("link", { name: "Privacy & retention" }),
    ).toBeNull();
  });

  // The other half: above the breakpoint the level is exactly what it was. Either
  // assertion alone is satisfied by a rail that ignores every section, or by one
  // that ignores the width.
  it("still walks into the section above the phone breakpoint", () => {
    render(
      <WorkspaceRail
        route={{ screen: "settings", id: "account" }}
        section={fixtureSection("account")}
      />,
    );
    expect(levelLabels()).toEqual(["Account", "Privacy & retention"]);
    expect(
      screen.getByRole("navigation", { name: "Primary navigation" }).className,
    ).toContain("leveled");
  });
});
