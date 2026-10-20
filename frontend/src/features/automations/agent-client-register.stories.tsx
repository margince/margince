// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import {
  AgentClientValuesDialog,
  RegisterAgentClientDialog,
} from "./agent-client-register";
import { AGENT_CLIENT_SCOPES, GEMINI_CLIENT } from "./agent-clients.fixtures";

// Registering a client by hand, then what to paste into the tool. The secret
// shows once, in the result, and nowhere after the dialog closes.

const meta: Meta<typeof RegisterAgentClientDialog> = {
  title: "Settings/AI/MCP/Register a client",
  component: RegisterAgentClientDialog,
};
export default meta;
type Story = StoryObj<typeof RegisterAgentClientDialog>;

function stub(connector: "on" | "off" = "on") {
  installFetchStub({
    "GET /me": meRoute({ agent_client: ["create", "read"] }),
    "GET /.well-known/oauth-protected-resource": () =>
      connector === "on"
        ? jsonResponse({ resource: "https://crm.example.com/mcp" })
        : jsonResponse({}, 404),
  });
}

/** The form: a name, the callbacks, and whether the tool needs a secret. */
export const Form: Story = {
  render: () => {
    stub();
    return (
      <StoryProviders>
        <RegisterAgentClientDialog onClose={() => undefined} />
      </StoryProviders>
    );
  },
};

/** The result for a tool that needs a secret: the warning, every value to
 * paste, and the Gemini Enterprise steps. */
export const ResultWithSecret: Story = {
  render: () => {
    stub();
    return (
      <StoryProviders>
        <AgentClientValuesDialog
          titleKey="agentClients.result.title"
          clientId={GEMINI_CLIENT.client_id}
          secret="mcs_4f0c9a1e7b2d48a6b3c5d7e9f1a2b3c4"
          scopes={AGENT_CLIENT_SCOPES}
          onClose={() => undefined}
        />
      </StoryProviders>
    );
  },
};

/** A public client: no secret, so no warning and no Gemini steps. */
export const ResultWithoutSecret: Story = {
  render: () => {
    stub();
    return (
      <StoryProviders>
        <AgentClientValuesDialog
          titleKey="agentClients.result.title"
          clientId="mc_claude_own"
          scopes={AGENT_CLIENT_SCOPES}
          onClose={() => undefined}
        />
      </StoryProviders>
    );
  },
};

/** The connector is off, so there is no address to derive the URLs from. */
export const ResultConnectorOff: Story = {
  render: () => {
    stub("off");
    return (
      <StoryProviders>
        <AgentClientValuesDialog
          titleKey="agentClients.result.title"
          clientId={GEMINI_CLIENT.client_id}
          secret="mcs_4f0c9a1e7b2d48a6b3c5d7e9f1a2b3c4"
          scopes={AGENT_CLIENT_SCOPES}
          onClose={() => undefined}
        />
      </StoryProviders>
    );
  },
};

/** In dark, because the warning and the code blocks are derived values. */
export const ResultWithSecretDark: Story = {
  globals: { theme: "dark" },
  render: ResultWithSecret.render,
};
