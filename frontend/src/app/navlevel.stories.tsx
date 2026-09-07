// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { House, UserRound } from "lucide-react";
import { StoryProviders } from "../screens/story-utils";
import { type NavSection, railTrail } from "./nav";
import { NavLevelView } from "./navlevel";

// One level of the sidebar as the rail draws it: the rows, their groups, the
// badge a row carries, and the way back up when the reader has drilled.
//
// The levels come from `railTrail` rather than from a hand-built fixture. A
// story that invented its own rows would be reviewing a rail nobody ships —
// and the interesting property of this component is that it does NOT know its
// own depth, which only holds if the level it is handed is a real one.
//
// A row's badge is an attention count, and it rides the LEVEL rather than
// being read from module scope inside a row, so the counts below are handed in
// the way the shell hands them.

const RESTING = { collapsed: false, tip: null, onTip: () => {} };
const COLLAPSED = { collapsed: true, tip: null, onTip: () => {} };

// The one section the app publishes is settings, and it is assembled from live
// grants — so the drilled level below is a fixture of the same SHAPE: a group
// carrying the level's own name with the Overview row in it, then a subject
// group. Handed to `railTrail`, because a route with no section answers with the
// primary level alone and the drilled story would be a second picture of it.
const SETTINGS: NavSection = {
  screen: "settings",
  titleKey: "nav.settings",
  activeId: "account",
  groups: [
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
      ],
    },
  ],
};

const primary = railTrail({ screen: "brief" })[0];
const settings = railTrail({ screen: "settings", id: "account" }, SETTINGS);

function level(
  which: (typeof settings)[number],
  state: typeof RESTING,
  counts?: Record<string, number>,
  parent?: (typeof settings)[number],
) {
  return () => (
    <StoryProviders>
      <nav className={state.collapsed ? "rail collapsed" : "rail expanded"}>
        <NavLevelView
          level={which}
          parent={parent}
          counts={counts}
          state={state}
          onSelect={() => {}}
          onWalkUp={() => {}}
        />
      </nav>
    </StoryProviders>
  );
}

const meta: Meta<typeof NavLevelView> = {
  title: "Shell/Nav level",
  component: NavLevelView,
};
export default meta;
type Story = StoryObj<typeof NavLevelView>;

/** The primary level: every destination, grouped, with nothing waiting. */
export const Primary: Story = { render: level(primary, RESTING) };

/**
 * Rows carrying counts. The badge is what a reader scans for, so the figure is
 * written in their own notation — at four digits that is the only thing telling
 * a German rail from an English one, and a rail is exactly where a bare `1204`
 * would sit beside a formatted figure elsewhere on the page.
 */
export const WithCounts: Story = {
  render: level(primary, RESTING, { tasks: 1204, inbox: 12 }),
};

/** The same counts in German. */
export const WithCountsGerman: Story = {
  render: () => (
    <StoryProviders locale="de">
      <nav className="rail expanded">
        <NavLevelView
          level={primary}
          counts={{ tasks: 1204, inbox: 12 }}
          state={RESTING}
          onSelect={() => {}}
          onWalkUp={() => {}}
        />
      </nav>
    </StoryProviders>
  ),
};

/**
 * The collapsed rail: the labels go and the rows keep their targets, so the
 * badge has to survive without the word it was counting. The accessible name
 * carries what the label no longer shows.
 */
export const Collapsed: Story = {
  render: level(primary, COLLAPSED, { tasks: 1204, inbox: 12 }),
};

/**
 * A drilled level, which is the case the component exists for: the same rows in
 * the same groups, named by the heading over the first of them, with the way out
 * of the section above them.
 */
export const Drilled: Story = {
  render: level(settings[1], RESTING, undefined, settings[0]),
};

/** At 390px the rail is the phone's own bar rather than a column, so the rows
 *  are judged at the width they are actually pressed at. */
export const Phone: Story = {
  tags: ["uat-phone"],
  render: level(primary, RESTING, { tasks: 1204 }),
};
