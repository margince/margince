// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { LeadCompany } from "./leadcompany";

// The offer to link the company the server matched, with the rule that matched it.

type Lead = components["schemas"]["Lead"];

const meta: Meta = {
  title: "Records/Leads/Company link offer",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const lead: Lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  company_name: "Nordwind Logistik",
  status: "contacted",
  score: 40,
  source: "manual",
  captured_by: "human:u1",
  writable: true,
  version: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

function offer(over: Partial<Lead>) {
  return () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse(
          meFixture({
            roles: ["rep"],
            allow: { lead: ["read", "update"], company: ["read"] },
          }),
        ),
    });
    return (
      <StoryProviders>
        <LeadCompany lead={{ ...lead, ...over }} />
      </StoryProviders>
    );
  };
}

/** The company claims the website domain the lead came with. */
export const MatchedByDomain: Story = {
  render: offer({
    company_name: null,
    candidate_company_key: "nordwind.example",
    company_match: {
      id: "c-9",
      name: "Nordwind Logistik GmbH",
      matched_by: "domain",
    },
  }),
};

/** The company's name equals the one typed on the lead. */
export const MatchedByName: Story = {
  render: offer({
    company_match: {
      id: "c-9",
      name: "Nordwind Logistik GmbH",
      matched_by: "name",
    },
  }),
};
