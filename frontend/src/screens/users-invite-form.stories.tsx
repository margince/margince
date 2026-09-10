// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";
import { InviteUserForm } from "./users-invite-form";

// An address, a name, a role and the teams the member lands in, committed
// together. TWO surfaces ask it — the roster's dialog in Settings → People and
// the setup journey's team step — and it is one form so the two cannot come to
// invite differently, which is the difference the `askName` story shows.

const meta: Meta<typeof InviteUserForm> = {
  title: "Settings/People/Members/Invite",
  component: InviteUserForm,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof InviteUserForm>;

function Served({ children }: Readonly<{ children: ReactNode }>) {
  installFetchStub({
    "GET /me": meRoute({
      user_admin: ["read", "create"],
      team_admin: ["read"],
    }),
    "GET /teams": () =>
      jsonResponse({
        data: [
          { id: "t-1", name: "Field sales" },
          { id: "t-2", name: "Inside sales" },
        ],
        page: { has_more: false, next_cursor: null },
      }),
    "GET /users/access-preview": () =>
      jsonResponse({
        objects: {
          person: { create: true, read: true, update: true, delete: false },
          deal: { create: true, read: true, update: true, delete: false },
        },
        row_scope: "own",
        seat_type: "full",
      }),
  });
  return <StoryProviders>{children}</StoryProviders>;
}

/** The roster's dialog, which asks for the name as well as the address. */
export const WithName: Story = {
  render: () => (
    <Served>
      <InviteUserForm onInvited={() => undefined} />
    </Served>
  ),
};

/**
 * The setup journey's step. One address is enough to get a first person in,
 * and the name is derived from it until somebody changes it.
 */
export const AddressOnly: Story = {
  render: () => (
    <Served>
      <InviteUserForm askName={false} onInvited={() => undefined} />
    </Served>
  ),
};
