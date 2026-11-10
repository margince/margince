// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { installFetchStub, StoryProviders } from "../../testing/story-utils";
import { ReasonSwitch } from "./composereason";

const meta: Meta<typeof ReasonSwitch> = {
  title: "Patterns/Compose mail/Recorded alternative",
  component: ReasonSwitch,
  args: { reason: "requested_followup", onSwitch: fn() },
  render: (args) => {
    installFetchStub({});
    return (
      <StoryProviders>
        <ReasonSwitch {...args} />
      </StoryProviders>
    );
  },
};
export default meta;
type Story = StoryObj<typeof meta>;

export const SupportedFollowUp: Story = {
  play: async ({ canvasElement, args }) => {
    const button = within(canvasElement).getByRole("button", {
      name: /^Use .*instead$/,
    });
    await userEvent.click(button);
    await expect(args.onSwitch).toHaveBeenCalledWith("requested_followup");
  },
};

export const SupportedFollowUpPhone: Story = {
  ...SupportedFollowUp,
  tags: ["uat-phone"],
};
