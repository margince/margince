// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { ConnectedAgentsCard } from "./connected-agents";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The OAuth clients holding a credential of their own. The connect guide opens
// itself only while nothing is connected, so `Connected` is the closed case.
const CLAUDE = {
  id: "pp-1",
  label: "Claude Desktop",
  revoked_at: null,
  expires_at: "2026-12-01T00:00:00Z",
  scopes: ["read", "draft"],
  connection: {
    client_name: "Claude Desktop",
    connected_at: "2026-07-30T14:10:00Z",
    renewable: true,
    lent_passport_label: null,
  },
};

// Expired and not renewable: the client has to be reconnected from its own end,
// which is a different sentence from "this lapsed and will come back".
const LAPSED = {
  id: "pp-2",
  label: "Scout",
  revoked_at: null,
  expires_at: "2026-01-04T00:00:00Z",
  scopes: ["read"],
  connection: {
    client_name: "Scout",
    connected_at: "2025-12-01T09:00:00Z",
    renewable: false,
    lent_passport_label: "Ops runner",
  },
};

// The connect guide asks the OAuth discovery document whether this installation
// serves the governed tool surface at all — a well-known path, not a /v1 one, so
// it needs routing like any other. Left unrouted it fell through to the stub's
// empty-list fallback, which carries no `resource`, and the guide rendered its
// own failure state under a story named for a working card.
function story(passports: Record<string, unknown>[], connectorEnabled = true) {
  return () => {
    globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /passports": () => jsonResponse({ data: passports }),
      "GET /.well-known/oauth-protected-resource": () =>
        connectorEnabled
          ? jsonResponse({ resource: "https://margince.example/mcp" })
          : jsonResponse({ code: "not_found" }, 404),
    });
    return (
      <StoryProviders>
        <ConnectedAgentsCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof ConnectedAgentsCard> = {
  title: "Settings/You/Agents/Connected MCP clients",
  component: ConnectedAgentsCard,
};
export default meta;
type Story = StoryObj<typeof ConnectedAgentsCard>;

export const Connected: Story = { render: story([CLAUDE, LAPSED]) };

// Nobody has connected YET — written out rather than left to the generic empty
// state, because "nothing here" beside a connect guide reads as a failed load.
// The guide stands open under it, which is what makes the empty state an
// instruction rather than a dead end.
export const NoneConnected: Story = { render: story([]) };

// The installation does not serve the tool surface, so discovery 404s and the
// guide is absent rather than broken — a capability this deployment does not
// have, which is the one cause that justifies a surface not being there.
export const ConnectorNotEnabled: Story = {
  render: story([], false),
};

// The lapsed row in dark: struck, not dimmed, beside its danger badge.
export const ConnectedDark: Story = {
  globals: { theme: "dark" },
  render: story([CLAUDE, LAPSED]),
};

// At 390px each row folds: the client over its dates, the permissions under
// them, and the menu at the end of the first line.
export const ConnectedPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story([CLAUDE, LAPSED]),
};

// Disconnect sits in the row's menu and still asks first.
export const DisconnectConfirm: Story = {
  render: story([CLAUDE, LAPSED]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Actions for Claude Desktop" }),
    );
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await body.findByRole("button", { name: "Disconnect Claude Desktop" }),
    );
    await body.findByRole("dialog");
  },
};
