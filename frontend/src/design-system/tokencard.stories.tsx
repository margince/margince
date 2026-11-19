// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { PeekCard } from "./peek";
import { tokenCard } from "./tokencard";

const meta: Meta = {
  title: "Components/Overlays and layering/Token card",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

export const ContactWithDetails: Story = {
  render: () => (
    <PeekCard
      {...tokenCard("alice@demo.test", "Alice Demo", {
        identity: "alice@demo.test",
        href: "#/contacts/alice",
        about: "Head of Sales, Demo GmbH",
      })}
    />
  ),
};

export const UnknownAddress: Story = {
  render: () => (
    <PeekCard {...tokenCard("bob@demo.test", "bob@demo.test", undefined)} />
  ),
};
