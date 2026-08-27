// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { OffsiteLink } from "./offsitelink";

// A destination off our origin. What these stories document is the decision the
// component owns and a call site must not repeat: an address the product may
// follow becomes a link, and one it may not stays as text with the fact intact.
const meta: Meta<typeof OffsiteLink> = {
  title: "Design System/OffsiteLink",
  component: OffsiteLink,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof OffsiteLink>;

export const Link: Story = {
  // The ordinary case: an https address, drawn as the secondary text
  // affordance, opening in a new tab with the opener handle withheld.
  args: {
    href: "https://linkedin.com/in/jdoe",
    children: "linkedin.com/in/jdoe",
  },
};

export const RefusedAddress: Story = {
  // A `javascript:` href is script execution on click, so it never becomes an
  // anchor. The reader still sees the value — losing the link is the whole
  // penalty, and hiding the fact would be a second one.
  args: {
    href: "javascript:alert(1)",
    children: "javascript:alert(1)",
  },
};

export const NotAnAddress: Story = {
  // What a half-typed field holds. Same treatment as a refused scheme: text.
  args: { href: "jdoe", children: "jdoe" },
};
