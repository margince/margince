// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { viewerZone } from "../format/timezone";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import type { User } from "./users-members";
import { MemberVerbs } from "./users-memberverbs";

function member(over: Partial<User>): User {
  return {
    id: "u-2",
    email: "dana@brandt.example",
    display_name: "Dana Kessler",
    timezone: viewerZone(),
    status: "active",
    is_agent: false,
    roles: ["rep"],
    ...over,
  };
}

function verbs(who: User) {
  return () => {
    installFetchStub({
      "POST /users/u-2/deactivate": () => jsonResponse({}),
      "POST /users/u-2/reactivate": () => jsonResponse({}),
    });
    return (
      <StoryProviders>
        <MemberVerbs member={who} />
      </StoryProviders>
    );
  };
}

const open = async ({ canvasElement }: { canvasElement: HTMLElement }) => {
  await userEvent.click(
    await within(canvasElement).findByRole("button", {
      name: "Actions for Dana Kessler",
    }),
  );
};

const meta: Meta<typeof MemberVerbs> = {
  title: "Settings/People/Members/Member actions",
  component: MemberVerbs,
};
export default meta;
type Story = StoryObj<typeof MemberVerbs>;

export const ActiveMember: Story = {
  render: verbs(
    member({ allowed_actions: ["issue_password_link", "deactivate"] }),
  ),
  play: open,
};

export const DeactivatedMember: Story = {
  render: verbs(
    member({ status: "deactivated", allowed_actions: ["reactivate"] }),
  ),
  play: open,
};

// Nothing offered draws no trigger at all.
export const NothingOffered: Story = {
  render: verbs(member({ allowed_actions: [] })),
};
