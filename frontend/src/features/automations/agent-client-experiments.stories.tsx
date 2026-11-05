// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { agentClientList } from "../../testing/agent-clients.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { AgentClientExperimentsCard } from "./agent-client-experiments";

const meta: Meta<typeof AgentClientExperimentsCard> = {
  title: "Settings/AI/MCP/Experiments",
  component: AgentClientExperimentsCard,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof AgentClientExperimentsCard>;

function served(on: boolean) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({ agent_client: ["read", "update"] }),
      "GET /installation/agent-clients": () =>
        jsonResponse({ ...agentClientList([]), client_check: on }),
    });
    return (
      <StoryProviders>
        <AgentClientExperimentsCard />
      </StoryProviders>
    );
  };
}

/** The check is off, as an installation starts. */
export const Off: Story = { render: served(false) };

/** An admin turned the check on. */
export const On: Story = { render: served(true) };

/** The check on, in the dark theme. */
export const OnDark: Story = { ...On, globals: { theme: "dark" } };
