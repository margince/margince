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
import { type GrantSpec, READS_DESTINATIONS } from "../testing/mefixture";
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
// account block stands at the end of the top bar (Shell/Account block), which
// is why every frame here renders that bar.
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
 * be named for something it never draws. The seat reads every destination, since
 * the rail leaves out one its seat cannot read; `allow` adds to that.
 */
function stubSession(allow: GrantSpec = {}, about: RouteMap = {}) {
  installFetchStub({
    ...SHELL_ROUTES,
    "GET /me": meRoute({ ...READS_DESTINATIONS, ...allow }),
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
 * A named page: the heading starts and ends where the card under it does, both
 * on the page gutter. Put a ruler on the left edge of the title and
 * the left edge of the card.
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
 * One sidebar state, in the `.app` grid that gives it its width.
 * The empty content card shows the edge between content and chrome.
 * The toggle at the rail's foot is live, so the transition can be watched.
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
        onToggle={() => setCollapsed((current) => !current)}
      />
      <main className="main">
        <TopBar route={{ screen: "deals" }} onOpenSearch={openSearch} />
        <div className="scroll" />
      </main>
      {palette}
    </div>
  );
}

// Both sidebar states, side by side, under the top bar that holds the search
// and the account block. Collapsed, each row is a 32px square.
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

export const SidebarStatesDark: Story = {
  ...SidebarStates,
  name: "sidebar — expanded and collapsed, dark",
  globals: { theme: "dark" },
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
 * A licence is stubbed because the shell's providers want one. The foot does
 * not report it; the posture has its own banner (lib/license-posture.ts).
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
              labelKey: "settings.tab.records",
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
// The story renders the top bar too, since the search lives there.
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
        onToggle={() => setCollapsed((current) => !current)}
      />
      <main className="main">
        <TopBar
          route={{ screen: "settings", id: tab, id2: sub }}
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

// German: the privacy entry is wider than the rail, so its label is cut on one
// line. The row keeps its height, and its tip carries the whole label.
export const SectionLevelGerman: Story = {
  name: "second level, a German entry cut to one line",
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
      if (!long || !short) {
        throw new Error("the settings level has no privacy or account row");
      }
      expect(long.offsetHeight).toBe(short.offsetHeight);
    });
    const privacy = within(canvasElement).getByRole("link", {
      name: "Datenschutz und Aufbewahrung",
    });
    privacy.focus();
    await waitFor(() =>
      expect(within(canvasElement).getByRole("tooltip").textContent).toBe(
        "Datenschutz und Aufbewahrung",
      ),
    );
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
        <TopBar route={route} onOpenSearch={openSearch} />
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
      const firstLine = Number.parseFloat(getComputedStyle(name).lineHeight);
      const glyph = chevron.getBoundingClientRect();
      expect(glyph.top + glyph.height / 2).toBeCloseTo(
        name.getBoundingClientRect().top + firstLine / 2,
        0,
      );
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
        <TopBar route={route} onOpenSearch={openSearch} />
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
    // The burger and the cross, as drawn once the turn between them settles.
    const glyphs = () =>
      [".railmore-open", ".railmore-close"].map((glyph) => {
        const drawn = canvasElement.querySelector(glyph);
        return drawn ? getComputedStyle(drawn).opacity : "missing";
      });
    const turned = (burger: string, cross: string) =>
      waitFor(() => expect(glyphs()).toEqual([burger, cross]));
    await turned("1", "0");
    await userEvent.click(canvas.getByRole("button", { name: "More" }));
    await turned("0", "1");
    await userEvent.click(canvas.getByRole("button", { name: "Close" }));
    await turned("1", "0");
    await userEvent.click(canvas.getByRole("button", { name: "More" }));
    await turned("0", "1");
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
        <TopBar route={route} onOpenSearch={openSearch} />
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

export const PhoneBarDark: Story = {
  ...PhoneBar,
  name: "phone — the bar, dark",
  globals: { viewport: { value: "phone" }, theme: "dark" },
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
      await canvas.findByRole("button", { name: en["agent.rail.open"] }),
    );
    await within(canvasElement.ownerDocument.body).findByRole("switch", {
      name: en["agent.setting.edgeLight"],
    });
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

// A frameless page paints its own ground to the viewport edge, so its scroller
// reserves no gutter, short page or long. Classic scrollbars are the case the
// default Mac look hides, so the computed style is asserted rather than seen.
function framelessScroller(canvasElement: HTMLElement) {
  const scroller = canvasElement.querySelector(".app.railless .scroll");
  if (!scroller) throw new Error("no frameless scroller on this frame");
  return scroller;
}

const LONG_PAGE = Array.from({ length: 40 }, (_, n) => `Content ${n + 1}`);

function FramelessPage({ rows }: Readonly<{ rows: readonly string[] }>) {
  stubSession();
  return (
    <StoryProviders>
      <SeedInstallation>
        <ShellExample at="#/onboarding">
          <div className="wrap">
            {rows.map((row) => (
              <Card as="div" key={row}>
                {row}
              </Card>
            ))}
          </div>
        </ShellExample>
      </SeedInstallation>
    </StoryProviders>
  );
}

export const FramelessPageShort: Story = {
  name: "a frameless page — short, no reserved gutter",
  render: () => <FramelessPage rows={["Content"]} />,
  play: async ({ canvasElement }) => {
    await waitFor(() =>
      expect(
        getComputedStyle(framelessScroller(canvasElement)).scrollbarGutter,
      ).toBe("auto"),
    );
  },
};

export const FramelessPageOverflowing: Story = {
  name: "a frameless page — overflowing, no reserved gutter",
  render: () => <FramelessPage rows={LONG_PAGE} />,
  play: FramelessPageShort.play,
};
