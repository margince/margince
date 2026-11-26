// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { LeadProductName, LeadProductPicker } from "./leadproduct";
import { useLeadPatch } from "./leads.patch";

// The product a lead asks about, as its row reads it and in the search the details' Edit opens.

const meta: Meta = {
  title: "Records/Lead/Product",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type Lead = components["schemas"]["Lead"];

const LEAD: Lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  status: "contacted",
  score: 72,
  source: "manual",
  captured_by: "human:u1",
  writable: true,
  version: 1,
  created_at: "2026-10-01T09:00:00Z",
  updated_at: "2026-10-01T09:00:00Z",
};

function Line({ lead }: Readonly<{ lead: Lead }>) {
  const writer = useLeadPatch(lead, lead.id, () => undefined);
  return (
    <>
      <LeadProductName lead={lead} />
      <LeadProductPicker
        lead={lead}
        writer={writer}
        open
        onClose={() => undefined}
      />
    </>
  );
}

function line(productId: string | null) {
  installFetchStub({
    "GET /me": meRoute({ lead: ["read", "update"], product: ["read"] }),
    "GET /products/p-suite": () =>
      jsonResponse({ id: "p-suite", name: "Suite" }),
    "GET /products": () =>
      jsonResponse({
        data: [{ id: "p-suite", name: "Suite" }],
        page: { next_cursor: null, has_more: false },
      }),
  });
  return (
    <StoryProviders>
      <Line lead={{ ...LEAD, product_id: productId }} />
    </StoryProviders>
  );
}

/** A lead that names the product it asks about: the search offers to clear it. */
export const Named: Story = { render: () => line("p-suite") };

/** A lead that names no product yet. */
export const NotNamed: Story = { render: () => line(null) };
