// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Avatar } from "./atoms";
import { AvatarStack } from "./avatarstack";

// A committee of contacts as overlapping monograms, folding into a "+N" once
// the group runs past `max`.
const meta = {
  title: "Components/Images and icons/Avatar stack",
  component: AvatarStack,
  parameters: { layout: "padded" },
} satisfies Meta<typeof AvatarStack>;
export default meta;

// Typed off `meta`, so every face must carry its required `identity`.
type Story = StoryObj<typeof meta>;

export const FewContacts: Story = {
  args: {
    contacts: [
      { name: "Alex Rivera", identity: "Alex Rivera" },
      { name: "Sam Okafor", identity: "Sam Okafor" },
    ],
  },
};

export const OverTheMax: Story = {
  args: {
    contacts: [
      { name: "Alex Rivera", identity: "Alex Rivera" },
      { name: "Sam Okafor", identity: "Sam Okafor" },
      { name: "Priya Nair", identity: "Priya Nair" },
      { name: "Jordan Blake", identity: "Jordan Blake" },
      { name: "Casey Lund", identity: "Casey Lund" },
      { name: "Mira Vance", identity: "Mira Vance" },
      { name: "Theo Marsh", identity: "Theo Marsh" },
    ],
    max: 5,
  },
};

// The ring costs the chip no size, so a folded face sits on the line a lone
// one does.
export const BesideALoneAvatar: StoryObj = {
  render: () => (
    <div style={{ display: "flex", gap: "0.75rem", alignItems: "center" }}>
      <AvatarStack
        contacts={[
          { name: "Alice Müller", identity: "Alice Müller" },
          { name: "Bob Schmidt", identity: "Bob Schmidt" },
          { name: "Carol Wagner", identity: "Carol Wagner" },
          { name: "Dara O'Brien", identity: "Dara O'Brien" },
          { name: "Eve Lindqvist", identity: "Eve Lindqvist" },
          { name: "Frank Osei", identity: "Frank Osei" },
        ]}
      />
      <Avatar name="Alice Müller" identity="Alice Müller" />
    </div>
  ),
};
