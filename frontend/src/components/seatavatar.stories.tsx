// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { PORTRAIT } from "../testing/seatphotos";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { SeatAvatar } from "./seatavatar";

const WITH_PHOTO = "00000000-0000-4000-8000-0000000000a1";
const WITHOUT_PHOTO = "00000000-0000-4000-8000-0000000000a2";

const wall: CSSProperties = {
  display: "flex",
  gap: "var(--space-3)",
  alignItems: "center",
};

function seats() {
  installFetchStub({
    "GET /users/names": () =>
      jsonResponse({
        data: [
          {
            id: WITH_PHOTO,
            display_name: "Lena Hoffmann",
            avatar_url: PORTRAIT,
            departed: false,
          },
          { id: WITHOUT_PHOTO, display_name: "Jonas Becker", departed: false },
        ],
      }),
  });
  return (
    <StoryProviders>
      <div style={wall}>
        <SeatAvatar id={WITH_PHOTO} name="Lena Hoffmann" size="md" />
        <SeatAvatar id={WITHOUT_PHOTO} name="Jonas Becker" size="md" />
        <SeatAvatar
          id="00000000-0000-4000-8000-0000000000a3"
          name="Mia Wolf"
          photo={PORTRAIT}
          size="md"
        />
      </div>
    </StoryProviders>
  );
}

// A colleague named by id finds their picture through the names read; one
// without a picture keeps their initials; a held seat passes its own.
const meta: Meta<typeof SeatAvatar> = {
  title: "Patterns/Seat avatar",
  component: SeatAvatar,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof SeatAvatar>;

export const ByIdAndHeld: Story = { render: seats };

export const ByIdAndHeldDark: Story = {
  globals: { theme: "dark" },
  render: seats,
};
