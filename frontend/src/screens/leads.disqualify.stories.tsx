// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { DisqualifyDialog } from "./leads.disqualify";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// Closing a lead asks WHY, from the administered list. The dialog names the
// lead in its heading, which is where the naming rule shows: a lead the product
// stored with an empty name is named by its address here, exactly as the server
// names it when the same lead is promoted.

const meta: Meta = {
  title: "Records/Leads/Disqualify",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

type Lead = components["schemas"]["Lead"];

const lead: Lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  email: "jonas@nordwind.example",
  company_name: "Nordwind Logistik",
  status: "contacted",
  score: 72,
  source: "manual",
  captured_by: "human:u1",
  version: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const reasons = [
  { id: "r-1", label: "No budget", position: 1, active: true },
  { id: "r-2", label: "Wrong fit", position: 2, active: true },
  { id: "r-3", label: "Retired reason", position: 3, active: false },
];

function withRoutes(subject: Lead) {
  installFetchStub({
    "GET /me": meRoute({ lead: ["read", "update"] }),
    "GET /lead-disqualify-reasons": () => jsonResponse({ data: reasons }),
  });
  return (
    <StoryProviders>
      <DisqualifyDialog
        lead={subject}
        open
        onClose={() => {}}
        onDisqualified={() => {}}
      />
    </StoryProviders>
  );
}

export const Named: Story = {
  render: () => withRoutes(lead),
};

// A `full_name` that is present and EMPTY is not a name — nothing between a
// `CreateLead` body and the stored row refuses one. The dialog names the lead by
// the address rather than heading a destructive confirmation with a blank.
export const NamedByItsAddress: Story = {
  render: () => withRoutes({ ...lead, full_name: "" }),
};
