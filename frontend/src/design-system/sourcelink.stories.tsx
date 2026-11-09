// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Mail } from "lucide-react";
import { SourceLink } from "./sourcelink";

const meta: Meta<typeof SourceLink> = {
  title: "Components/AI and provenance/Source link",
  component: SourceLink,
  args: {
    icon: <Mail aria-hidden="true" />,
    children: "Pricing follow-up",
    onOpen: () => {},
  },
};
export default meta;
type Story = StoryObj<typeof SourceLink>;

export const Openable: Story = {};

// A subject longer than the column wraps and still reads from the start edge.
export const LongName: Story = {
  render: (args) => (
    <div style={{ inlineSize: "20rem" }}>
      <SourceLink {...args}>
        Reminder: Margince Sneak Preview on 17 September - the AI-native CRM
        that works for sales
      </SourceLink>
    </div>
  ),
};

export const NowhereToOpen: Story = { args: { onOpen: undefined } };
