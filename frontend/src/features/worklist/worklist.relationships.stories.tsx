// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { en as englishCopy } from "../../i18n/en";
import { StoryProviders, stubWithSession } from "../../testing/story-utils";
import { row } from "./worklist.fixtures";
import { OptionalRelationships } from "./worklist.relationships";

const meta: Meta<typeof OptionalRelationships> = {
  title: "Records/Worklist/Optional relationships",
  component: OptionalRelationships,
  decorators: [
    (Story) => {
      stubWithSession({}, {});
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
  args: {
    owner: "",
    scope: "mine",
    day: {
      optional_relationships: [
        row({
          id: "01930000-0000-7000-8000-000000000031",
          source: "relationship_decay",
          category: "relationships",
          level: 6,
          title: "Demo contact",
          consequence: "data_drifts",
          because: [{ kind: "quiet_days", value: { kind: "days", days: 180 } }],
          actions: ["open", "dismiss"],
          move: { action: "draft_email" },
        }),
        row({
          id: "01930000-0000-7000-8000-000000000032",
          source: "relationship_decay",
          category: "relationships",
          level: 6,
          title: "Second demo contact",
          consequence: "data_drifts",
          because: [{ kind: "quiet_days", value: { kind: "days", days: 120 } }],
          actions: ["open", "dismiss"],
        }),
      ],
      optional_relationships_more_available: true,
    },
  },
};
export default meta;
type Story = StoryObj<typeof OptionalRelationships>;
export const Optional: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText(englishCopy["worklist.relationships.optional"]),
    ).toBeVisible();
    await expect(
      canvas.getByText("No recent contact with Demo contact"),
    ).toBeVisible();
    await expect(
      canvas.getByText(englishCopy["worklist.relationships.more"]),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", {
        name: englishCopy["worklist.verb.dismissAll"],
      }),
    ).toBeVisible();
    await expect(
      canvas.getAllByRole("button", {
        name: englishCopy["worklist.verb.stopFollowing"],
      }),
    ).toHaveLength(2);
  },
};
export const Dark: Story = { ...Optional, globals: { theme: "dark" } };
export const BoundedEmpty: Story = {
  args: {
    day: {
      optional_relationships: [],
      optional_relationships_more_available: true,
    },
  },
};
