// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Panel } from "../design-system/panel";
import { ListsSection } from "./companyraillists";
import { shortlist } from "./lists.fixtures";
import { StoryProviders } from "./story-utils";

// The Shortlists an account is on, as one more slice of the company rail.
const meta: Meta = { title: "Records/Company rail/Lists" };
export default meta;

type Story = StoryObj;

export const OnTwoShortlists: Story = {
  render: () => (
    <StoryProviders>
      <Panel>
        <ListsSection
          lists={[
            shortlist,
            {
              ...shortlist,
              id: "01a0f000-0000-7000-8000-000000000009",
              name: "Dinner guests",
            },
          ]}
        />
      </Panel>
    </StoryProviders>
  ),
};
