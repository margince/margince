// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { AiAccessTab } from "./settings.aiaccess";

const meta: Meta<typeof AiAccessTab> = {
  title: "Settings/You/AI access/Personal review settings",
  component: AiAccessTab,
};
export default meta;
type Story = StoryObj<typeof AiAccessTab>;
function settings() {
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
      <AiAccessTab />
    </StoryProviders>
  );
}
async function showRules({ canvasElement }: { canvasElement: HTMLElement }) {
  const canvas = within(canvasElement);
  await userEvent.click(await canvas.findByText("For developers"));
  await userEvent.click(
    await canvas.findByRole("button", { name: "Agent rules" }),
  );
}
export const PersonalReview: Story = { render: settings, play: showRules };
export const PersonalReviewDark: Story = {
  render: settings,
  play: showRules,
  globals: { theme: "dark" },
};
