// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { StoryProviders } from "../story-utils";
import { WayOnward } from "./way-onward";

// The button always presses; what still blocks the step is said beside it only
// once it was pressed, so the blocked story has to press it.

const meta: Meta<typeof WayOnward> = {
  title: "Onboarding/Conversation/Way onward",
  component: WayOnward,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  args: {
    label: "Continue",
    stillNeeded: (blockers) => `Still needed: ${blockers.join(", ")}.`,
    onGo: () => undefined,
  },
};
export default meta;

type Story = StoryObj<typeof WayOnward>;

export const Clear: Story = {};

export const BlockedAndPressed: Story = {
  args: { blockers: ["a company name", "a website"] },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Continue" }),
    );
    await expect(await canvas.findByRole("alert")).toHaveTextContent(
      "Still needed: a company name, a website.",
    );
  },
};
