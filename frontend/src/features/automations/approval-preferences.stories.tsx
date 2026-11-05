// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ApprovalPreferencesCard } from "./approval-preferences";

const meta: Meta<typeof ApprovalPreferencesCard> = {
  title: "Settings/You/AI access/Personal approval mutes",
  component: ApprovalPreferencesCard,
};
export default meta;
type Story = StoryObj<typeof ApprovalPreferencesCard>;
function preferences() {
  installFetchStub({
    "GET /me": meRoute({}),
    "GET /me/approval-preferences": () =>
      jsonResponse({
        data: [
          { kind: "close_date_correction", muted: true },
          { kind: "lifecycle_change", muted: false },
          { kind: "deal_follow_up", muted: false },
        ],
      }),
  });
  return (
    <StoryProviders>
      <ApprovalPreferencesCard />
    </StoryProviders>
  );
}
export const PersonalChoices: Story = { render: preferences };
export const PersonalChoicesDark: Story = {
  render: preferences,
  globals: { theme: "dark" },
};
