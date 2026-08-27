// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { StoryProviders } from "../story-utils";
import { DealCommitteeMap } from "./dealcommittee";

// The buying committee, drawn.
//
// The states worth seeing are the ones that render identically if their
// distinction is dropped: a WITHHELD coverage read against an empty one, and a
// well-threaded deal against a single-threaded one. A map that showed those
// pairs the same way would report a clean deal from a check that never ran.

type DealCoverage = components["schemas"]["DealCoverage"];

const meta: Meta<typeof DealCommitteeMap> = {
  title: "Records/Deal committee",
  component: DealCommitteeMap,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <div style={{ maxWidth: 420 }}>
          <Story />
        </div>
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof DealCommitteeMap>;

const DEAL_ID = "01a03000-0000-7000-8000-000000000001";

const coverage = (over: Partial<DealCoverage> = {}): DealCoverage => ({
  deal_id: DEAL_ID,
  stakeholders: [
    {
      person_id: "01a03000-0000-7000-8000-0000000000b1",
      person_name: "Dana Weiss",
      role: "champion",
      engaged: true,
    },
    {
      person_id: "01a03000-0000-7000-8000-0000000000b2",
      person_name: "Tomas Berg",
      role: "economic_buyer",
      engaged: true,
    },
    {
      person_id: "01a03000-0000-7000-8000-0000000000b3",
      person_name: "Ines Kraft",
      role: "evaluator",
      engaged: false,
    },
  ],
  our_side: [
    {
      user_id: "01a03000-0000-7000-8000-0000000000c1",
      display_name: "Lena Fischer",
      strength_bucket: "strong",
      interactions_90d: 24,
      last_at: "2026-08-20T09:00:00Z",
    },
  ],
  risks: [],
  sections_omitted: [],
  ...over,
});

/** A deal threaded through more than one engaged stakeholder. */
export const WellThreaded: Story = {
  args: {
    coverage: coverage(),
    withheld: false,
    pending: false,
    overlay: false,
  },
};

/**
 * One engaged contact and a gap. The ghost seat is the point: the missing
 * cover has no row in a list of who exists, and only the map can count it.
 */
export const SingleThreaded: Story = {
  args: {
    coverage: coverage({
      stakeholders: [
        {
          person_id: "01a03000-0000-7000-8000-0000000000b1",
          person_name: "Dana Weiss",
          role: "champion",
          engaged: true,
        },
        {
          person_id: "01a03000-0000-7000-8000-0000000000b3",
          person_name: "Ines Kraft",
          role: "evaluator",
          engaged: false,
        },
      ],
      risks: [
        {
          kind: "single_threaded_theirs",
          summary: "Only one person here is talking to us.",
          person_ids: ["01a03000-0000-7000-8000-0000000000b1"],
        },
      ],
    }),
    withheld: false,
    pending: false,
    overlay: false,
  },
};

/**
 * A seat the reader may not name. The seat still counts toward coverage — how
 * many people carry a deal is not the fact being withheld, only who they are.
 */
export const SeatWithoutAName: Story = {
  args: {
    coverage: coverage({
      stakeholders: [
        {
          person_id: "01a03000-0000-7000-8000-0000000000b1",
          person_name: "Dana Weiss",
          role: "champion",
          engaged: true,
        },
        {
          person_id: "01a03000-0000-7000-8000-0000000000b9",
          person_name: null,
          role: "evaluator",
          engaged: false,
        },
      ],
    }),
    withheld: false,
    pending: false,
    overlay: false,
  },
};

/**
 * WITHHELD, not empty. Without the relationship grant a reader is served no
 * seats at all, and drawing that as an uncovered deal would be a finding from
 * a check that never ran.
 */
export const Withheld: Story = {
  args: {
    coverage: coverage({
      stakeholders: [],
      our_side: [],
      risks: [],
      sections_omitted: ["stakeholders", "our_side", "risks"],
    }),
    withheld: true,
    pending: false,
    overlay: false,
  },
};

/** A deal with nobody recorded on it. */
export const Empty: Story = {
  args: {
    coverage: coverage({ stakeholders: [], our_side: [] }),
    withheld: false,
    pending: false,
    overlay: false,
  },
};
