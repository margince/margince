/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { Avatar } from "./atoms";
import { ImageSlot } from "./imageslot";

afterEach(cleanup);

const props = (
  over: Partial<ComponentProps<typeof ImageSlot>> = {},
): ComponentProps<typeof ImageSlot> => ({
  shape: "round",
  filled: true,
  preview: <Avatar identity="seat-1" name="Lena Hoffmann" />,
  editable: true,
  name: "Profile picture",
  emptyLabel: "Drop picture here, or choose a file",
  accept: "image/png",
  verbs: {
    edit: "Edit",
    editLabel: "Edit profile picture",
    upload: "Upload picture",
    remove: "Remove picture",
  },
  uploading: false,
  removing: false,
  onPick: vi.fn(),
  onRemove: vi.fn(),
  ...over,
});

it.each([
  { flight: "an upload", over: { uploading: true } },
  { flight: "a removal", over: { removing: true } },
])(
  "takes no second write from its open menu while $flight is out",
  async ({ over }) => {
    const user = userEvent.setup();
    const onRemove = vi.fn();
    const { rerender } = render(<ImageSlot {...props({ onRemove })} />);
    await user.click(
      screen.getByRole("button", { name: "Edit profile picture" }),
    );

    rerender(<ImageSlot {...props({ onRemove, ...over })} />);

    expect(
      screen.getByRole("button", { name: "Upload picture" }),
    ).toBeDisabled();
    const remove = screen.getByRole("button", { name: "Remove picture" });
    expect(remove).toBeDisabled();
    await user.click(remove);
    expect(onRemove).not.toHaveBeenCalled();
  },
);
