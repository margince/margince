// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { LeadHistoryTab } from "./leadhistory";
import { LeadScreen } from "./leads";

// The lead page's two tabs past the Overview. Deals holds the deal a qualify
// opened, or the door to one, then the project the lead names. History holds
// the cuts in the card's head, with search and dates behind their toggle.

type Lead = components["schemas"]["Lead"];
type Activity = components["schemas"]["Activity"];

const meta: Meta = {
  title: "Records/Leads/Tabs",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const lead: Lead = {
  id: "l-1",
  full_name: "Jonas Weber",
  company_name: "Weber Logistik",
  email: "jonas@weber.example",
  status: "engaged",
  score: 64,
  source: "manual",
  captured_by: "human:u1",
  writable: true,
  version: 1,
  created_at: "2026-08-02T09:12:00Z",
  updated_at: "2026-08-13T08:00:00Z",
};

const activity = (over: Partial<Activity>): Activity =>
  ({
    is_done: true,
    source: "manual",
    captured_by: "human:u1",
    created_at: "2026-08-02T09:12:00Z",
    updated_at: "2026-08-02T09:12:00Z",
    ...over,
  }) as Activity;

const ACTIVITIES = [
  activity({
    id: "a-2",
    kind: "email",
    direction: "inbound",
    subject: "Re: Fleet tracking for Weber Logistik",
    occurred_at: "2026-08-09T15:22:00Z",
  }),
  activity({
    id: "a-1",
    kind: "note",
    subject: "Contact form: fleet tracking",
    body: "We run 120 vans across three depots and want live tracking before winter.",
    occurred_at: "2026-08-02T09:12:00Z",
  }),
];

/** No deal yet: the Deals panel says so, and Qualify is the door to one. */
export const DealsTab: Story = {
  render: () => {
    installFetchStub({
      "GET /leads/l-1": () => jsonResponse(lead),
      "GET /me": meRoute({ lead: ["read", "update"], deal: ["read"] }),
    });
    return (
      <StoryProviders>
        <LeadScreen id="l-1" />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByRole("button", { name: "Deals" }));
    await canvas.findByRole("heading", { name: "Deals" });
  },
};

/** The whole chronology, the cuts in the card's head. */
export const HistoryTab: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ lead: ["read", "update"], activity: ["read"] }),
      "GET /activities": () =>
        jsonResponse({ data: ACTIVITIES, page: { has_more: false } }),
    });
    return (
      <StoryProviders>
        <LeadHistoryTab lead={lead} onOpenEmail={() => {}} />
      </StoryProviders>
    );
  },
};

/** The search and date row opened from its toggle. */
export const HistorySearchOpen: Story = {
  ...HistoryTab,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Search this timeline" }),
    );
  },
};

export const HistoryTabDark: Story = {
  ...HistoryTab,
  globals: { theme: "dark" },
};
