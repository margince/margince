// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { LeadProductLine } from "./leadproduct";
import { useLeadPatch } from "./leads.patch";

// The product a lead asks about: named, and not yet named.

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
  return <LeadProductLine lead={lead} writer={writer} />;
}

function line(productId: string | null) {
  installFetchStub({
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

/** A lead that names the product it asks about. */
export const Named: Story = { render: () => line("p-suite") };

/** A lead that names no product yet. */
export const NotNamed: Story = { render: () => line(null) };
