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
import { ConnectedAgentsCard } from "./connected-agents";

// The AI tools holding a credential of their own.
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

function story(passports: Record<string, unknown>[]) {
  return () => {
    globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /passports": () => jsonResponse({ data: passports }),
    });
    return (
      <StoryProviders>
        <ConnectedAgentsCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof ConnectedAgentsCard> = {
  title: "Settings/You/AI access/Your connections",
  component: ConnectedAgentsCard,
};
export default meta;
type Story = StoryObj<typeof ConnectedAgentsCard>;

export const Connected: Story = { render: story([CLAUDE, LAPSED]) };

// Nobody has connected yet: written out rather than left to the generic empty
// state, which reads as a failed load.
export const NoneConnected: Story = { render: story([]) };

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
