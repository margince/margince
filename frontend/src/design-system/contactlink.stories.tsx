// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Mail, Phone } from "lucide-react";
import { WriteToProvider } from "../screens/writeto";
import { ContactLink } from "./contactlink";

// An address or a number the reader can act on. The stories document the one
// decision the component owns: a value in the shape its scheme admits becomes
// an action, and anything else stays as text with the fact intact.
//
// The shell's composer host is stood in for by a provider that opens nothing:
// the story is about the affordance, and without a host the address would be
// drawn as the reader's own mail client's link.
const meta: Meta<typeof ContactLink> = {
  title: "Design System/ContactLink",
  component: ContactLink,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <WriteToProvider writeTo={() => {}}>
        <Story />
      </WriteToProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof ContactLink>;

const dana = { entityType: "contact", entityId: "p-1" } as const;

export const Email: Story = {
  args: { kind: "email", value: "dana@brandt.example", record: dana },
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
      <ContactLink kind="email" value="dana@brandt.example" record={dana}>
        <Mail size={13} aria-hidden="true" /> dana@brandt.example
      </ContactLink>
      <ContactLink kind="phone" value="+33 6 12 44 08 91">
        <Phone size={13} aria-hidden="true" /> +33 6 12 44 08 91
      </ContactLink>
    </div>
  ),
};

export const RefusedValue: Story = {
  // An address carrying a second header never reaches the composer. The
  // reader still sees what was recorded.
  args: {
    kind: "email",
    value: "dana@brandt.example?subject=hi",
    record: dana,
  },
};

// The link's accent and the refused text on the dark ground, where the accent
// lifts and the two must still tell apart.
export const EmailDark: Story = {
  ...Email,
  globals: { theme: "dark" },
};

export const RefusedValueDark: Story = {
  ...RefusedValue,
  globals: { theme: "dark" },
};
