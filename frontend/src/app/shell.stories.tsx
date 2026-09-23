// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useQueryClient } from "@tanstack/react-query";
import {
  Building2,
  House,
  KeyRound,
  Mail,
  Mic,
  Plug,
  ShieldCheck,
  Sparkles,
  UserRound,
  UsersRound,
  Wrench,
} from "lucide-react";
import { type ReactNode, useRef, useState } from "react";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { Card } from "../design-system/atoms";
import { en } from "../i18n/en";
import { SETTINGS_PAGES } from "../screens/settingscatalog";
import { SettingsSearchBox } from "../screens/settingssearchbox";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "../screens/story-utils";
import type { GrantSpec } from "./mefixture";
import type { NavSection } from "./nav";
import { CommandPalette, useBuiltinCommands } from "./palette";
import type { Route } from "./router";
import { PageTitle, Shell, WorkspaceRail } from "./shell";
import { TopBar } from "./topbar";
// The settings stack's own sheet. A screen sheet loads with its screen, and the
// settings frame below stands the stack under the shell WITHOUT that screen —
// so without this the frame would draw an uncapped column and quietly
// misrepresent the one thing it exists to show.
import "../screens/settings.css";

// fullscreen: the shell sizes itself to the viewport, so Storybook's default
// canvas padding would clip the sidebar foot and misrepresent the layout — and
// the foot is where the entitlement row reports the installation's seats. The
// account block is NOT there any more; it stands at the end of the top bar's
// trail (Shell/Account block), which is why every frame here renders that bar.
const meta: Meta<typeof Shell> = {
  title: "Shell/Navigation shell",
  component: Shell,
  parameters: {
    layout: "fullscreen",
    // The `phone` viewport this file's last story selects is declared once for
    // the whole catalog in .storybook/preview.tsx — it stopped being this
    // file's private need the moment the settings pages wanted it too.
  },
};
export default meta;
type Story = StoryObj<typeof Shell>;

// What is waiting, spelled once and handed to BOTH halves of the chrome the way
// the shell hands it: the Decisions row's badge in the panel and the mark's chip
// in the strip are two readings of one queue, and a frame that fed one and left
// the other at nothing would draw the same queue as 12 and as empty at once.
const COUNTS = { inbox: 12, tasks: 4 };
const COMPANY_WORDMARK =
  "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 700 200'%3E%3Crect width='700' height='200' fill='white'/%3E%3Ctext x='350' y='125' text-anchor='middle' font-family='sans-serif' font-size='92' font-weight='700' fill='%23ff6500'%3EGRADION%3C/text%3E%3C/svg%3E";
// The same company's square badge. It exists because the wordmark above is
// unreadable at 32px, so the two are only worth looking at together.
const COMPANY_BADGE =
  "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 200 200'%3E%3Crect width='200' height='200' rx='36' fill='%23ff6500'/%3E%3Ctext x='100' y='142' text-anchor='middle' font-family='sans-serif' font-size='128' font-weight='700' fill='white'%3EG%3C/text%3E%3C/svg%3E";

/**
 * The session every frame here mounts on, plus whatever routes the frame is about.
 *
 * `allow` is the grants, not a role: three things in this chrome are capability
 * questions — the entitlement row at the foot asks `license:read`, and the two app
 * banners ask `automation:update` and `embedding_reindex:read` — so a hand-written
 * body carrying no `authorization` at all answers every one of them closed. That
 * looks exactly like a deliberate denial on screen, which is how a frame comes to
 * be named for something it never draws. `meRoute({})` is the honest default here:
 * an admin on a full seat holding no object grants.
 */
function stubSession(allow: GrantSpec = {}, about: RouteMap = {}) {
  installFetchStub({
    "GET /me": meRoute(allow),
    "GET /ai/usage": () =>
      jsonResponse({
        days: [],
        budget: { monthly_tokens: 100, spent_tokens: 20, band: "normal" },
      }),
    ...about,
  });
}

// The brand block reads the installation profile from the cache the onboarding
// gate fills in the real app; a story seeds the same entry so the company heads
// the rail the way it does for every reader of a live installation. Without it
// the block honestly falls back to the product's own mark and name.
//
// No logo_url: Storybook serves no object store, and what a broken image would
// draw here is the monogram anyway. The monogram IS the case worth framing —
// the mark a company gets when its site declared no icon.
function SeedInstallation({
  children,
  logoUrl,
  logoIconUrl,
}: Readonly<{
  children: ReactNode;
  logoUrl?: string;
  logoIconUrl?: string;
}>) {
  const client = useQueryClient();
  if (client.getQueryData(["company"]) === undefined) {
    client.setQueryData(["company"], {
      company_id: "company-1",
      display_name: "Gradion GmbH",
      logo_url: logoUrl,
      logo_icon_url: logoIconUrl,
    });
  }
  return <>{children}</>;
}

/**
 * The search row wired to the thing it actually opens.
 *
 * `onOpenSearch` is not a decorative prop: the top bar's field is the only
 * pointer route into the command palette, so a story that stubbed it would show
 * a control that does nothing and prove nothing. The app mounts the palette
 * beside the shell (App.tsx) — so does this, off the same builtin command list.
 */
function usePaletteSeam() {
  const [open, setOpen] = useState(false);
  const commands = useBuiltinCommands();
  const palette = (
    <CommandPalette
      open={open}
      onClose={() => setOpen(false)}
      commands={commands}
    />
  );
  return { openSearch: () => setOpen(true), palette };
}

function ShellExample({
  at = "#/",
  children,
}: Readonly<{
  // The address this frame stands on. Stated on every frame rather than left to
  // whatever the last one visited: the catalog keeps one iframe across a walk
  // between stories, so a frame that named a route used to rename the page in
  // every frame opened after it. `#/` is the product's own first destination.
  at?: string;
  children: ReactNode;
}>) {
  // Where the frame OPENS, once, and not where it is held: the rail's rows are
  // real links, so a frame that restated its address on every render would drag
  // a reader back the instant they used the navigation it is a picture of.
  const opened = useRef(false);
  if (!opened.current) {
    opened.current = true;
    globalThis.location.hash = at;
  }
  const { openSearch, palette } = usePaletteSeam();
  return (
    <>
      <Shell onOpenSearch={openSearch} counts={COUNTS}>
        {children}
      </Shell>
      {palette}
    </>
  );
}

export const Default: Story = {
  render: () => {
    stubSession();
    return (
      <StoryProviders>
        <SeedInstallation>
          <ShellExample>
            <div className="wrap">
              <Card as="div">Content</Card>
            </div>
          </ShellExample>
        </SeedInstallation>
      </StoryProviders>
    );
  },
};

/**
 * A named page on the capped reading column: the heading starts and ends
 * exactly where the card under it does.
 *
 * The head used to run the full width of the window while the column was
 * capped, so on a wide display the page's name began a hand's width to the left
 * of its own content. One measure now caps both (shell.css, --pageMeasure), and
 * this frame is where a change to either is visible: put a ruler on the left
 * edge of the title and the left edge of the card.
 */
export const PageHeadOnItsColumn: Story = {
  name: "a named page — head on the column",
  render: () => {
    stubSession();
    return (
      <StoryProviders>
        <SeedInstallation>
          <ShellExample at="#/filters">
            <div className="wrap">
              <Card as="div">Content</Card>
            </div>
          </ShellExample>
        </SeedInstallation>
      </StoryProviders>
    );
  },
};

/**
 * The same rule where the page's column is NARROWER than the page: a settings
 * entry, whose body is the settings stack.
 *
 * The head follows the stack rather than the page, because what a heading has
 * to line up with is the content it names. Both read one token
 * (--settingsColumn), so the two cannot be given different numbers — which is
 * what `pagecolumn.test.ts` holds and what this frame shows.
 */
export const SettingsHeadOnItsColumn: Story = {
  name: "a settings entry — head on the stack's measure",
  render: () => {
    stubSession();
    return (
      <StoryProviders>
        <SeedInstallation>
          <ShellExample at="#/settings/account">
            <div className="wrap">
              <div className="settings-stack">
                <Card as="div">Content</Card>
              </div>
            </div>
          </ShellExample>
        </SeedInstallation>
      </StoryProviders>
    );
  },
};

/** A company's wide logo owns the expanded head, with Margince credited on the
 * line beneath it. This is deliberately not an Avatar story: squeezing this
 * asset into the navigation rows' square is the failure this state catches. */
export const CompanyWordmark: Story = {
  render: () => {
    stubSession();
    return (
      <StoryProviders>
        <SeedInstallation logoUrl={COMPANY_WORDMARK}>
          <ShellExample>
            <div className="wrap">
              <Card as="div">Content</Card>
            </div>
          </ShellExample>
        </SeedInstallation>
      </StoryProviders>
    );
  },
};

/**
 * One sidebar state, in the frame it really sits in.
 *
 * The `.app` grid is what gives the sidebar its width (56px collapsed, 252px
 * labeled) and the content column its edge, so the example renders the grid
 * rather than a hand-set width — otherwise the story would be showing geometry
 * the product does not use. The content column is present and empty on purpose:
 * the sidebar is flush to the frame's left edge, separated by its own
 * border-right and nothing else, and that reads only against something beside it.
 *
 * The collapse control is live, so each panel can be moved to the other state —
 * the two are one component, and a story where the control does nothing hides
 * the transition it exists for.
 */
function SidebarExample({
  initiallyCollapsed,
}: Readonly<{ initiallyCollapsed: boolean }>) {
  const [collapsed, setCollapsed] = useState(initiallyCollapsed);
  const { openSearch, palette } = usePaletteSeam();
  return (
    <div className={collapsed ? "app" : "app railexpanded"}>
      <WorkspaceRail
        route={{ screen: "deals" }}
        counts={COUNTS}
        collapsed={collapsed}
      />
      <main className="main">
        <TopBar
          route={{ screen: "deals" }}
          collapsed={collapsed}
          onToggle={() => setCollapsed((current) => !current)}
          onOpenSearch={openSearch}
        />
        <div className="scroll" />
      </main>
      {palette}
    </div>
  );
}

// Both sidebar states, side by side. Expanded is 252px of 34px rows on a 4px
// gutter; collapsed is the canonical 56px geometry, where a row IS its target and
// keeps 44px, the head is the mark with the build badge under it, and the group
// headings are not drawn at all — 56px carries a glyph and not a label, and the
// break between two groups is what says where one ends.
//
// The head is TWO rows either way: the brand block, then what the installation
// runs on and which build of it this is. Expanded that line reads "Powered by
// Margince" with the version badge beside it; at 56px the attribution goes and
// the badge stays, shortened to one glyph.
//
// What is NOT in either panel: the search and the account block. Both moved to
// the strip above (app/topbar.tsx), which is why these frames render that strip
// rather than the sidebar alone — and the current row's own signal is now a tint
// plus a weight plus the accent on its glyph, with the indicator bar that used to
// sit in the gutter gone.
export const SidebarStates: Story = {
  name: "sidebar — expanded and collapsed",
  render: () => {
    stubSession();
    return (
      <StoryProviders>
        <SeedInstallation>
          <div style={{ display: "flex", height: "100vh" }}>
            <div style={{ flex: 1, minWidth: 0 }}>
              <SidebarExample initiallyCollapsed={false} />
            </div>
            <div style={{ flex: 1, minWidth: 0 }}>
              <SidebarExample initiallyCollapsed />
            </div>
          </div>
        </SeedInstallation>
      </StoryProviders>
    );
  },
};

/**
 * The two marks in the two panels that ask for them, side by side.
 *
 * This is the frame the second slot exists for: the same company, wordmark
 * across the open head and badge alone in the 56px rail. Reading them together
 * is the only way to see that the swap happens at all, and the only way to
 * catch the failure it replaced — a wordmark contained into a 32px square,
 * which renders as a smear rather than as a missing image.
 */
export const CompanyMarksAtBothWidths: Story = {
  name: "sidebar — wordmark expanded, badge collapsed",
  render: () => {
    stubSession();
    return (
      <StoryProviders>
        <SeedInstallation
          logoUrl={COMPANY_WORDMARK}
          logoIconUrl={COMPANY_BADGE}
        >
          <div style={{ display: "flex", height: "100vh" }}>
            <div style={{ flex: 1, minWidth: 0 }}>
              <SidebarExample initiallyCollapsed={false} />
            </div>
            <div style={{ flex: 1, minWidth: 0 }}>
              <SidebarExample initiallyCollapsed />
            </div>
          </div>
        </SeedInstallation>
      </StoryProviders>
    );
  },
};

/**
 * The foot with an entitlement read behind it.
 *
 * The foot is the agent and draws no licence posture: a refused licence is the
 * shell banner's, and absent or refused is the agent panel's pill. These frames
 * stub a live entitlement so a posture that reached the rail would show here.
 *
 * `checked_at` is a fixed instant rather than `new Date()`: a fixture that reads
 * the clock makes a capture that differs from the last one for no reason.
 */
type LicenseEntitlement = components["schemas"]["LicenseEntitlement"];

const CHECKED_AT = "2026-08-19T09:00:00Z";

const WITHIN_CAP: LicenseEntitlement = {
  state: "valid",
  seats_used: 12,
  seats_granted: 25,
  over_limit: false,
  checked_at: CHECKED_AT,
};

/**
 * The session a frame about the foot needs: `license:read`, and the entitlement
 * itself. Both halves, because without either the read is never answered.
 */
function stubEntitlement(entitlement: LicenseEntitlement) {
  stubSession(
    { license: ["read"] },
    { "GET /installation/license": () => jsonResponse(entitlement) },
  );
}

/**
 * The rail's foot under both panel states side by side.
 *
 * A licence is stubbed because the shell's providers want one, not because the
 * foot reports it — it has not since the posture moved to its own banner. Two
 * further frames stubbed an over-cap licence and a seat without `license:read`
 * and photographed this same picture, which is what retired them; what each of
 * them knew is now said where the behaviour is, in app/license-posture.ts.
 *
 * `.rail.collapsed` sets `overflow: visible` so its tooltips can escape the 56px
 * box, so the collapsed panel neither scrolls nor clips: in a window shorter
 * than its 44px rows add up to, its foot sits below the viewport. The expanded
 * panel scrolls; read the collapsed foot in a window tall enough to hold it.
 */
export const RailFoot: Story = {
  name: "the foot — collapsed and expanded",
  render: () => {
    stubEntitlement(WITHIN_CAP);
    return (
      <StoryProviders>
        <SeedInstallation>
          <div style={{ display: "flex", height: "100vh" }}>
            <div style={{ flex: 1, minWidth: 0 }}>
              <SidebarExample initiallyCollapsed={false} />
            </div>
            <div style={{ flex: 1, minWidth: 0 }}>
              <SidebarExample initiallyCollapsed />
            </div>
          </div>
        </SeedInstallation>
      </StoryProviders>
    );
  },
};

/**
 * The sidebar's SECOND level.
 *
 * The section below is a fixture, and deliberately so: at runtime the settings
 * screen publishes this shape from live grants (`useSettingsSection`), which
 * would make these stories a picture of a permission matrix rather than of the
 * level. The point here is what the SHELL does with a section — one level at a
 * time, the sidebar's own head above it, and the level named by the heading over
 * its first group — so the data is held still and the rendering is what varies.
 *
 * `privacy` carries children no settings entry really has, which is the one part
 * of the fixture that is not a copy of production: it is how the third level
 * (and the back control that names the level it leads to) can be seen at all.
 */
const SETTINGS_SECTION: NavSection = {
  screen: "settings",
  titleKey: "nav.settings",
  activeId: "privacy",
  // The search box the settings screen puts above its rows, so the story shows
  // the field at the width it is actually drawn at — flush with the rows.
  lead: <SettingsSearchBox pages={SETTINGS_PAGES} />,
  groups: [
    // The group that carries the level's NAME, and the Overview row with it.
    // Production builds this from the catalog (screens/settingsnav.tsx); here it
    // is spelled out, because a level with no first heading would be a picture
    // of an unnamed panel.
    {
      headingKey: "nav.settings",
      items: [
        { id: "home", labelKey: "settings.home", icon: House, level: true },
      ],
    },
    {
      headingKey: "settings.group.me",
      items: [
        { id: "account", labelKey: "settings.tab.account", icon: UserRound },
        { id: "voice", labelKey: "settings.tab.voice", icon: Mic },
        { id: "agents", labelKey: "settings.tab.agents", icon: KeyRound },
        { id: "connections", labelKey: "settings.tab.connections", icon: Plug },
      ],
    },
    {
      headingKey: "settings.group.company",
      items: [
        { id: "company", labelKey: "settings.tab.company", icon: Building2 },
      ],
    },
    {
      headingKey: "settings.group.people",
      items: [
        { id: "members", labelKey: "settings.tab.members", icon: UsersRound },
      ],
    },
    {
      headingKey: "settings.group.data",
      items: [{ id: "capture", labelKey: "settings.tab.capture", icon: Mail }],
    },
    {
      headingKey: "settings.group.ai",
      items: [
        { id: "models", labelKey: "settings.tab.models", icon: Sparkles },
      ],
    },
    {
      headingKey: "settings.group.governance",
      items: [
        {
          id: "privacy",
          labelKey: "settings.tab.privacy",
          icon: ShieldCheck,
          // The children are the synthetic part: no settings entry publishes
          // any, so without them the third-level story would be a second
          // picture of the second level.
          children: [
            {
              id: "data-model",
              labelKey: "settings.tab.fields",
              icon: Wrench,
            },
          ],
        },
        {
          id: "system-health",
          labelKey: "settings.tab.system-health",
          icon: Wrench,
        },
      ],
    },
  ],
};
// The level shown is derived from the ADDRESS, and the way back up navigates —
// so in a story holding a route still it moves the URL and leaves the panel
// where the story put it. Each depth below is therefore a story of its own,
// which is also how a reviewer sees them side by side.
//
// A level takes the brand's words for its rows and nothing else. The search and
// the collapse control belong to the top bar above, so the story renders that bar
// too — a panel shown without it would be a picture of half the chrome.
function LevelExample({
  initiallyCollapsed,
  tab = "general",
  sub,
}: Readonly<{ initiallyCollapsed: boolean; tab?: string; sub?: string }>) {
  const [collapsed, setCollapsed] = useState(initiallyCollapsed);
  const { openSearch, palette } = usePaletteSeam();
  return (
    <div className={collapsed ? "app" : "app railexpanded"}>
      <WorkspaceRail
        route={{ screen: "settings", id: tab, id2: sub }}
        section={{ ...SETTINGS_SECTION, activeId: tab }}
        counts={COUNTS}
        collapsed={collapsed}
      />
      <main className="main">
        <TopBar
          route={{ screen: "settings", id: tab, id2: sub }}
          section={{ ...SETTINGS_SECTION, activeId: tab }}
          collapsed={collapsed}
          onToggle={() => setCollapsed((current) => !current)}
          onOpenSearch={openSearch}
        />
        <div className="scroll" />
      </main>
      {palette}
    </div>
  );
}

function LevelStory({ children }: Readonly<{ children: ReactNode }>) {
  stubSession();
  return (
    <StoryProviders>
      <SeedInstallation>{children}</SeedInstallation>
    </StoryProviders>
  );
}

// The section's level: the sidebar's own head, the way out of the section, the
// search box, then the groups — the first of them headed with the section's name.
// The ten destinations are GONE rather than pushed below a second list, since one
// column carrying both levels reads as a list of twenty places to go, and the
// head is untouched because this is the same sidebar showing a different list.
export const SectionLevel: Story = {
  name: "second level — expanded",
  render: () => (
    <LevelStory>
      <LevelExample initiallyCollapsed={false} />
    </LevelStory>
  ),
};

// The same level at 56px: icons, the collapsed rail's tooltip on hover or
// keyboard focus, no group headings — the level's own name among them, since it
// is one — and no search box, which has no icon-sized form. What is left saying
// where one group ends is the space between them.
export const SectionLevelCollapsed: Story = {
  name: "second level — collapsed",
  render: () => (
    <LevelStory>
      <LevelExample initiallyCollapsed />
    </LevelStory>
  ),
};

// The third level, reached by standing on an entry that has children: the level
// is named by the entry the reader drilled through — as the heading over its one
// group — and the back control reads "Back", named for the list it leads to.
export const ThirdLevel: Story = {
  name: "third level — expanded",
  render: () => (
    <LevelStory>
      {/* The child segment too, or the third level renders with no row current
          — a picture of a level nobody is standing in. */}
      <LevelExample initiallyCollapsed={false} tab="privacy" sub="data-model" />
    </LevelStory>
  ),
};

/**
 * A section at phone width, where the sidebar does NOT hand its bar over.
 *
 * The bar keeps the four destinations plus More on a section route — handing them
 * to a section's entries lost the whole product's navigation and left two
 * controls floating in a card — so the section lives in the PAGE HEAD here: the
 * heading names it, and the switcher under the heading names the entry the reader
 * is on and opens the rest as a sheet.
 *
 * Rendered from the parts rather than through `Shell`, because the real settings
 * section is published from live grants: the rail and the head are both handed the
 * fixture, which is what keeps this a picture of the CHROME.
 *
 * The bar itself carries five glyphs and no captions — four destinations and
 * More. Every row keeps its name in `aria-label`, so nothing is lost to a screen
 * reader or a voice-control user; the words are simply not drawn at a size where
 * they would be legible, and the sheet behind More is where they come back.
 *
 * One thing about the viewport tool is worth knowing before trusting a picture
 * of this: it is applied by the MANAGER, which resizes the preview iframe, and
 * these are viewport media queries — so a story opened as a bare `iframe.html`,
 * which is how the fe-uat capture gate renders, would get the harness's own width
 * and draw the SIDEBAR. `uat-phone` is what stops that: the gate reads the tag and
 * drives the browser itself to 390px. Without it every story named for a phone was
 * captured at 1024px, which is how three of them once shipped showing a sidebar.
 */
function PhoneSectionExample() {
  const route: Route = { screen: "settings", id: "privacy" };
  const { openSearch, palette } = usePaletteSeam();
  return (
    <div className="app railexpanded">
      <WorkspaceRail route={route} section={SETTINGS_SECTION} counts={COUNTS} />
      <main className="main">
        <TopBar
          route={route}
          section={SETTINGS_SECTION}
          collapsed={false}
          onOpenSearch={openSearch}
        />
        <div className="scroll">
          <PageTitle route={route} section={SETTINGS_SECTION} />
          <div className="wrap">
            <Card as="div">Content</Card>
          </div>
        </div>
      </main>
      {palette}
    </div>
  );
}

export const SectionPhone: Story = {
  name: "a section — phone bar and the head's switcher",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => (
    <LevelStory>
      <PhoneSectionExample />
    </LevelStory>
  ),
};

/**
 * The More sheet, open — the phone's whole sidebar.
 *
 * The bar it grew out of is five cells with no captions, which is all a 390px
 * row can hold; the sheet is where the ten destinations become a LIST again, and
 * everything the bar had to give up comes back with them:
 *
 * - a head, with the installation's mark and name — the bar has no room for one,
 *   so this is the only place at this width that says which workspace this is;
 * - the labels themselves, at body size, with each row's glyph on the left and
 *   the whole width as its target;
 * - the group headings, spelled out where the collapsed sidebar draws none,
 *   because the sheet is 600px wide whatever the desktop preference it
 *   inherited was left at;
 * - the badge as a TRAILING figure in its row rather than a chip pinned to a
 *   tab — a list row has somewhere to put a number.
 *
 * What does NOT come back is the agent. It is a cell of the BAR, and the bar is
 * not on screen while this is; a row for it in a list of ten places would file
 * the one thing in the chrome that reports rather than navigates as an eleventh
 * place to go. What the cell it left behind buys the sheet is the room for the
 * whole list: the ceiling is the viewport less the ledge it stands on and the
 * same air again at the top, and the ten destinations now sit inside it without
 * scrolling. It is still a scroller — a longer locale or a level with more rows
 * will reach that ceiling.
 *
 * The close control is the same More button in its other state, renamed, and it
 * is PINNED to the head's corner rather than left at the end of a scrolling list
 * — a reader who opened the sheet by mistake should not have to walk ten rows to
 * leave it. There is deliberately no grab handle: a handle is a control on every
 * platform that documents one, and this sheet has one height and no drag, so
 * drawing it would promise a gesture that does nothing.
 *
 * The route is Reports, which the bar does not carry. That is the case the sheet
 * exists to answer: the current page's own row is `display: none` in the bar, so
 * the closed bar reports it through More instead, and opening the sheet is what
 * makes it visible. It is also why the scrim and the `inert` content column are
 * part of the frame rather than decoration — the page under a sheet this size has
 * to stop being reachable by Tab, not merely be dimmed.
 *
 * `uat-phone` is what makes the capture gate drive the browser to 390px, and the
 * `play` below opens the sheet the way a reader does; there is no prop that forces
 * it open, and adding one would put a control in the product that exists only for
 * the catalog. The focus ring on the first row in the captured frame is that
 * opening, not a stray state: a sheet that covers the page has to take focus with
 * it, or a keyboard reader is left tabbing through a page they can no longer see.
 */
function PhoneSheetExample() {
  const route: Route = { screen: "analytics" };
  const { openSearch, palette } = usePaletteSeam();
  return (
    <div className="app railexpanded">
      <WorkspaceRail route={route} counts={COUNTS} />
      <main className="main">
        <TopBar route={route} collapsed={false} onOpenSearch={openSearch} />
        <div className="scroll">
          <PageTitle route={route} />
          <div className="wrap">
            <Card as="div">The page the sheet covers.</Card>
          </div>
        </div>
      </main>
      {palette}
    </div>
  );
}

export const PhoneMoreSheet: Story = {
  name: "phone — the More sheet as a list",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "More" }));
  },
  render: () => {
    stubEntitlement(WITHIN_CAP);
    return (
      <StoryProviders>
        <SeedInstallation>
          <PhoneSheetExample />
        </SeedInstallation>
      </StoryProviders>
    );
  },
};

/**
 * The phone bar, closed — three destinations, the agent, and More.
 *
 * Five cells of equal width, and only three of them are places to go. The middle
 * one is the agent, and everything about how it is drawn is one claim: it is a
 * different KIND of thing from the four glyphs around it. It rises out of the
 * bar's top edge in a round well of the bar's own material, it carries the only
 * lit object in the chrome, and it has no caption — the bar has no captions at
 * all, and a word under the one cell with room for one would read as a label for
 * the bar rather than for the agent. Its accessible name says the rest.
 *
 * The Worklist is what the bar gave up for that cell. It is a tap away in the
 * sheet, which is the same distance the other six destinations already are.
 *
 * Position is load-bearing twice over. It is the CENTRE because the agent
 * belongs to the whole session rather than to any destination, so it stands
 * outside the row of them rather than at one end. And it is the third cell in
 * the DOM as well as on screen (app/navlevel.tsx renders it into the row stream),
 * so a thumb and a Tab key read the bar in the same order — a cell placed by a
 * grid column alone would have been third to the eye and last to the keyboard.
 */
function PhoneBarExample() {
  const route: Route = { screen: "home" };
  const { openSearch, palette } = usePaletteSeam();
  return (
    <div className="app railexpanded">
      <WorkspaceRail route={route} counts={COUNTS} />
      <main className="main">
        <TopBar route={route} collapsed={false} onOpenSearch={openSearch} />
        <div className="scroll">
          <PageTitle route={route} />
          <div className="wrap">
            <Card as="div">The page the bar floats over.</Card>
          </div>
        </div>
      </main>
      {palette}
    </div>
  );
}

export const PhoneBar: Story = {
  name: "phone — the bar, with the agent in the middle",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => {
    stubEntitlement(WITHIN_CAP);
    return (
      <StoryProviders>
        <SeedInstallation>
          <PhoneBarExample />
        </SeedInstallation>
      </StoryProviders>
    );
  },
};

/**
 * The agent's panel, open over the phone bar.
 *
 * The same panel the sidebar opens beside its own card, in the only place a
 * bottom bar leaves for it: above. Two things make it read as this cell's panel
 * rather than as a sheet that arrived from nowhere.
 *
 * It is MEASURED from the well, not from the cell behind it — the well rises
 * clear of the bar, and a frame taken from the cell would open the panel across
 * the orb it belongs to. And it carries a notch that points back down at that
 * well, drawn on the portalled wrapper rather than on the panel, because the
 * panel scrolls its own body and would clip anything outside it. The notch's x
 * is the measured middle of the well, so it still lands if the bar's cells ever
 * stop being even.
 *
 * There is no scrim. This is a popover, not the More sheet: the page behind it
 * stays live and readable, an outside tap or Escape closes it, and focus goes
 * back to the orb. A dimmed page would claim the reader has to answer something
 * before carrying on, and there is nothing here to answer.
 */
export const PhoneAgentPanel: Story = {
  name: "phone — the agent's panel over the bar",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    // Named from the catalogue, not written out: the orb's name is a message
    // key now, and a literal here went stale the day the copy moved.
    await userEvent.click(
      canvas.getByRole("button", { name: en["agent.rail.open"] }),
    );
  },
  render: () => {
    stubEntitlement(WITHIN_CAP);
    return (
      <StoryProviders>
        <SeedInstallation>
          <PhoneBarExample />
        </SeedInstallation>
      </StoryProviders>
    );
  },
};
