// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { MessageChannelField } from "./logactivity.channel";

const meta: Meta<typeof MessageChannelField> = {
  title: "Patterns/Message channel",
  component: MessageChannelField,
};
export default meta;
type Story = StoryObj<typeof MessageChannelField>;

function Channel() {
  const [value, onChange] = useState("");
  return (
    <MessageChannelField
      active
      options={[
        { value: "whatsapp", label: "WhatsApp" },
        { value: "linkedin", label: "LinkedIn" },
      ]}
      value={value}
      onChange={onChange}
    />
  );
}
export const Choose: Story = { render: () => <Channel /> };
export const ChooseDark: Story = { ...Choose, globals: { theme: "dark" } };
