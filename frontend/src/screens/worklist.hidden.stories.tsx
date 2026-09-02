// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "./story-utils";
import { HiddenFigures } from "./worklist.hidden";

// The guardrail's readings — see worklist.hidden.tsx for why they sit behind a
// disclosure and why a healthy answer is a row of zeros.
//
// The FIGURES rather than the panel, because the panel fetches: a story that
// mounted it would draw a loading skeleton and nothing else.

const meta: Meta<typeof HiddenFigures> = {
  title: "Records/Worklist/Hidden backlog",
  component: HiddenFigures,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof HiddenFigures>;

const base = {
  as_of: "2026-09-02T09:00:00Z",
  shown: 12,
  set_aside: 0,
  not_sales: 0,
  past_horizon: 0,
  unlinked: 0,
  truncated: false,
  clear: false,
};

// The case worth looking at: work hidden by a rule NOBODY chose. A customer who
// wrote months ago and was never answered is what this reading exists to find.
export const NobodyChoseThis: Story = {
  args: { backlog: { ...base, past_horizon: 4, unlinked: 2 } },
};

// Every rule holding something, which is what a queue in trouble looks like.
export const EveryRuleHoldingWork: Story = {
  args: {
    backlog: {
      ...base,
      past_horizon: 4,
      unlinked: 2,
      not_sales: 9,
      set_aside: 3,
    },
  },
};

// The read hit its own scan bound, so every figure is a floor. The caveat is
// drawn before any number, because a reader who saw the counts first would have
// drawn a conclusion from them already.
export const CutShortByItsOwnBound: Story = {
  args: {
    backlog: { ...base, shown: 200, truncated: true, past_horizon: 5 },
  },
};
