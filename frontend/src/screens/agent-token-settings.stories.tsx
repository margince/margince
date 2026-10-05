// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { AgentConnectionsCard } from "./agent-token-settings";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The passport lifetime, editable and refused. The refused seat holds the read
// and not the write, so it sees the number and the sentence saying why it is
// fixed for them.
function story(allow: Parameters<typeof meRoute>[0]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /installation/settings": () =>
        jsonResponse({ oauth_access_token_ttl_minutes: 43_200 }),
      "PATCH /installation/settings": (body) => jsonResponse(body),
    });
    return (
      <StoryProviders>
        <AgentConnectionsCard />
      </StoryProviders>
    );
  };
}

const MANAGER = { installation_settings: ["read", "update"] } as const;
const READER = { installation_settings: ["read"] } as const;

const meta: Meta<typeof AgentConnectionsCard> = {
  title: "Settings/Company/Sign-in and apps/Agent connections",
  component: AgentConnectionsCard,
};
export default meta;
type Story = StoryObj<typeof AgentConnectionsCard>;

export const Editable: Story = { render: story(MANAGER) };

export const CannotChange: Story = { render: story(READER) };

export const CannotChangeDark: Story = {
  globals: { theme: "dark" },
  render: story(READER),
};
