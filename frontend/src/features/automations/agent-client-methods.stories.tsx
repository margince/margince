// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import {
  agentClientList,
  BOTH_METHODS_ON,
} from "../../testing/agent-clients.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { AgentClientMethodsCard } from "./agent-client-methods";

// The two ways a tool gets a client without an admin, and the one that is
// always open. The deployment file can hold either way off.

const meta: Meta<typeof AgentClientMethodsCard> = {
  title: "Settings/AI/MCP/How MCP clients may connect",
  component: AgentClientMethodsCard,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof AgentClientMethodsCard>;

function served(
  registration: components["schemas"]["AgentClientRegistration"],
  actions: readonly ("read" | "update")[] = ["read", "update"],
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({ agent_client: actions }),
      "GET /installation/agent-clients": () =>
        jsonResponse(agentClientList([], registration)),
    });
    return (
      <StoryProviders>
        <AgentClientMethodsCard />
      </StoryProviders>
    );
  };
}

/** Both ways open, as an installation starts. */
export const BothOn: Story = { render: served(BOTH_METHODS_ON) };

/** The admin closed self-registration; published identities stay open. */
export const SelfRegistrationOff: Story = {
  render: served({
    ...BOTH_METHODS_ON,
    dynamic: { enabled: false, setting: false, deployment_allows: true },
  }),
};

/** The deployment file holds self-registration off: the switch shows off and
 * refuses, whatever the admin's own setting says. */
export const HeldOffByDeployment: Story = {
  render: served({
    ...BOTH_METHODS_ON,
    dynamic: { enabled: false, setting: true, deployment_allows: false },
  }),
};

/** A management seat reads the switches and cannot turn them. */
export const ReadOnly: Story = { render: served(BOTH_METHODS_ON, ["read"]) };

/** In dark, because the hairlines between rows are derived values. */
export const BothOnDark: Story = {
  globals: { theme: "dark" },
  render: served(BOTH_METHODS_ON),
};
