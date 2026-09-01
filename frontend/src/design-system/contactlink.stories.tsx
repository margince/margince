// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Mail, Phone } from "lucide-react";
import { ContactLink } from "./contactlink";

// An address or a number the reader can act on. The stories document the one
// decision the component owns: a value in the shape its scheme admits becomes a
// link, and anything else stays as text with the fact intact.
const meta: Meta<typeof ContactLink> = {
  title: "Design System/ContactLink",
  component: ContactLink,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof ContactLink>;

export const Email: Story = {
  args: { kind: "email", value: "dana@brandt.example" },
};

export const Phone_: Story = {
  name: "Phone",
  args: { kind: "phone", value: "+33 6 12 44 08 91" },
};

export const WithIcon: Story = {
  // The header's identity line leads each fact with its glyph; the link takes
  // the icon as its children rather than drawing one of its own.
  render: () => (
    <div style={{ display: "flex", gap: "var(--space-4)" }}>
      <ContactLink kind="email" value="dana@brandt.example">
        <Mail size={13} aria-hidden="true" /> dana@brandt.example
      </ContactLink>
      <ContactLink kind="phone" value="+33 6 12 44 08 91">
        <Phone size={13} aria-hidden="true" /> +33 6 12 44 08 91
      </ContactLink>
    </div>
  ),
};

export const RefusedValue: Story = {
  // An address carrying a second header never reaches the mail client as a
  // link. The reader still sees what was recorded.
  args: { kind: "email", value: "dana@brandt.example?subject=hi" },
};
