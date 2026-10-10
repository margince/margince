// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { pickOption } from "../design-system/select-testing";
import { AgentToolsCard } from "./settings-agenttools";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// Served descriptions end with the governance clause the server appends, so the
// fixtures carry it: the clamped line and the opened one both have to hold it.
const TOOLS = [
  {
    name: "search_records",
    title: "Search records",
    description:
      'Find contacts, companies, deals, leads and projects by name. (Governance: runs immediately; requires passport scope "read".)',
    required_scope: "read",
    tier: "auto_execute",
    egress: false,
  },
  {
    name: "send_email",
    title: "Send email",
    description:
      'Put a mail on the wire to a real recipient, exactly as it is given. (Governance: a human approves every call before it runs; requires passport scope "send".)',
    required_scope: "send",
    tier: "confirmation_required",
    egress: true,
  },
  {
    name: "list_pipelines",
    title: "List pipelines and their stages",
    description: "Every pipeline with its live stages.",
    tier: "auto_execute",
    egress: false,
  },
];

const SCOUT = {
  id: "pp-1",
  label: "Scout",
  scopes: ["read"],
  created_at: "2026-07-01T08:00:00Z",
  expires_at: null,
  revoked_at: null,
};

function story(passports: readonly Record<string, unknown>[] = []) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /agent-tools": () => jsonResponse({ data: TOOLS }),
      "GET /passports": () =>
        jsonResponse({
          data: passports,
          page: { next_cursor: null, has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <AgentToolsCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof AgentToolsCard> = {
  title: "Settings/You/Agents/Agent tools",
  component: AgentToolsCard,
};
export default meta;
type Story = StoryObj<typeof AgentToolsCard>;

export const Tools: Story = { render: story() };

export const ToolsDark: Story = {
  globals: { theme: "dark" },
  render: story(),
};

export const ToolsPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(),
};

// A row opened: its whole description, governance clause included.
export const DescriptionOpen: Story = {
  render: story(),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Full description of Send email",
      }),
    );
  },
};

export const SearchNarrows: Story = {
  render: story(),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(
      await canvas.findByRole("searchbox", { name: "Search tools" }),
      "mail",
    );
  },
};

export const SearchFindsNothing: Story = {
  render: story(),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(
      await canvas.findByRole("searchbox", { name: "Search tools" }),
      "invoice",
    );
  },
};

// Scoped to a read-only passport, the send tool is struck and says why.
export const ScopedToPassport: Story = {
  render: story([SCOUT]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await pickOption(
      userEvent.setup(),
      await canvas.findByRole("combobox", { name: "All passports" }),
      "Reachable by Scout",
    );
  },
};
