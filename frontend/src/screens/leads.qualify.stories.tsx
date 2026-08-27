// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { QualifyDialog } from "./leads.qualify";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// Promoting a lead: what it will land on, and the deal it may open alongside.
// The dialog names the lead twice — in its heading, and in the deal name it
// suggests — and both go through the one naming rule, so a lead stored with an
// empty name is named by its address rather than leaving a blank in a deal name
// somebody is about to save.

const meta: Meta = {
  title: "Records/Leads/Qualify",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

type Lead = components["schemas"]["Lead"];

const lead: Lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  email: "jonas@nordwind.example",
  status: "engaged",
  score: 72,
  source: "manual",
  captured_by: "human:u1",
  version: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const pipeline = {
  id: "pl-1",
  name: "New business",
  is_default: true,
  position: 0,
  stages: [
    {
      id: "s-1",
      pipeline_id: "pl-1",
      name: "Discovery",
      position: 1,
      semantic: "open" as const,
      win_probability: 20,
    },
    {
      id: "s-2",
      pipeline_id: "pl-1",
      name: "Proposal",
      position: 2,
      semantic: "open" as const,
      win_probability: 60,
    },
  ],
};

function withRoutes(subject: Lead) {
  installFetchStub({
    "GET /me": meRoute({ lead: ["read", "update"], deal: ["read", "create"] }),
    "GET /installation/settings": () =>
      jsonResponse({ base_currency: "EUR", max_upload_bytes: 10_000_000 }),
    "GET /pipelines": () => jsonResponse({ data: [pipeline] }),
    [`GET /leads/${subject.id}/promote-preview`]: () =>
      jsonResponse({ outcome: "create", person_withheld: false }),
  });
  return (
    <StoryProviders>
      <QualifyDialog
        lead={subject}
        open
        onClose={() => {}}
        onQualified={() => {}}
      />
    </StoryProviders>
  );
}

export const Named: Story = {
  render: () => withRoutes(lead),
};

// The suggested deal name is the lead's own naming with the stage after it. A
// lead whose `full_name` is present and empty would otherwise seed that field
// with " — Discovery", which is a deal name somebody saves without noticing.
export const NamedByItsAddress: Story = {
  render: () => withRoutes({ ...lead, full_name: "" }),
};
