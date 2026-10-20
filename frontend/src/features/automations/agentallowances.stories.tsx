// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { defaultAgentAllowances } from "../../testing/agentallowances.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { AgentAllowancesCard } from "./agentallowances";

// What one passport may do per window. The stories cover rest, the edit form, a
// reader and a window change waiting for its reset.
function story(
  allow: Parameters<typeof meRoute>[0],
  allowances: object = defaultAgentAllowances,
) {
  return () => {
    let agent_allowances = { ...allowances };
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /installation/settings": () => jsonResponse({ agent_allowances }),
      "PATCH /installation/settings": (body) => {
        agent_allowances = { ...agent_allowances, ...(body as object) };
        return jsonResponse({ agent_allowances });
      },
    });
    return (
      <StoryProviders>
        <AgentAllowancesCard />
      </StoryProviders>
    );
  };
}

const MANAGER = { installation_settings: ["read", "update"] } as const;
const READER = { installation_settings: ["read"] } as const;

const meta: Meta<typeof AgentAllowancesCard> = {
  title: "Settings/AI/AI usage/Agent passport allowances",
  component: AgentAllowancesCard,
};
export default meta;
type Story = StoryObj<typeof AgentAllowancesCard>;

export const Allowances: Story = { render: story(MANAGER) };

export const EditingAllowances: Story = {
  render: story(MANAGER),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Edit allowances" }),
    );
    const calls = await canvas.findByTestId("allowance-agent_calls_per_window");
    await userEvent.clear(calls);
    await userEvent.type(calls, "500");
    await expect(
      await canvas.findByText(/Calls in total 1,000 → 500/),
    ).toBeVisible();
  },
};

export const CannotChange: Story = { render: story(READER) };

export const WindowChangeWaitingDark: Story = {
  globals: { theme: "dark" },
  render: story(MANAGER, {
    ...defaultAgentAllowances,
    agent_window_next_hours: 12,
    agent_window_next_starts_at: "2026-10-15T00:00:00Z",
  }),
};

export const AllowancesAtPhoneWidth: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(MANAGER),
};
