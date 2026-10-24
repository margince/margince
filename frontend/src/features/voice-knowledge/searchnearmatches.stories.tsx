// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../testing/story-utils";
import { NearMatches } from "./searchnearmatches";

const meta: Meta<typeof NearMatches> = {
  title: "Patterns/Search near matches",
  component: NearMatches,
};
export default meta;
type Story = StoryObj<typeof NearMatches>;

export const Names: Story = {
  render: () => (
    <StoryProviders>
      <NearMatches
        matches={[
          { type: "contact", id: "c1", title: "Anna Kowalczyk" },
          {
            type: "lead",
            id: "l1",
            title: "Ida Fenn",
            snippet: "Brandtmeier Logistik",
          },
        ]}
      />
    </StoryProviders>
  ),
};
