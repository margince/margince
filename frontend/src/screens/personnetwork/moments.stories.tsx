// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { StoryProviders } from "../story-utils";
import "../personnetwork.css";
import { MomentsCard } from "./moments";

// What moved in a relationship lately. The three states are the reason this
// pane exists rather than a list: something moved, nothing did, and the reader
// may not see whether anything did. The last two look the same on a card that
// draws an empty list for both, which is the mistake the section state exists
// to stop.

type Moments = Pick<
  components["schemas"]["Person360"],
  "relationship_changes" | "sections_omitted"
>;

const moved: Moments = {
  sections_omitted: [],
  relationship_changes: [
    { kind: "replied_after_gap", at: "2026-09-02T09:00:00Z", days: 41 },
    {
      kind: "warmed",
      at: "2026-08-27T09:00:00Z",
      from_bucket: "weak",
      to_bucket: "moderate",
    },
  ],
};

function card(view: Moments) {
  return () => (
    <StoryProviders>
      <MomentsCard view={view} />
    </StoryProviders>
  );
}

const meta: Meta<typeof MomentsCard> = {
  title: "Records/Person network/What changed lately",
  component: MomentsCard,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof MomentsCard>;

/** Something moved, and the newest change is the strip's reason to act. */
export const Moved: Story = { render: card(moved) };

/** Nothing has moved — a fact the page HAS, so it is stated as one. */
export const Nothing: Story = {
  render: card({ sections_omitted: [], relationship_changes: [] }),
};

/**
 * Withheld: the reader has no grant on relationship changes, so the pane says
 * it cannot show them rather than drawing the empty list above.
 */
export const Withheld: Story = {
  render: card({
    sections_omitted: ["relationship_changes"],
    relationship_changes: [],
  }),
};

/**
 * Dark, because the row hairlines between moments and the accent badge on the
 * newest one are carried by tokens that move with the theme.
 */
export const MovedDark: Story = {
  name: "Moved — dark",
  globals: { theme: "dark" },
  render: card(moved),
};
