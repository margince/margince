// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { agentUsagePage } from "../../testing/agentallowances.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { AgentUsageByUserCard } from "./agentusage";

// Three users, each drawn against the allowance their own roles give: an
// Admin far inside the preset, a two-role user near a limit, and a User
// stopped at the installation value.
const PAGE = agentUsagePage;

function story(allow: Parameters<typeof meRoute>[0], page: object = PAGE) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /ai/agent-usage": () => jsonResponse(page),
    });
    return (
      <StoryProviders>
        <AgentUsageByUserCard />
      </StoryProviders>
    );
  };
}

const READER = { ai_diagnostics: ["read"] } as const;

const meta: Meta<typeof AgentUsageByUserCard> = {
  title: "Settings/AI/AI usage/Agent usage by user",
  component: AgentUsageByUserCard,
};
export default meta;
type Story = StoryObj<typeof AgentUsageByUserCard>;

export const Usage: Story = { render: story(READER) };

export const UsageDark: Story = {
  globals: { theme: "dark" },
  render: story(READER),
};

// The user whose two roles each raise one count: each value names its role.
export const OneUsersAllowance: Story = {
  render: story(READER),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByTestId("allowance-check-user"));
    await userEvent.click(
      await within(document.body).findByRole("option", {
        name: /Minh Tran/,
      }),
    );
    await expect(await canvas.findByText("From Team Lead")).toBeVisible();
    await expect(canvas.getByText("From Ops / Integrations")).toBeVisible();
  },
};

export const NoLivePassports: Story = {
  render: story(READER, { ...PAGE, data: [] }),
};

export const Withheld: Story = { render: story({}) };

export const UsageAtPhoneWidth: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(READER),
};
