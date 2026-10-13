// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import {
  LeadDisqualifyReasonsCard,
  LeadHandlingCard,
  LeadSourcesCard,
} from "./leadvocab";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// Settings › Lead handling: where leads come from, the reply deadline, and why
// leads are dropped. Every role reads the lists; the custom_field write verbs
// decide who may change them.

function source(
  key: string,
  label: string,
  intent: string,
  extra: Record<string, unknown> = {},
) {
  return {
    id: `src-${key}`,
    key,
    label,
    intent,
    sort_order: 10,
    active: true,
    system: false,
    lead_count: 0,
    version: 1,
    created_at: "2026-08-01T08:00:00Z",
    updated_at: "2026-08-01T08:00:00Z",
    ...extra,
  };
}

const SOURCES = {
  data: [
    source("manual", "Created manually", "neutral", {
      system: true,
      lead_count: 12,
    }),
    source("inbound", "Inbound", "high", { system: true, lead_count: 31 }),
    source("webform", "Web form", "high", { system: true, lead_count: 4 }),
    source("referral", "Referral", "high", { system: true, lead_count: 9 }),
    source("import", "Import", "low", {
      system: true,
      lead_count: 140,
      active: false,
    }),
    source("crawl", "Web research", "low", { system: true, lead_count: 2 }),
    source("trade_show", "Trade show", "high", { lead_count: 3 }),
  ],
  discovered: [{ key: "connector:apollo", lead_count: 27 }],
};

const REASONS = {
  data: [
    ["r1", "Not a good fit", 6],
    ["r2", "Bad timing", 11],
    ["r3", "No budget", 2],
    ["r4", "No decision power", 0],
    ["r5", "Chose a competitor", 1],
    ["r6", "No interest", 4],
    ["r7", "Not reachable", 8],
    ["r8", "Duplicate or spam", 3],
  ].map(([id, label, count]) => ({
    id,
    label,
    sort_order: 10,
    active: true,
    system: true,
    lead_count: count,
    version: 1,
    created_at: "2026-08-01T08:00:00Z",
    updated_at: "2026-08-01T08:00:00Z",
  })),
};

const ADMIN = { custom_field: ["read", "create", "update", "delete"] } as const;
const READER = { custom_field: ["read"] } as const;

function story(allow: Parameters<typeof meRoute>[0], slaOn: boolean) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /lead-sources": () => jsonResponse(SOURCES),
      "GET /lead-disqualify-reasons": () => jsonResponse(REASONS),
      "GET /leads/settings": () =>
        jsonResponse({
          first_response_enabled: slaOn,
          first_response_target_minutes: 240,
        }),
    });
    return (
      <StoryProviders>
        <LeadSourcesCard />
        <LeadHandlingCard />
        <LeadDisqualifyReasonsCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof LeadSourcesCard> = {
  title: "Settings/Sales/Lead handling/Lead vocabularies",
  component: LeadSourcesCard,
};
export default meta;
type Story = StoryObj<typeof LeadSourcesCard>;

export const Admin: Story = { render: story(ADMIN, false) };
export const AdminWithTargetOn: Story = { render: story(ADMIN, true) };
export const Reader: Story = { render: story(READER, false) };

// The terms tables in dark: the hairline between two rows is a token
// (`--borderSubtle`), and a table that loses its rules reads as a wall.
export const AdminDark: Story = {
  globals: { theme: "dark" },
  render: story(ADMIN, true),
};
export const ReaderDark: Story = {
  globals: { theme: "dark" },
  render: story(READER, false),
};

// A source is a name AND a score effect, so its form is a dialog behind the
// card's header verb, with the three effects as readable choices.
export const AddingSource: Story = {
  render: story(ADMIN, false),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "New source" }),
    );
  },
};
export const AddingSourceDark: Story = {
  ...AddingSource,
  globals: { theme: "dark" },
};

// A built-in source's menu: Rename, and a caption saying why Remove is absent.
export const BuiltInMenu: Story = {
  render: story(ADMIN, false),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "More actions for Inbound" }),
    );
  },
};
export const BuiltInMenuDark: Story = {
  ...BuiltInMenu,
  globals: { theme: "dark" },
};

// A target outside the 15-minutes-to-7-days window the server enforces, drawn
// under the row's label and description.
export const TargetRefused: Story = {
  render: story(ADMIN, true),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const minutes = await canvas.findByTestId("lead-first-response-target");
    await userEvent.clear(minutes);
    await userEvent.type(minutes, "2");
    await userEvent.tab();
    await canvas.findByRole("alert");
  },
};
