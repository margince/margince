// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { AgentUsageByUserCard } from "./agentusage";

const counter = (used: number, limit: number) => ({ used, limit, released: 0 });

// Three users: one stopped on a used-up allowance, one near a limit, one
// working well inside them.
const PAGE = {
  resets_at: "2026-10-15T00:00:00Z",
  data: [
    {
      user_id: "u-1",
      display_name: "Anna Weber",
      passport_count: 1,
      writes: counter(44, 200),
      egress: counter(2, 20),
      calls: counter(120, 1_000),
      state: "ok",
    },
    {
      user_id: "u-2",
      display_name: "Minh Tran",
      passport_count: 2,
      writes: counter(400, 400),
      egress: counter(20, 40),
      calls: counter(980, 2_000),
      state: "stopped",
    },
    {
      user_id: "u-3",
      display_name: "Jonas Becker",
      passport_count: 3,
      writes: counter(186, 600),
      egress: counter(0, 60),
      calls: counter(890, 3_000),
      state: "near",
    },
  ],
};

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

export const NoLivePassports: Story = {
  render: story(READER, { ...PAGE, data: [] }),
};

export const Withheld: Story = { render: story({}) };

export const UsageAtPhoneWidth: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(READER),
};
