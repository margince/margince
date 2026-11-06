// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Badge } from "./atoms";
import { DocumentTile } from "./documenttile";

// A document as a tile: the kind on the stage, a mark in its far corner, and
// the title as the door. A stored file previews; a dialog door opens over the
// page; a lapsed file recedes.

const meta: Meta<typeof DocumentTile> = {
  title: "Components/Text and data display/Document tile",
  component: DocumentTile,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: "16rem" }}>
        <Story />
      </div>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof DocumentTile>;

export const StoredFile: Story = {
  args: {
    filename: "Retrofit-proposal-v2.pdf",
    title: "Retrofit-proposal-v2.pdf",
    anchor: {
      href: "/v1/attachments/a-1",
      download: "Retrofit-proposal-v2.pdf",
    },
    mark: <Badge>Current</Badge>,
    meta: "Proposal/Offer · 6 Nov 2026",
  },
};

export const Superseded: Story = {
  args: {
    ...StoredFile.args,
    lapsed: true,
    mark: <Badge tone="warning">Superseded</Badge>,
  },
};
