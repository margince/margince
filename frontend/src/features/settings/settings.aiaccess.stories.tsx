// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { within } from "storybook/test";
import { AGENT_RULES_SUBPAGE } from "../../lib/settingsrouting";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { AiAccessTab } from "./settings.aiaccess";

const meta: Meta<typeof AiAccessTab> = {
  title: "Settings/You/AI assistants/Personal review settings",
  component: AiAccessTab,
};
export default meta;
type Story = StoryObj<typeof AiAccessTab>;
function settings(subpage?: string) {
  installFetchStub({
    "GET /me": meRoute({}),
    "GET /.well-known/oauth-protected-resource": () =>
      jsonResponse({ resource: "https://margince.example/mcp" }),
    "GET /autonomy": () =>
      jsonResponse({
        data: [
          "close_date_correction",
          "company_name_promotion",
          "lifecycle_change",
        ].map((kind) => ({
          kind,
          mode: "manual",
          approved_clean: 0,
          approved_edited: 0,
          rejected: 0,
        })),
      }),
    "GET /me/approval-preferences": () =>
      jsonResponse({
        data: [
          { kind: "close_date_correction", muted: true },
          { kind: "lifecycle_change", muted: false },
        ],
      }),
  });
  return (
    <StoryProviders>
      <AiAccessTab subpage={subpage} />
    </StoryProviders>
  );
}
// Your agent stands open on the page, so the mutes need no section opened.
async function showMutes({ canvasElement }: { canvasElement: HTMLElement }) {
  const canvas = within(canvasElement);
  (await canvas.findByText("Personal approval mutes")).scrollIntoView();
}
export const PersonalReview: Story = {
  render: () => settings(),
  play: showMutes,
};
export const PersonalReviewDark: Story = {
  render: () => settings(),
  play: showMutes,
  globals: { theme: "dark" },
};
// Arrived on Agent rules' own address: For developers open on that tab.
export const AgentRulesAddress: Story = {
  render: () => settings(AGENT_RULES_SUBPAGE),
};
export const AgentRulesAddressPhone: Story = {
  render: () => settings(AGENT_RULES_SUBPAGE),
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
