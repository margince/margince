// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { RelationshipsTab } from "./relationships";
import { jsonResponse, StoryProviders, stubWithSession } from "./story-utils";

// RelationshipsTab reads GET /relationships?person_id=… (there is no
// GET /relationships/{id} in the contract — every row is hydrated straight
// off the list read). The fixture mirrors contacts.test.tsx's employmentRel.
//
// The panel is a WORK surface: it gates Add, Edit and Remove on
// `relationship` create/update/delete, so every story here holds all three.
// A story that left the session unrouted would draw the same rows with no
// verbs on them, which is the read-only panel under another name.
const MAY_WRITE_RELATIONSHIPS = {
  relationship: ["create", "update", "delete"],
} as const;

const meta: Meta = {
  title: "Records/Relationships",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const employmentRel = {
  id: "rel-1",
  kind: "employment",
  person_id: "p-1",
  company_id: "o-1",
  role: "cto",
  is_current_primary: true,
  started_at: "2024-01-01",
  ended_at: null,
  source: "manual",
  captured_by: "human:u1",
  version: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const partnerOfRel = {
  ...employmentRel,
  id: "rel-2",
  kind: "partner_of",
  role: "referral partner",
  company_id: "o-2",
};

export const WithRelationships: Story = {
  render: () => {
    stubWithSession(
      {
        "GET /relationships": () =>
          jsonResponse({
            data: [employmentRel, partnerOfRel],
            page: { next_cursor: null, has_more: false },
          }),
      },
      MAY_WRITE_RELATIONSHIPS,
    );
    return (
      <StoryProviders>
        <RelationshipsTab scope={{ person_id: "p-1" }} />
      </StoryProviders>
    );
  },
};

export const Empty: Story = {
  render: () => {
    stubWithSession(
      {
        "GET /relationships": () =>
          jsonResponse({
            data: [],
            page: { next_cursor: null, has_more: false },
          }),
      },
      MAY_WRITE_RELATIONSHIPS,
    );
    return (
      <StoryProviders>
        <RelationshipsTab scope={{ person_id: "p-1" }} />
      </StoryProviders>
    );
  },
};

const stakeholderRel = {
  ...employmentRel,
  id: "rel-3",
  kind: "deal_stakeholder",
  deal_id: "d-1",
  company_id: null,
  role: "champion",
  is_current_primary: false,
};

// The same panel under a deal scope: one kind, so the Kind column and the Kind
// picker both go — a column repeating one badge down every row, and a question
// with a single answer.
export const DealStakeholders: Story = {
  render: () => {
    stubWithSession(
      {
        "GET /relationships": () =>
          jsonResponse({
            data: [
              stakeholderRel,
              { ...stakeholderRel, id: "rel-4", role: "economic_buyer" },
            ],
            page: { next_cursor: null, has_more: false },
          }),
      },
      MAY_WRITE_RELATIONSHIPS,
    );
    return (
      <StoryProviders>
        <RelationshipsTab scope={{ deal_id: "d-1" }} />
      </StoryProviders>
    );
  },
};

export const DealStakeholdersEmpty: Story = {
  render: () => {
    stubWithSession(
      {
        "GET /relationships": () =>
          jsonResponse({
            data: [],
            page: { next_cursor: null, has_more: false },
          }),
      },
      MAY_WRITE_RELATIONSHIPS,
    );
    return (
      <StoryProviders>
        <RelationshipsTab scope={{ deal_id: "d-1" }} />
      </StoryProviders>
    );
  },
};
