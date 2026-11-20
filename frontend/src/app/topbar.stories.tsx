// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { expect, waitFor, within } from "storybook/test";
import { Card } from "../design-system/atoms";
import type { Route } from "../lib/router";
import { type GrantSpec, READS_DESTINATIONS } from "../testing/mefixture";
import {
  setupReminderGrants,
  setupReminderRoutes,
} from "../testing/setupreminder.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
  WithInstallationBrand,
} from "../testing/story-utils";
import { type NavCounts, recordKindOf } from "./nav";
import { CommandPalette, useBuiltinCommands } from "./palette";
import { PageTitle, WorkspaceRail } from "./shell";
import { TopBar } from "./topbar";

/**
 * The strip that is true of the whole session: the search leads, and what is
 * waiting and who is signed in stand at the end. Every frame renders the real
 * frame: the `.app` grid, the sidebar, and the bar over the content card.
 * Above phone width the search starts on the card's left edge.
 */
const meta: Meta<typeof TopBar> = {
  title: "Shell/Top bar",
  component: TopBar,
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof TopBar>;

// One queue, read by the sidebar and by the bell, as the shell hands it.
const COUNTS = { inbox: 12, tasks: 4 };
const NOTHING_WAITING = { inbox: 0, tasks: 0 };

const WIDE_VIEWPORT = {
  viewport: {
    options: {
      wide: { name: "Wide", styles: { width: "1440px", height: "900px" } },
    },
  },
};

// 960px is inside the bar's `max-width: 1100px` rule.
const NARROW_VIEWPORT = {
  viewport: {
    options: {
      narrow: {
        name: "Narrow (max 1100px)",
        styles: { width: "960px", height: "720px" },
      },
    },
  },
};

// The seat reads every destination, so the rail beside the bar is the whole one.
function stubSession(routes: RouteMap = {}, allow: GrantSpec = {}) {
  installFetchStub({
    "GET /me": meRoute({ ...READS_DESTINATIONS, ...allow }),
    ...routes,
  });
}

// The record's name is seeded, so the page under the bar shows it.
function SeedCache({
  record,
  kind = "contact",
  children,
}: Readonly<{
  record?: { id: string; name: string };
  kind?: string;
  children: ReactNode;
}>) {
  const client = useQueryClient();
  if (record && client.getQueryData([kind, "ref", record.id]) === undefined) {
    client.setQueryData([kind, "ref", record.id], record.name);
  }
  return (
    <WithInstallationBrand brand={{ display_name: "Gradion GmbH" }}>
      {children}
    </WithInstallationBrand>
  );
}

// The search opens the command palette, so the frame mounts it as App.tsx does.
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

function BarFrame({
  route,
  counts = COUNTS,
  startCollapsed = false,
}: Readonly<{ route: Route; counts?: NavCounts; startCollapsed?: boolean }>) {
  const [collapsed, setCollapsed] = useState(startCollapsed);
  const { openSearch, palette } = usePaletteSeam();
  return (
    <div className={collapsed ? "app" : "app railexpanded"}>
      <WorkspaceRail
        route={route}
        counts={counts}
        collapsed={collapsed}
        onToggle={() => setCollapsed((current) => !current)}
      />
      <main className="main">
        <TopBar route={route} onOpenSearch={openSearch} />
        <div className="scroll">
          <PageTitle route={route} />
          <div className="wrap">
            <Card as="div">The page, under the strip.</Card>
          </div>
        </div>
      </main>
      {palette}
    </div>
  );
}

function BarStory({
  route,
  counts,
  startCollapsed,
  record,
  routes,
  allow,
}: Readonly<{
  route: Route;
  counts?: NavCounts;
  startCollapsed?: boolean;
  record?: { id: string; name: string };
  /** Reads beyond the session, for a frame that draws the end controls' extras. */
  routes?: RouteMap;
  allow?: GrantSpec;
}>) {
  stubSession(routes, allow);
  return (
    <StoryProviders>
      <SeedCache record={record} kind={recordKindOf(route)}>
        <BarFrame
          route={route}
          counts={counts}
          startCollapsed={startCollapsed}
        />
      </SeedCache>
    </StoryProviders>
  );
}

/** A list route: the search on the card's left edge, the controls at the end. */
export const ListRoute: Story = {
  name: "a list route",
  render: () => <BarStory route={{ screen: "deals" }} />,
};

/** The same route with the sidebar at 56px; the search moves with the card. */
export const ListRouteCollapsed: Story = {
  name: "a list route, sidebar collapsed",
  render: () => <BarStory route={{ screen: "deals" }} startCollapsed />,
};

/** Nobody waiting: the bell keeps its place and wears no chip. */
export const NothingWaiting: Story = {
  name: "nothing waiting, the bell is silent",
  render: () => (
    <BarStory route={{ screen: "deals" }} counts={NOTHING_WAITING} />
  ),
};

/** A record: its back control leads the bar, before the search. */
export const RecordRoute: Story = {
  name: "a record, the back control leading",
  render: () => (
    <BarStory
      route={{ screen: "contacts", id: "p-anna" }}
      record={{ id: "p-anna", name: "Anna Weber" }}
    />
  ),
};

/**
 * A deal page with the setup reminder and the scheduled-messages clock up.
 *
 * The deal's long name stays in the page heading, so the bar holds the Back
 * control, the search and the end controls. Those keep their own width: the
 * reminder's words stay whole and no glyph draws over another.
 */
export const RecordBesideTheReminder: Story = {
  name: "a record page beside the setup reminder",
  parameters: WIDE_VIEWPORT,
  globals: { viewport: { value: "wide" } },
  tags: ["uat-wide"],
  render: () => (
    <BarStory
      route={{ screen: "deals", id: "d-long" }}
      record={{
        id: "d-long",
        name:
          "Framework agreement for the cold-chain fleet renewal across the " +
          "Hamburg, Rotterdam and Antwerp depots, phase two",
      }}
      routes={{
        ...setupReminderRoutes(),
        "GET /scheduled-sends": () => jsonResponse([]),
      }}
      allow={setupReminderGrants()}
    />
  ),
  play: async ({ canvasElement }) => {
    await waitFor(() => {
      const end = canvasElement.querySelector(".topbar-trail");
      const words = end?.querySelector(".setup-reminder-word");
      const search = canvasElement.querySelector(".topbar-searchslot");
      const lead = canvasElement.querySelector(".topbar-lead");
      if (!end?.querySelector(".outbox") || !words || !search || !lead) {
        throw new Error("the bar has no reminder, clock, search or lead yet");
      }
      expect(words.scrollWidth).toBeLessThanOrEqual(words.clientWidth);
      const boxes = [...end.children].map((c) => c.getBoundingClientRect());
      for (const [i, box] of boxes.entries()) {
        expect(box.left).toBeGreaterThanOrEqual((boxes[i - 1]?.right ?? 0) - 1);
      }
      expect(search.getBoundingClientRect().left).toBeGreaterThanOrEqual(
        lead.getBoundingClientRect().right,
      );
      const bar = canvasElement.querySelector(".topbar");
      expect(boxes.at(-1)?.right ?? 0).toBeLessThanOrEqual(
        bar?.getBoundingClientRect().right ?? 0,
      );
    });
  },
};

/** Under 1100px the search drops to its glyph, never off the bar. */
export const Narrow: Story = {
  name: "under 1100px, search drops to its glyph",
  parameters: NARROW_VIEWPORT,
  globals: { viewport: { value: "narrow" } },
  render: () => (
    <BarStory
      route={{ screen: "contacts", id: "p-anna" }}
      record={{ id: "p-anna", name: "Anna Weber" }}
    />
  ),
};

/**
 * Phone width, where the sidebar is the bottom bar and the company's mark leads
 * the strip. `uat-phone` drives the capture gate to 390px.
 */
export const Phone: Story = {
  name: "phone, the company's mark leading",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => <BarStory route={{ screen: "deals" }} />,
  play: async ({ canvasElement }) => {
    const bar = canvasElement.querySelector<HTMLElement>(".topbar");
    if (!bar) {
      throw new Error("the top bar is missing");
    }
    // The chip's initials lead the name while it prints them (WCAG 2.5.3).
    const home = await within(bar).findByRole("link", {
      name: /^(GG )?Gradion GmbH home, powered by Margince$/,
    });
    await expect(home).toHaveAttribute("href", "/#/home");
    // Each glyph is one control square inside the bar, a --space-1 from the next.
    const edge = bar.getBoundingClientRect();
    const token = (name: string) =>
      Number.parseFloat(getComputedStyle(bar).getPropertyValue(name));
    const boxes = [
      ...bar.querySelectorAll(".topbar-search, .topbar-trail button"),
    ].map((control) => control.getBoundingClientRect());
    await expect(boxes.length).toBeGreaterThan(1);
    for (const [at, box] of boxes.entries()) {
      await expect(box.width).toBe(token("--controlHeight"));
      await expect(box.height).toBe(token("--controlHeight"));
      await expect(box.top).toBeGreaterThanOrEqual(edge.top);
      await expect(box.bottom).toBeLessThanOrEqual(edge.bottom);
      const before = boxes[at - 1];
      if (before) {
        await expect(box.left - before.right).toBe(token("--space-1"));
      }
    }
  },
};
