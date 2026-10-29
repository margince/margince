// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { AppLink } from "./applink";

const meta: Meta<typeof AppLink> = {
  title: "Components/Navigation/App link",
  component: AppLink,
  args: {
    children: "Example renewal",
    href: "#/deals/01a00000-0000-7000-8000-000000000001",
  },
};
export default meta;
type Story = StoryObj<typeof AppLink>;
export const Record: Story = {};
export const NewTab: Story = {
  args: { target: "_blank", rel: "noopener noreferrer" },
};
export const Home: Story = { args: { children: "Home", href: "#/home" } };

export const WithoutDestination: Story = {
  args: { href: undefined, children: "Unavailable record" },
};
