// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { RelationshipMap, type RelationshipMapLabels } from "./relationshipmap";
import type { RelationshipMapModel } from "./relationshipmap.layout";

// The account's routes, in the states a live stack rarely shows all of.
//
// A seeded demo account has a complete committee and a full graph, so the
// readings that matter most — a hole in the buying team, a capped picture, an
// account with nothing recorded — are only visible here.

const LABELS: RelationshipMapLabels = {
  region: "Who can reach whom at this account",
  band: { strong: "strong", developing: "developing", cold: "cold" },
  bestRoute: "Best route",
  alternatives: "Alternatives",
  noRoute: "No route recorded",
  laneMore: (hidden) => `Show ${hidden} more`,
  clearFocus: "Clear selection",
  emptyTitle: "No route recorded yet",
  emptyBody:
    "Start by assigning the buying roles, or import the interactions this account already has.",
  nothingSelected: "Select a person to see the best route into them.",
};

const FULL: RelationshipMapModel = {
  nodes: [
    {
      id: "u-1",
      kind: "user",
      label: "Sofia Meier",
      sublabel: "3 contacts here",
    },
    {
      id: "u-2",
      kind: "user",
      label: "Lars Meyer",
      sublabel: "2 contacts here",
    },
    { id: "u-3", kind: "user", label: "Mei Kato", sublabel: "1 contact here" },
    {
      id: "o-1",
      kind: "organization",
      label: "Brandt GmbH",
      sublabel: "Account",
    },
    { id: "d-1", kind: "deal", label: "Retrofit 2026", sublabel: "Proposal" },
    {
      id: "p-1",
      kind: "person",
      label: "Philipp Königs",
      sublabel: "CFO",
      engagement: "untried",
      engagementLabel: "Not approached",
      actions: [
        { id: "write", label: "Write to Philipp", primary: true },
        { id: "open", label: "Open contact" },
      ],
    },
    {
      id: "p-2",
      kind: "person",
      label: "Anne Wiegert",
      sublabel: "Head of Operations",
      engagement: "answered",
      engagementLabel: "Answered",
    },
    {
      id: "p-3",
      kind: "person",
      label: "Jan Roth",
      sublabel: "Workshop lead",
      engagement: "no_reply",
      engagementLabel: "No reply",
    },
    {
      id: "p-4",
      kind: "person",
      label: "Sabine Vogel",
      sublabel: "Head of Partnerships",
      engagement: "waiting",
      engagementLabel: "Needs reply",
    },
  ],
  lanes: [
    {
      id: "users",
      column: "left",
      label: "Our side",
      nodeIds: ["u-1", "u-2", "u-3"],
    },
    { id: "centre", column: "center", label: "", nodeIds: ["o-1", "d-1"] },
    {
      id: "economic_buyer",
      column: "right",
      label: "Economic buyer",
      nodeIds: ["p-1"],
    },
    {
      id: "influencer",
      column: "right",
      label: "Influencers",
      nodeIds: ["p-2"],
    },
    { id: "user", column: "right", label: "Users", nodeIds: ["p-3", "p-4"] },
  ],
  edges: [
    {
      id: "e-1",
      from: "u-1",
      to: "p-1",
      kind: "route",
      band: "developing",
      lastAt: "2026-08-20T09:00:00Z",
      words: "awaiting reply · sent 12 days ago",
    },
    {
      id: "e-2",
      from: "u-2",
      to: "p-1",
      kind: "route",
      band: "cold",
      words: "never written to",
    },
    {
      id: "e-3",
      from: "u-2",
      to: "p-2",
      kind: "route",
      band: "strong",
      lastAt: "2026-08-28T09:00:00Z",
      words: "last reply from buyer · 3 days ago",
    },
    {
      id: "m-1",
      from: "p-1",
      to: "d-1",
      kind: "membership",
      words: "on the deal",
    },
    {
      id: "m-2",
      from: "p-2",
      to: "d-1",
      kind: "membership",
      words: "on the deal",
    },
  ],
};

const meta = {
  title: "Design System/Relationship map",
  component: RelationshipMap,
  parameters: { layout: "padded" },
  args: {
    model: FULL,
    focusId: null,
    onFocus: () => {},
    completenessText: "Showing 3 of 105 contacts · selected deal only.",
    labels: LABELS,
  },
} satisfies Meta<typeof RelationshipMap>;

export default meta;
type Story = StoryObj<typeof meta>;

/** At rest: every node named, every band told apart by thickness. */
export const Resting: Story = {};

/**
 * The moment the map exists for. Sofia's route to Philipp lights end to end,
 * everything unrelated recedes, and the panel says WHY that route and not the
 * other — with the rejected one listed underneath.
 */
export const RouteHighlighted: Story = { args: { focusId: "p-1" } };

/** A colleague focused: the same walk, from our side. */
export const ColleagueFocused: Story = { args: { focusId: "u-2" } };

/**
 * A buying team with a hole. The gap is a node in the lane that would hold
 * them, dashed and in the warning family, rather than a sentence elsewhere.
 */
export const MissingChampion: Story = {
  args: {
    model: {
      ...FULL,
      nodes: [
        ...FULL.nodes,
        {
          id: "gap:champion",
          kind: "gap",
          label: "Champion missing",
          sublabel: "Nobody is carrying this deal",
        },
      ],
      lanes: [
        FULL.lanes[0],
        FULL.lanes[1],
        {
          id: "champion",
          column: "right",
          label: "Champion",
          nodeIds: ["gap:champion"],
        },
        ...FULL.lanes.slice(2),
      ],
    },
  },
};

/**
 * A contact nobody can reach. The map still draws them — being unreachable is
 * the finding — and the panel says so instead of showing an empty space.
 */
export const NoRoute: Story = {
  args: { model: { ...FULL, edges: [] }, focusId: "p-1" },
};

/** A lane longer than the cap draws what fits and offers the rest. */
export const LargeAccount: Story = {
  args: {
    model: {
      nodes: [
        { id: "u-1", kind: "user", label: "Sofia Meier" },
        ...Array.from({ length: 14 }, (_, i) => ({
          id: `p-${i}`,
          kind: "person" as const,
          label: `Contact ${i + 1}`,
          engagement: "untried" as const,
          engagementLabel: "Not approached",
        })),
      ],
      lanes: [
        { id: "users", column: "left", label: "Our side", nodeIds: ["u-1"] },
        {
          id: "influencer",
          column: "right",
          label: "Influencers",
          nodeIds: Array.from({ length: 14 }, (_, i) => `p-${i}`),
        },
      ],
      edges: [],
    },
    completenessText: "Showing 8 of 105 contacts · selected deal only.",
  },
};

/** Nothing recorded yet, and what to do about it. */
export const Empty: Story = {
  args: { model: { nodes: [], lanes: [], edges: [] }, completenessText: "" },
};

/** The still version: no transitions, for a reader who asked for less motion. */
export const ReducedMotion: Story = {
  args: { focusId: "p-1", reducedMotion: true },
};

/**
 * The map in the column it actually gets on a record page with the details
 * pane open — about half the window. The panel folds under the picture and
 * the picture fits its box, so nothing is cut off at the right edge; the
 * fold reads this column's width, not the window's.
 */
export const BesideDetailsPane: Story = {
  args: { focusId: "p-1" },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: "46rem" }}>
        <Story />
      </div>
    ),
  ],
};
