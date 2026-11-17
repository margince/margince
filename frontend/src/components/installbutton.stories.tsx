// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { en } from "../i18n/en";
import { StoryProviders } from "../testing/story-utils";
import { InstallButton } from "./installbutton";

// The browser's dialog never answers here, so a press stays pending.
const unanswered = () => new Promise<never>(() => undefined);

const meta: Meta<typeof InstallButton> = {
  title: "Components/Install button",
  component: InstallButton,
  render: (args) => (
    <StoryProviders>
      <InstallButton {...args} />
    </StoryProviders>
  ),
  args: { prompt: unanswered },
};
export default meta;
type Story = StoryObj<typeof InstallButton>;

export const Idle: Story = {};

// The browser's install window is open: the button waits for its answer.
export const Pending: Story = {
  play: async ({ canvasElement }) => {
    const install = await within(canvasElement).findByRole("button", {
      name: en["settings.installAppAction"],
    });
    await userEvent.click(install);
    await expect(install).toHaveAttribute("aria-busy", "true");
  },
};
