// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
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
import { type ReactNode, useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../api/schema";
import { PageHeadAction } from "../components/pageheadaction";
import { Button, Card } from "../design-system/atoms";
import { SettingsSearchBox } from "../features/settings/settingssearchbox";
import type { Locale } from "../i18n";
import { en } from "../i18n/en";
import type { Route } from "../lib/router";
import { SETTINGS_PAGES } from "../lib/settingscatalog";
import type { GrantSpec } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "../testing/story-utils";
import type { NavSection } from "./nav";
import { PageTitle, Shell } from "./shell";
import {
  COUNTS,
  SeedInstallation,
  SHELL_ROUTES,
  ShellExample,
  usePaletteSeam,
} from "./shell.storykit";
import { WorkspaceRail } from "./shellrail";
import { TopBar } from "./topbar";
// The settings stack's own sheet. A screen sheet loads with its screen, and the
// settings frame below stands the stack under the shell WITHOUT that screen —
// so without this the frame would draw an uncapped column and quietly
// misrepresent the one thing it exists to show.
import "../features/settings/settings.css";

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
    ...SHELL_ROUTES,
    "GET /me": meRoute(allow),
    ...about,
  });
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

// A one-list settings page puts its create verb beside the page's name. The
// head keeps the name 16rem wide; below that the verb wraps under it.
function SettingsHeadWithVerb() {
  stubSession({ user_admin: ["read", "create"] });
  return (
    <StoryProviders>
      <SeedInstallation>
        <ShellExample at="#/settings/members">
          <PageHeadAction>
            <Button>{en["users.inviteOpen"]}</Button>
          </PageHeadAction>
          <div className="wrap">
            <div className="settings-stack">
              <Card as="div">Content</Card>
            </div>
          </div>
        </ShellExample>
      </SeedInstallation>
    </StoryProviders>
  );
}

export const SettingsHeadWithAction: Story = {
  name: "a settings entry — the page's verb in the head",
  render: () => <SettingsHeadWithVerb />,
};

export const SettingsHeadWithActionPhone: Story = {
  name: "a settings entry — the page's verb under the head on a phone",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => <SettingsHeadWithVerb />,
};

/** A company with only a wide wordmark wears its monogram in the head. A
 * wordmark contained into the head's square renders as a smear, not as a
 * missing image, and that smear is the failure this state catches. */
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
// Neither panel holds the search or the account block. Both live in
// the strip above (app/topbar.tsx), which is why these frames render that strip
// rather than the sidebar alone.
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
 * The company's square icon in both panels, side by side: the open head's
 * card beside the 56px rail. The wide wordmark is seeded too, and appears in
 * neither, because the head draws one square mark at every width.
 */
export const CompanyMarksAtBothWidths: Story = {
  name: "sidebar — the company's square icon at both widths",
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
 * A company name near the 200 characters the installation allows. The head
 * keeps one line at every width and ends the name in an ellipsis; hovering or
 * focusing the name shows the whole of it.
 */
export const LongCompanyName: Story = {
  name: "sidebar — a very long company name",
  render: () => {
    stubSession();
    return (
      <StoryProviders>
        <SeedInstallation displayName="Bundesverband der mittelständischen Industrie- und Handelsunternehmen für erneuerbare Energien, nachhaltige Technologien und die digitale Transformation e. V.">
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
 * foot reports it; the posture has its own banner. A frame with an over-cap
 * licence or a seat without `license:read` draws this same picture, so none
 * exists. What each would show is said where the behaviour is, in
 * lib/license-posture.ts.
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
    // Production builds this from the catalog (features/settings/settingsnav.tsx). Here it
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

function LevelStory({
  children,
  locale,
}: Readonly<{ children: ReactNode; locale?: Locale }>) {
  stubSession();
  return (
    <StoryProviders locale={locale}>
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

// German: the privacy entry is wider than the rail, so its row wraps and grows
// while the rows around it keep their height.
export const SectionLevelGerman: Story = {
  name: "second level — a German entry that wraps",
  render: () => (
    <LevelStory locale="de">
      <LevelExample initiallyCollapsed={false} tab="account" />
    </LevelStory>
  ),
  play: async ({ canvasElement }) => {
    await waitFor(() => {
      const rows = [
        ...canvasElement.querySelectorAll<HTMLElement>(".rail .navitem"),
      ];
      const long = rows.find((row) =>
        row.textContent?.includes("Datenschutz und Aufbewahrung"),
      );
      const short = rows.find((row) =>
        row.textContent?.includes("Nutzerkonto"),
      );
      const label = long?.querySelector(".navlabel");
      if (!long || !short || !(label instanceof HTMLElement)) {
        throw new Error("the settings level has no privacy or account row");
      }
      expect(label.scrollWidth).toBeLessThanOrEqual(label.clientWidth);
      expect(long.offsetHeight).toBeGreaterThan(short.offsetHeight);
    });
  },
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
 * A section at phone width: the bar keeps its destinations, and the section
 * lives in the page head, whose switcher opens the other entries. Rendered from
 * the parts, because the real settings section comes from live grants.
 * `uat-phone` makes the capture gate drive the browser to 390px; the viewport
 * tool alone resizes only the manager's iframe.
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

/** The same head in German: the badge moves under the switcher, the name wraps
 *  with the chevron on its first line, and the bar above carries no trail. */
export const SectionPhoneGerman: Story = {
  name: "a section — the head's switcher, a German name that wraps",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => (
    <LevelStory locale="de">
      <PhoneSectionExample />
    </LevelStory>
  ),
  play: async ({ canvasElement }) => {
    await waitFor(() => {
      const name = canvasElement.querySelector(".pageswitch > span");
      const chevron = canvasElement.querySelector(".pageswitch svg");
      if (!(name instanceof HTMLElement) || !(chevron instanceof SVGElement)) {
        throw new Error("the head has no page switch");
      }
      expect(name.textContent).toBe("Datenschutz und Aufbewahrung");
      expect(name.scrollWidth).toBeLessThanOrEqual(name.clientWidth);
      const firstLine = parseFloat(getComputedStyle(name).lineHeight);
      const glyph = chevron.getBoundingClientRect();
      expect(glyph.top + glyph.height / 2).toBeCloseTo(
        name.getBoundingClientRect().top + firstLine / 2,
        0,
      );
      expect(canvasElement.querySelector(".topbar .crumbs")).toBeNull();
    });
  },
};

export const SectionSwitcherOpen: Story = {
  name: "a section — the head's switcher, open",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: SectionPhone.render,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: /change section/ }),
    );
    await within(document.body).findByRole("dialog");
  },
};

/**
 * The More menu open over the bar on Analytics, which the bar does not carry.
 * The bar stays live, More reads Close, and the menu's own row is current.
 */
function PhoneMenuExample() {
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
            <Card as="div">The page under the menu.</Card>
          </div>
        </div>
      </main>
      {palette}
    </div>
  );
}

export const PhoneMoreMenu: Story = {
  name: "phone — the More menu above the bar",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "More" }));
    const close = await canvas.findByRole("button", { name: "Close" });
    await expect(close).toHaveAttribute("aria-expanded", "true");
    const menu = canvasElement.querySelector(".railmenu");
    const bar = canvasElement.querySelector(".rail > .navlevel .navitem");
    if (!(menu instanceof HTMLElement) || !(bar instanceof HTMLElement)) {
      throw new Error("the open menu or the bar under it is missing");
    }
    await expect(menu.getBoundingClientRect().bottom).toBeLessThan(
      bar.getBoundingClientRect().top,
    );
    await expect(
      within(menu).getByRole("link", { name: "Analytics" }),
    ).toHaveAttribute("aria-current", "page");
    // The company's mark stands in the top bar at this width, not in the menu.
    await expect(menu.querySelector(".railhead")).toBeNull();
  },
  render: () => {
    stubEntitlement(WITHIN_CAP);
    return (
      <StoryProviders>
        <SeedInstallation>
          <PhoneMenuExample />
        </SeedInstallation>
      </StoryProviders>
    );
  },
};

/**
 * The phone bar, closed: three destinations, the agent in the middle, and More.
 * The agent is the centre cell because it belongs to the whole session, not to
 * one destination. It is third in the DOM too, so a thumb and Tab read the bar
 * in the same order.
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
 * The agent's panel, open over the phone bar and measured from the well, with a
 * notch pointing back at it. There is no scrim: this is a popover, and the page
 * behind it stays live.
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
