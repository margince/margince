// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../../api/schema";
import type { GrantSpec } from "../../testing/mefixture";
import {
  emptyPage,
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { DealDetails } from "./deals.details";

// The partner picker on a deal's details asks /partners only for a reader who
// may read companies, since every partner is a company.
const meta: Meta = {
  title: "Records/Deal 360/Partner field",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const deal: components["schemas"]["Deal"] = {
  id: "d1",
  name: "Fleet retrofit",
  status: "open",
  writable: true,
  version: 4,
  pipeline_id: "p1",
  stage_id: "s1",
  currency: "EUR",
  amount_minor: 4800000,
  source: "manual",
  captured_by: "human:u",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const partnerCompany = { id: "c-partner", display_name: "Nordlicht Partners" };

// Counted in the stub, so a play can state that a reader was asked nothing.
const partnerReads = { count: 0 };

function routePartners(allow: GrantSpec) {
  partnerReads.count = 0;
  installFetchStub({
    "GET /me": meRoute(allow, { seat: "full" }),
    "GET /partners": () => {
      partnerReads.count += 1;
      return jsonResponse({
        data: [
          {
            company_id: partnerCompany.id,
            partner_role: "hosting",
            cert_status: "certified",
            margin_tier: "tier2_20",
            relationship_stage: "active",
            version: 1,
            created_at: "2026-01-01T00:00:00Z",
            updated_at: "2026-01-01T00:00:00Z",
          },
        ],
        page: emptyPage.page,
      });
    },
    "GET /companies": () =>
      jsonResponse({ data: [partnerCompany], page: emptyPage.page }),
  });
}

function Details() {
  return (
    <StoryProviders>
      <DealDetails deal={deal} meId="u" stages={[]} />
    </StoryProviders>
  );
}

export const ReaderOfCompanies: Story = {
  beforeEach: () => {
    routePartners({ deal: ["read", "update"], company: ["read"] });
  },
  render: () => <Details />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Change Referring partner" }),
    );
    await userEvent.click(
      await canvas.findByRole("combobox", { name: "Referring partner" }),
    );
    // The choices open in a popover outside the story's canvas.
    await expect(
      await within(canvasElement.ownerDocument.body).findByRole("option", {
        name: partnerCompany.display_name,
      }),
    ).toBeInTheDocument();
    await expect(partnerReads.count).toBe(1);
  },
};

export const NoCompanyRead: Story = {
  beforeEach: () => {
    routePartners({ deal: ["read", "update"] });
  },
  render: () => <Details />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    // The edit affordance waits on /me, which settles after every mount query
    // has already been sent, so the count below is final.
    await canvas.findByRole("button", { name: "Change Deal brief" });
    await waitFor(() =>
      expect(
        canvas.queryByRole("button", { name: "Change Referring partner" }),
      ).toBeNull(),
    );
    await expect(partnerReads.count).toBe(0);
  },
};
