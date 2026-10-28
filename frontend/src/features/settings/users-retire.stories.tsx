// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { UTC_ZONE } from "../../format/timezone";
import { meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import type { User } from "./users-members";
import { RetireMember } from "./users-retire";

const MEMBER: User = {
  id: "10000000-0000-4000-8000-000000000001",
  display_name: "Robin Example",
  email: "robin@example.test",
  timezone: UTC_ZONE,
  status: "deactivated",
  is_agent: false,
};
const SURVIVOR: User = {
  ...MEMBER,
  id: "10000000-0000-4000-8000-000000000002",
  email: "robin.new@example.test",
  status: "invited",
};
function renderRetirement(merge: boolean, blocked: boolean) {
  installFetchStub({
    "GET /me": () =>
      jsonResponse(
        meFixture({
          roles: ["admin"],
          allow: { user_admin: ["read", "delete"] },
        }),
      ),
    [`GET /users/${MEMBER.id}/retirement-preview`]: () =>
      jsonResponse({
        source_name: MEMBER.display_name,
        source_email: MEMBER.email,
        owned: {
          contact: 2,
          company: 0,
          lead: 0,
          deal: 1,
          project: 0,
          task: 3,
        },
        signal_count: 1,
        blockers: blocked ? ["automations"] : [],
        consequences: merge
          ? ["source_signed_out", "survivor_access", "survivor_inactive"]
          : [
              "crm_private_records_move",
              "private_audiences_retained",
              "email_reserved",
            ],
        token: "reviewed-preview",
      }),
  });
  return (
    <StoryProviders>
      <RetireMember
        member={MEMBER}
        members={[MEMBER, SURVIVOR]}
        merge={merge}
        onClose={() => undefined}
      />
    </StoryProviders>
  );
}
const meta = {
  title: "Settings/People/Members/Retire member",
  component: RetireMember,
  args: {
    member: MEMBER,
    members: [MEMBER, SURVIVOR],
    merge: true,
    onClose: () => undefined,
  },
} satisfies Meta<typeof RetireMember>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Merge: Story = { render: () => renderRetirement(true, false) };
export const Remove: Story = { render: () => renderRetirement(false, false) };
export const Blocked: Story = { render: () => renderRetirement(true, true) };
