// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { StoryProviders } from "../../testing/story-utils";
import { InternalMailHistoryChoice } from "./internalmailhistorychoice";

const meta = {
  title: "Records/Capture/History choice",
  component: InternalMailHistoryChoice,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof InternalMailHistoryChoice>;
export default meta;
type Story = StoryObj<typeof meta>;
function Choice(args: Parameters<typeof InternalMailHistoryChoice>[0]) {
  const [checked, setChecked] = useState(args.checked);
  return (
    <InternalMailHistoryChoice
      {...args}
      checked={checked}
      onChange={setChecked}
    />
  );
}
export const Offered: Story = {
  args: {
    offer: {
      preview_token: "preview-1",
      received_after: "2026-01-01T00:00:00Z",
      folders: ["Operations"],
    },
    checked: false,
    disabled: false,
    onChange: () => {},
  },
  render: (args) => <Choice {...args} />,
  play: async ({ canvasElement }) => {
    const checkbox = await within(canvasElement).findByRole("checkbox");
    await userEvent.click(checkbox);
    await expect(checkbox).toBeChecked();
  },
};
export const Disabled: Story = {
  ...Offered,
  play: undefined,
  args: { ...Offered.args, disabled: true },
};
export const AllFoldersDark: Story = {
  ...Offered,
  globals: { theme: "dark" },
  play: undefined,
  args: {
    ...Offered.args,
    offer: {
      preview_token: "preview-1",
      received_after: "2026-01-01T00:00:00Z",
      folders: [],
    },
  },
};
