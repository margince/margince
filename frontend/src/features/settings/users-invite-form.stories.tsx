// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { SEEDED_ASSIGNABLE_ROLES } from "../../testing/roles.testkit";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { InviteUserForm } from "./users-invite-form";

// An address, a name, a role, the teams the member lands in and the legal
// entity they work for, committed together. TWO surfaces ask it: the roster's
// dialog in Settings → Contacts and the setup journey's team step. It is one
// form so the two cannot come to invite differently; the `askName` story shows
// the one difference.

const meta: Meta<typeof InviteUserForm> = {
  title: "Settings/People/Members/Invite",
  component: InviteUserForm,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof InviteUserForm>;

function Served({
  children,
  entities = false,
}: Readonly<{ children: ReactNode; entities?: boolean }>) {
  installFetchStub({
    "GET /me": meRoute({
      user_admin: entities ? ["read", "create", "update"] : ["read", "create"],
      team_admin: ["read"],
      ...(entities ? { legal_entity: ["read", "update"] as const } : {}),
    }),
    "GET /legal-entities": () =>
      jsonResponse({
        data: [
          { id: "le-default", is_default: true, display_name: "Acme" },
          { id: "le-vienna", is_default: false, display_name: "Acme Wien" },
        ],
      }),
    "GET /teams": () =>
      jsonResponse({
        data: [
          { id: "t-1", name: "Field sales" },
          { id: "t-2", name: "Inside sales" },
        ],
        page: { has_more: false, next_cursor: null },
      }),
    "GET /users/assignable-roles": () =>
      jsonResponse({ roles: SEEDED_ASSIGNABLE_ROLES }),
    "GET /users/access-preview": () =>
      jsonResponse({
        objects: {
          contact: { create: true, read: true, update: true, delete: false },
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
 * The setup journey's step. One address is enough to get a first contact in,
 * and the name is derived from it until somebody changes it.
 */
export const AddressOnly: Story = {
  render: () => (
    <Served>
      <InviteUserForm askName={false} onInvited={() => undefined} />
    </Served>
  ),
};

/**
 * An installation that trades as two legal entities, and an inviter who may
 * assign members. The form asks which one the member works for, and starts on
 * the default.
 */
export const WithLegalEntity: Story = {
  render: () => (
    <Served entities>
      <InviteUserForm onInvited={() => undefined} />
    </Served>
  ),
};
