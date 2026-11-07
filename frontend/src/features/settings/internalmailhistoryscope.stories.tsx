// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { InternalMailHistoryScope } from "./internalmailhistoryscope";

const meta = {
  title: "Settings/Data/Capture rules/History scope",
  component: InternalMailHistoryScope,
} satisfies Meta<typeof InternalMailHistoryScope>;
export default meta;
type Story = StoryObj<typeof meta>;
function Editable(args: Parameters<typeof InternalMailHistoryScope>[0]) {
  const [value, setValue] = useState(args.value);
  return (
    <InternalMailHistoryScope {...args} value={value} onChange={setValue} />
  );
}
export const NotAuthorized: Story = {
  args: { value: undefined, disabled: false, onChange: () => {} },
  render: (args) => <Editable {...args} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByRole("checkbox"));
    await expect(canvas.getByRole("checkbox")).toBeChecked();
    await expect(
      canvas.getByLabelText("Historical mail received on or after (UTC)"),
    ).toBeVisible();
  },
};
export const Authorized: Story = {
  ...NotAuthorized,
  play: undefined,
  args: { ...NotAuthorized.args, value: "2026-01-01" },
};
export const ReadOnly: Story = {
  ...Authorized,
  args: { ...Authorized.args, disabled: true },
};
export const AuthorizedDark: Story = {
  ...Authorized,
  globals: { theme: "dark" },
};
