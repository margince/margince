// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { fn } from "storybook/test";
import { PORTRAIT } from "../testing/seatphotos";
import { Avatar } from "./atoms";
import { ImageSlot } from "./imageslot";

const WORDS = { add: "Add", replace: "Replace", remove: "Remove" };

const wall: CSSProperties = {
  display: "flex",
  flexWrap: "wrap",
  gap: "var(--space-5)",
};

const meta = {
  title: "Components/Forms and input/Image slot",
  component: ImageSlot,
  parameters: { layout: "padded" },
  args: {
    shape: "round",
    filled: false,
    preview: <Avatar identity="seat-1" name="Lena Hoffmann" size="xl" />,
    editable: true,
    name: "Profile picture",
    emptyLabel: "Drop picture here, or choose a file",
    accept: "image/png,image/jpeg,image/webp",
    words: WORDS,
    uploading: false,
    removing: false,
    onPick: fn(),
    onRemove: fn(),
  },
} satisfies Meta<typeof ImageSlot>;
export default meta;

type Story = StoryObj<typeof meta>;

// Nothing stored: the monogram stands in, and only Add is offered.
export const Empty: Story = {};

export const Filled: Story = {
  args: {
    filled: true,
    preview: (
      <Avatar
        identity="seat-1"
        name="Lena Hoffmann"
        photo={PORTRAIT}
        size="xl"
      />
    ),
  },
};

// The upload is out: the tile takes nothing, and the verb that sent it waits.
export const Uploading: Story = {
  args: { ...Filled.args, uploading: true },
};

// A reader who may not change it sees the picture and no verbs.
export const ReadOnly: Story = {
  args: { ...Filled.args, editable: false },
};

// Each tile is cut to the shape of what it holds.
export const Shapes: Story = {
  render: (args) => (
    <div style={wall}>
      <ImageSlot {...args} shape="wide" name="Wide logo" />
      <ImageSlot {...args} shape="square" name="Square icon" />
      <ImageSlot {...args} shape="round" name="Profile picture" />
    </div>
  ),
};

export const ShapesDark: Story = {
  ...Shapes,
  globals: { theme: "dark" },
};
