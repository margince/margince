// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  agentClientList,
  GEMINI_CLIENT,
  PUBLISHED_CLIENT,
  SELF_REGISTERED_CLIENT,
} from "../../testing/agent-clients.fixtures";
import type { GrantSpec } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { AgentClientsCard } from "./agent-clients";
import type { AgentClient } from "./agent-clients.queries";

// Every client an AI tool signs in through, one of each way it came to exist.

const meta: Meta<typeof AgentClientsCard> = {
  title: "Settings/AI/MCP/MCP clients",
  component: AgentClientsCard,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof AgentClientsCard>;

const ADMIN: GrantSpec = {
  agent_client: ["create", "read", "update", "delete"],
};
const EVERY_KIND = [GEMINI_CLIENT, SELF_REGISTERED_CLIENT, PUBLISHED_CLIENT];

function served(
  clients: readonly AgentClient[],
  allow: GrantSpec = ADMIN,
  nextCursor: string | null = null,
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /installation/agent-clients": () =>
        jsonResponse(agentClientList(clients, undefined, nextCursor)),
      "GET /.well-known/oauth-protected-resource": () =>
        jsonResponse({ resource: "https://crm.example.com/mcp" }),
    });
    return (
      <StoryProviders>
        <AgentClientsCard />
      </StoryProviders>
    );
  };
}

/** One client of each kind, one of them disabled. */
export const EveryKind: Story = { render: served(EVERY_KIND) };

/** No client yet: the register verb is the way in. */
export const Empty: Story = { render: served([]) };

/** More clients than one page: the list reads on when asked. */
export const Paged: Story = {
  render: served([GEMINI_CLIENT, PUBLISHED_CLIENT], ADMIN, "page-2"),
};

/** An ops seat reads the list and changes nothing, and the card says so. */
export const ReadOnly: Story = {
  render: served(EVERY_KIND, { agent_client: ["read"] }),
};

/** In dark, because the badges and hairlines are derived values. */
export const EveryKindDark: Story = {
  globals: { theme: "dark" },
  render: served(EVERY_KIND),
};
