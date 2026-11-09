// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { PORTRAIT, WHITE_PORTRAIT } from "../../testing/seatphotos";
import { StoryProviders } from "../../testing/story-utils";
import { MemberIdentity } from "./users-membercells";
import type { User } from "./users-members";

const column: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "var(--space-4)",
};

function member(id: string, name: string, photo?: string): User {
  return {
    id,
    email: `${name.split(" ")[0]?.toLowerCase()}@demo.test`,
    display_name: name,
    avatar_url: photo,
    status: "active",
    is_agent: false,
  };
}

function identities() {
  return (
    <StoryProviders>
      <div style={column}>
        <MemberIdentity member={member("seat-1", "Lena Hoffmann", PORTRAIT)} />
        <MemberIdentity
          member={member("seat-2", "Jonas Becker", WHITE_PORTRAIT)}
        />
        <MemberIdentity member={member("seat-3", "Mia Wolf")} />
      </div>
    </StoryProviders>
  );
}

// A member's row face: their picture, a white one inside its ring, and the
// mesh and initials of a seat with none.
const meta: Meta<typeof MemberIdentity> = {
  title: "Settings/People/Members/Member identity",
  component: MemberIdentity,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof MemberIdentity>;

export const WithAndWithoutPictures: Story = { render: identities };

export const WithAndWithoutPicturesDark: Story = {
  globals: { theme: "dark" },
  render: identities,
};
