// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { HandoffAssigneeModal } from "./handoffassignee";

function renderAssignment(canReassign = true) {
  installFetchStub({
    "GET /sdr-handoffs/example/assignee": () =>
      jsonResponse({
        id: "example",
        version: 3,
        status: canReassign ? "submitted" : "accepted",
        can_reassign: canReassign,
      }),
    "GET /users": () =>
      jsonResponse({
        page: { has_more: false, next_cursor: null },
        data: [
          {
            id: "colleague",
            display_name: "Dana Example",
            status: "active",
            is_agent: false,
            can_own_records: true,
          },
        ],
      }),
  });
  return (
    <StoryProviders>
      <HandoffAssigneeModal id="example" onClose={() => {}} />
    </StoryProviders>
  );
}

const meta: Meta<typeof HandoffAssigneeModal> = {
  title: "Records/Reports/Analytics/Handoff receiver",
  component: HandoffAssigneeModal,
};
export default meta;
type Story = StoryObj<typeof HandoffAssigneeModal>;
export const ChooseColleague: Story = { render: () => renderAssignment() };
export const ChooseColleagueDark: Story = {
  ...ChooseColleague,
  globals: { theme: "dark" },
};
export const AlreadyAnswered: Story = { render: () => renderAssignment(false) };
