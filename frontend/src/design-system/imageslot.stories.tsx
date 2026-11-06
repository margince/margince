// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { expect, fn, userEvent, within } from "storybook/test";
import { PORTRAIT } from "../testing/seatphotos";
import { Avatar } from "./atoms";
import { ImageSlot } from "./imageslot";

const VERBS = {
  edit: "Edit",
  editLabel: "Edit profile picture",
  upload: "Upload picture",
  remove: "Remove picture",
};

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
    verbs: VERBS,
    uploading: false,
    removing: false,
    onPick: fn(),
    onRemove: fn(),
  },
} satisfies Meta<typeof ImageSlot>;
export default meta;

type Story = StoryObj<typeof meta>;

// Nothing stored: the monogram stands in, and the menu offers only Upload.
export const Empty: Story = {};

async function openEdit({ canvasElement }: { canvasElement: HTMLElement }) {
  await userEvent.click(
    within(canvasElement).getByRole("button", { name: "Edit profile picture" }),
  );
}

export const EmptyMenuOpen: Story = {
  play: async (context) => {
    await openEdit(context);
    const page = within(context.canvasElement.ownerDocument.body);
    await expect(
      page.getByRole("button", { name: "Upload picture" }),
    ).toBeVisible();
    await expect(
      page.queryByRole("button", { name: "Remove picture" }),
    ).toBeNull();
  },
};

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

export const FilledMenuOpen: Story = {
  args: { ...Filled.args },
  play: async (context) => {
    await openEdit(context);
    await within(context.canvasElement.ownerDocument.body).findByRole(
      "button",
      {
        name: "Remove picture",
      },
    );
  },
};

export const FilledMenuOpenDark: Story = {
  ...FilledMenuOpen,
  globals: { theme: "dark" },
};

export const FilledMenuOpenPhone: Story = {
  ...FilledMenuOpen,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// The upload is out: nothing takes a file, and the Edit pill shows the wait.
export const Uploading: Story = {
  args: { ...Filled.args, uploading: true },
};

// A reader who may not change it sees the picture and no pill.
export const ReadOnly: Story = {
  args: { ...Filled.args, editable: false },
};

// Each tile is cut to the shape of what it holds.
export const Shapes: Story = {
  render: (args) => (
    <div style={wall}>
      <ImageSlot
        {...args}
        shape="wide"
        name="Wide logo"
        verbs={{ ...VERBS, editLabel: "Edit wide logo" }}
      />
      <ImageSlot
        {...args}
        shape="square"
        name="Square icon"
        verbs={{ ...VERBS, editLabel: "Edit square icon" }}
      />
      <ImageSlot {...args} shape="round" name="Profile picture" />
    </div>
  ),
};

export const ShapesDark: Story = {
  ...Shapes,
  globals: { theme: "dark" },
};

export const ShapesPhone: Story = {
  ...Shapes,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
