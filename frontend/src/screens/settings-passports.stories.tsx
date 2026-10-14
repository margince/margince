// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { busyUsage } from "./agentallowances.fixtures";
import { PassportCard } from "./settings-passports";

// Three passports: one in use and stopped on a used-up allowance, and one
// never used with every permission. The third is revoked: it waits behind the
// toggle and shows struck, with no verb left to offer.
const PASSPORTS = [
  {
    id: "pp-1",
    label: "Claude Code on my laptop",
    scopes: ["read", "draft"],
    created_at: "2026-07-01T08:00:00Z",
    last_used_at: "2026-09-28T14:10:00Z",
    expires_at: "2026-10-31T08:00:00Z",
    revoked_at: null,
    usage: busyUsage,
  },
  {
    id: "pp-2",
    label: "Nightly enrichment script with a name long enough to truncate",
    scopes: ["read", "draft", "write", "send", "enrich"],
    created_at: "2026-08-01T08:00:00Z",
    last_used_at: null,
    expires_at: null,
    revoked_at: null,
  },
  {
    id: "pp-3",
    label: "Old runner",
    scopes: ["read"],
    created_at: "2026-06-01T08:00:00Z",
    last_used_at: "2026-06-20T09:00:00Z",
    expires_at: "2026-07-01T08:00:00Z",
    revoked_at: "2026-06-21T08:00:00Z",
  },
];

function story(passports: readonly Record<string, unknown>[]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /passports": () =>
        jsonResponse({
          data: passports,
          api_base_url: "https://crm.example.com/v1",
          page: { next_cursor: null, has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <PassportCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof PassportCard> = {
  title: "Settings/You/Agents/Agent passports",
  component: PassportCard,
};
export default meta;
type Story = StoryObj<typeof PassportCard>;

export const Passports: Story = { render: story(PASSPORTS) };

export const PassportsDark: Story = {
  globals: { theme: "dark" },
  render: story(PASSPORTS),
};

// At 390px each row folds: the name over its dates, the permissions under it,
// and the menu at the end of the first line.
export const PassportsPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(PASSPORTS),
};

export const NoPassports: Story = { render: story([]) };

export const RevokeConfirm: Story = {
  render: story(PASSPORTS),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Actions for Claude Code on my laptop",
      }),
    );
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await body.findByRole("button", {
        name: "Revoke Claude Code on my laptop",
      }),
    );
    await body.findByRole("dialog");
  },
};

export const RevokedShown: Story = {
  render: story(PASSPORTS),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Show 1 revoked passport" }),
    );
    await canvas.findByText("Old runner");
  },
};

// A row opened onto all four allowances of this window.
export const UsageOpen: Story = {
  render: story(PASSPORTS),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Show this window’s usage for Claude Code on my laptop",
      }),
    );
    await canvas.findByRole("meter", { name: "Records read" });
  },
};
