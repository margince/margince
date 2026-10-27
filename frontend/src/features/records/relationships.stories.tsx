// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { translate } from "../../i18n";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { RelationshipsTab } from "./relationshiprows";
import { AddRelationshipAction } from "./relationships";

// RelationshipsTab reads GET /relationships?contact_id=… (there is no
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
  title: "Records/Company 360/Relationships",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const employmentRel = {
  id: "rel-1",
  kind: "employment",
  contact_id: "p-1",
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
        <RelationshipsTab scope={{ contact_id: "p-1" }} />
      </StoryProviders>
    );
  },
};

/** The add form, open: the counterparty search above the edge's own fields. */
export const Adding: Story = {
  ...WithRelationships,
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByTestId("add-relationship"),
    );
    await screen.findByRole("dialog");
  },
};

// The bodies the missing-seat story's form posts.
const seated: unknown[] = [];

/**
 * The verb offered for one missing seat. Its description names the gap it
 * fills; its form shows that seat in the reader's words and sends its key.
 */
export const AddingForAMissingSeat: Story = {
  render: () => {
    seated.length = 0;
    stubWithSession(
      {
        "GET /contacts": () =>
          jsonResponse({
            data: [{ id: "p-9", full_name: "Mai Tran" }],
            page: { next_cursor: null },
          }),
        "POST /relationships": (body) => {
          seated.push(body);
          return jsonResponse({ ...employmentRel, id: "rel-new" }, 201);
        },
      },
      MAY_WRITE_RELATIONSHIPS,
    );
    return (
      <StoryProviders>
        <p id="missing-seat">Missing: an engaged champion</p>
        <AddRelationshipAction
          scope={{ deal_id: "d-1" }}
          seatRole="champion"
          describedBy="missing-seat"
        />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const verb = await within(canvasElement).findByRole("button", {
      name: "Add stakeholder",
    });
    await expect(verb).toHaveAccessibleDescription(
      "Missing: an engaged champion",
    );
    await userEvent.click(verb);
    const dialog = within(await screen.findByRole("dialog"));
    await expect(dialog.getByRole("textbox", { name: "Role" })).toHaveValue(
      translate("en", "co.roleLabel.champion"),
    );
    await userEvent.type(
      dialog.getByPlaceholderText(translate("en", "merge.searchPlaceholder")),
      "mai",
    );
    await userEvent.click(
      await dialog.findByRole("button", { name: "Mai Tran" }),
    );
    await userEvent.click(
      dialog.getByRole("button", { name: translate("en", "create.save") }),
    );
    await waitFor(() => expect(seated).toHaveLength(1));
    await expect(seated[0]).toMatchObject({ role: "champion" });
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
        <RelationshipsTab scope={{ contact_id: "p-1" }} />
      </StoryProviders>
    );
  },
};

// A role holding no `relationship` grant at all: the head band keeps its
// measure with nothing beside the title, and the rows carry no verbs. A
// withheld verb is absent rather than disabled — there is no fact about this
// record to report, so there is nothing for a refusal to explain.
export const NoVerbsForThisRole: Story = {
  render: () => {
    stubWithSession(
      {
        "GET /relationships": () =>
          jsonResponse({
            data: [employmentRel, partnerOfRel],
            page: { next_cursor: null, has_more: false },
          }),
      },
      {},
    );
    return (
      <StoryProviders>
        <RelationshipsTab scope={{ contact_id: "p-1" }} />
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
