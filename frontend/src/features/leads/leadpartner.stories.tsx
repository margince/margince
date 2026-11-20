// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import { RecordDetails } from "../../design-system/recorddetails";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { useLeadPartnerRow } from "./leadpartner";

// The "Referred by partner" row reads the partner; About's Edit changes it.

const meta: Meta = {
  title: "Records/Leads/Partner",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type Lead = components["schemas"]["Lead"];

const lead: Lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  status: "contacted",
  score: 72,
  partner_company_id: "c-partner",
  source: "manual",
  captured_by: "human:u1",
  writable: true,
  version: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

function PartnerRow({ lead }: Readonly<{ lead: Lead }>) {
  const row = useLeadPartnerRow(lead);
  return (
    <RecordDetails
      sections={[{ key: "about", label: "About", rows: row ? [row] : [] }]}
    />
  );
}

function partnerRow(shown: Lead) {
  installFetchStub({
    "GET /me": meRoute({
      lead: ["read", "update"],
      company: ["read"],
      partner: ["read", "update"],
    }),
    "GET /companies": () =>
      jsonResponse({
        data: [{ id: "c-partner", display_name: "Nordlicht Partners" }],
        page: { has_more: false, next_cursor: null },
      }),
  });
  return (
    <StoryProviders>
      <div style={{ maxWidth: 340 }}>
        <PartnerRow lead={shown} />
      </div>
    </StoryProviders>
  );
}

// Named and not yet accepted: a seat that manages partners may accept the lead for it.
export const Awaiting: Story = {
  render: () => partnerRow(lead),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("Nordlicht Partners")).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Accept for partner" }),
    ).toBeVisible();
  },
};

// Accepted: the row says when, and the partner is kept.
export const Accepted: Story = {
  render: () =>
    partnerRow({ ...lead, partner_accepted_at: "2026-03-04T10:00:00Z" }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByText(/^Accepted for the partner on /),
    ).toBeVisible();
    await expect(
      canvas.queryByRole("button", { name: "Accept for partner" }),
    ).toBeNull();
  },
};
