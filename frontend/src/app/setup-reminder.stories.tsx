// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent } from "storybook/test";
import { parseHash } from "../lib/router";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../testing/story-utils";
import { TopBar } from "./topbar";

// The top bar's "Finish setup" trigger and the checklist behind it. The default
// world: Gmail connected, the calendar skipped, so booking waits on it.

const gmail = {
  id: "00000000-0000-4000-8000-000000000011",
  provider: "gmail",
  status: "connected",
  scopes: [],
  account_label: "ada@example.test",
};

type World = Readonly<{
  rows?: readonly object[];
  reason?: string;
  finished?: boolean;
  aiConnected?: boolean;
  /** An admin who skipped the AI key on first run: no model is bound. */
  modelUnbound?: boolean;
}>;

// An AI tool the reader approved through sign-in, which finishes the AI step.
const claudeConnection = {
  id: "00000000-0000-4000-8000-000000000021",
  label: "Claude",
  scopes: ["read", "draft"],
  created_at: "2026-11-01T08:00:00Z",
  last_used_at: "2026-11-04T10:00:00Z",
  expires_at: null,
  revoked_at: null,
  connection: {
    client_id: "claude-ai",
    client_name: "Claude",
    renewable: true,
    connected_at: "2026-11-01T08:00:00Z",
  },
};

function reminder(world: World = {}) {
  stubWithSession(
    {
      "GET /connectors": () =>
        jsonResponse({
          data: world.rows ?? [gmail],
          providers: ["gmail", "graph", "imap"].map((provider) => ({
            provider,
            reason: world.reason ?? "ready",
          })),
        }),
      "GET /voice-profiles": () =>
        jsonResponse({
          data: world.finished ? [{ profile_version: 2 }] : [],
        }),
      "GET /scheduling/profile": () =>
        jsonResponse({
          enabled: world.finished ?? false,
          provider: "",
          calendar_id: "",
          duration_minutes: 30,
          notice_minutes: 240,
          horizon_days: 30,
          buffer_minutes: 0,
          title: "Intro call",
          location: "",
        }),
      "GET /.well-known/oauth-protected-resource": () =>
        jsonResponse({ resource: "https://margince.example/mcp" }),
      "GET /installation/setup": () =>
        jsonResponse({
          complete: !world.modelUnbound,
          steps: [
            {
              step: "ai_models",
              configured: !world.modelUnbound,
              blocking: true,
            },
            { step: "oauth_app", configured: true, blocking: false },
          ],
        }),
      "GET /passports": () =>
        jsonResponse({
          data: (world.aiConnected ?? world.finished) ? [claudeConnection] : [],
          api_base_url: "https://margince.example/v1",
          page: { next_cursor: null, has_more: false },
        }),
    },
    {
      voice_profile: ["read", "create", "update"],
      ...(world.modelUnbound ? { ai_routing: ["read", "update"] } : {}),
    },
  );
  return (
    <StoryProviders>
      <div className="app">
        <main className="main">
          <TopBar
            route={parseHash("#/settings")}
            collapsed={false}
            onOpenSearch={() => undefined}
          />
        </main>
      </div>
    </StoryProviders>
  );
}

const meta = {
  title: "Shell/Setup reminder",
  render: () => reminder(),
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

const openPanel: Story["play"] = async () => {
  await userEvent.click(
    await screen.findByRole("button", { name: /Finish setup/ }),
  );
};

export const Closed: Story = {};
export const ClosedDark: Story = { globals: { theme: "dark" } };
/** Open, hanging at the page gutter like every panel the strip opens. */
export const Open: Story = {
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", { name: /Finish setup/ }),
    );
    const panel = await screen.findByRole("heading", {
      name: "Finish setting up",
    });
    const bar = document.querySelector<HTMLElement>(".topbar");
    const pop = panel.closest(".popover-panel");
    if (!bar || !pop) {
      throw new Error("the top bar or the panel is missing");
    }
    await expect(window.innerWidth - pop.getBoundingClientRect().right).toBe(
      Number.parseFloat(getComputedStyle(bar).paddingRight),
    );
  },
};
export const OpenDark: Story = { play: openPanel, globals: { theme: "dark" } };
// A phone's bar has no room for the trigger, so the account menu carries it.
// The row follows Help and feedback and opens the next step's page.
export const Phone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", { name: /account$/i }),
    );
    const row = await screen.findByRole("menuitem", { name: /Finish setup/ });
    await expect(row).toHaveAttribute("href", "/#/settings/connections");
    await expect(
      screen.queryByRole("button", { name: /Finish setup/ }),
    ).toBeNull();
  },
};

// No company app yet: mail waits on an administrator, so no step offers a verb.
export const Blocked: Story = {
  render: () => reminder({ rows: [], reason: "app_missing" }),
  play: openPanel,
};

// An expired calendar: the current step's one primary verb is Reconnect.
export const NeedsReconnect: Story = {
  render: () =>
    reminder({
      rows: [
        gmail,
        {
          ...gmail,
          id: "00000000-0000-4000-8000-000000000012",
          provider: "gcal",
          status: "reauth_required",
        },
      ],
    }),
  play: openPanel,
};

// The default world opened: booking shows the grey Waiting badge.
export const Waiting: Story = { play: openPanel };

// Mail, voice and booking done: connecting an AI tool is the one step left.
export const AiToolLeft: Story = {
  render: () =>
    reminder({
      rows: [
        gmail,
        {
          ...gmail,
          id: "00000000-0000-4000-8000-000000000012",
          provider: "gcal",
          scopes: ["https://www.googleapis.com/auth/calendar.events"],
        },
      ],
      finished: true,
      aiConnected: false,
    }),
  play: openPanel,
};

// The admin skipped the AI key on first run, so setting it up leads the list.
// Its verb opens Settings, AI models, where the same form waits.
export const AiKeySkipped: Story = {
  render: () => reminder({ modelUnbound: true }),
  play: openPanel,
};

// Every step done: the reminder stands down and the bar carries no trigger.
export const AllDone: Story = {
  render: () =>
    reminder({
      rows: [
        gmail,
        {
          ...gmail,
          id: "00000000-0000-4000-8000-000000000012",
          provider: "gcal",
          scopes: ["https://www.googleapis.com/auth/calendar.events"],
        },
      ],
      finished: true,
    }),
};
