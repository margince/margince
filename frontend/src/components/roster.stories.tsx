// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryStates } from "../design-system/querystates";
import { UTC_ZONE } from "../format/timezone";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { useRoster } from "./roster";

function RosterRead() {
  const roster = useRoster("user", true);
  return (
    <QueryStates query={roster} pendingLabel="Members">
      <ul>
        {roster.data?.map((member) => (
          <li key={member.id}>
            {"display_name" in member ? member.display_name : member.name}
          </li>
        ))}
      </ul>
    </QueryStates>
  );
}
const meta = {
  title: "Settings/People/Members/Member roster",
  component: RosterRead,
} satisfies Meta<typeof RosterRead>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Members: Story = {
  render: () => {
    installFetchStub({
      "GET /users": () =>
        jsonResponse({
          data: [
            {
              id: "demo-member",
              display_name: "Robin Example",
              email: "robin@example.test",
              timezone: UTC_ZONE,
              status: "active",
              is_agent: false,
            },
          ],
          page: { has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <RosterRead />
      </StoryProviders>
    );
  },
};
