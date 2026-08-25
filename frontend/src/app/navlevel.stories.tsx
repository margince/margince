// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../screens/story-utils";
import { railTrail } from "./nav";
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

const primary = railTrail({ screen: "home" })[0];
const settings = railTrail({ screen: "settings" });

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
 * A drilled level, which is the case the component exists for: it prints its
 * own heading, pushes the group labels a heading level down, and offers the way
 * back to the level above.
 */
export const Drilled: Story = {
  render: () =>
    settings.length > 1
      ? level(settings[1], RESTING, undefined, settings[0])()
      : level(primary, RESTING)(),
};

/** At 390px the rail is the phone's own bar rather than a column, so the rows
 *  are judged at the width they are actually pressed at. */
export const Phone: Story = {
  tags: ["uat-phone"],
  render: level(primary, RESTING, { tasks: 1204 }),
};
