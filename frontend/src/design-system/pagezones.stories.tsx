// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { PageZones } from "./pagezones";
import { Panel, PanelBody } from "./panel";

// The four shapes, each with enough content in every column to show what the
// grid is actually deciding: which column grows, and which one folds under it.
// Narrow the Storybook viewport past 1200px and then past 720px to see the two
// folds — the work column jumps to full width and stays FIRST, then the rails
// stack under it in one column.
//
// The work column is first in the DOM at every width, so what folds is only
// where the rails are DRAWN. `Folded` below is the width that used to disagree
// with that, and it is a rendered story rather than an instruction to resize
// because a fold nobody has a picture of is a fold nobody checks.

const meta: Meta<typeof PageZones> = {
  title: "Design System/PageZones",
  component: PageZones,
};
export default meta;

type Story = StoryObj<typeof PageZones>;

const work = (
  <Panel title="What is happening">
    <PanelBody>
      <p>
        The work column. It takes the largest share of the page at every shape,
        because it is the column a reader came for — and at 1200px it goes full
        width and keeps its place at the top, ahead of the rails.
      </p>
    </PanelBody>
  </Panel>
);

const context = (
  <Panel title="Context">
    <PanelBody>
      <p>A rail card is a glance rather than something you read.</p>
    </PanelBody>
  </Panel>
);

const business = (
  <Panel title="Around it">
    <PanelBody>
      <p>The business around the subject: who owns it, what it is worth.</p>
    </PanelBody>
  </Panel>
);

// A work column with a rail of context on the RIGHT — the Home screen's shape
// and the company record's.
export const Aside: Story = {
  args: {
    shape: "aside",
    main: work,
    aside: business,
    asideLabel: "Context",
  },
};

// The rail on the LEFT: what the subject IS, read before what is happening to
// it — the person record's shape.
export const Rail: Story = {
  args: {
    shape: "rail",
    main: work,
    rail: context,
    railLabel: "Profile",
  },
};

// Both rails. The work column keeps the largest share (3fr / 5fr / 3fr), which
// is the whole reason the shape is a named template rather than three equal
// columns.
export const Both: Story = {
  args: {
    shape: "both",
    main: work,
    rail: context,
    railLabel: "Profile",
    aside: business,
    asideLabel: "Context",
  },
};

// No rails at all, which is NO grid rather than a one-column one: the work
// column is the page, and a single-track grid would add a rhythm nothing needs.
export const Single: Story = {
  args: {
    shape: "single",
    main: work,
  },
};

/**
 * Both rails at the first fold: the work column goes full width and the two
 * rails sit under it, side by side.
 *
 * Declared here rather than in the shared preview config, and named after the
 * RULE rather than a device: 1024px is a width inside `max-width: 1200px`,
 * which is where the columns fold. Storybook's viewport tool resizes the
 * preview iframe from the manager outside it, so the frame is honest here — and
 * honest under `fe-uat` too, which drives every non-phone story at exactly this
 * width.
 */
const FOLDED_VIEWPORT = {
  viewport: {
    options: {
      folded: {
        name: "Folded (max 1200px)",
        styles: { width: "1024px", height: "900px" },
      },
    },
  },
};

export const Folded: Story = {
  parameters: FOLDED_VIEWPORT,
  globals: { viewport: { value: "folded" } },
  args: {
    shape: "both",
    main: work,
    rail: context,
    railLabel: "Profile",
    aside: business,
    asideLabel: "Context",
  },
};
