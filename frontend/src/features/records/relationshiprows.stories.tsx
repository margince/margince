// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import { translate } from "../../i18n";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { RelationshipRows } from "./relationshiprows";

// The rows without their panel, the shape the deal's committee card mounts.
// The remove dialog portals to document.body, so it is reached via `screen`.
const meta: Meta = {
  title: "Records/Record 360/Relationship rows",
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

// A refused hard delete keeps the dialog open and says why under the question.
export const RemoveRefused: Story = {
  render: () => {
    stubWithSession(
      {
        "GET /relationships": () =>
          jsonResponse({
            data: [employmentRel],
            page: { next_cursor: null, has_more: false },
          }),
        "DELETE /relationships/rel-1": () =>
          jsonResponse(
            {
              type: "about:blank",
              title: "Relationship in use",
              status: 409,
              detail: "This edge is the contact's current primary employment.",
            },
            409,
          ),
      },
      { relationship: ["create", "update", "delete"] },
    );
    return (
      <StoryProviders>
        <RelationshipRows scope={{ contact_id: "p-1" }} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByTestId("remove-relationship"),
    );
    await userEvent.click(
      within(await screen.findByRole("dialog")).getByRole("button", {
        name: "Remove",
      }),
    );
    await expect(await screen.findByRole("alert")).toHaveTextContent(
      "This edge is the contact's current primary employment.",
    );
  },
};

// A deal role reads in the reader's words, in the row and in its edit form,
// never as the key the coverage rules count.
export const StakeholderRoleInVietnamese: Story = {
  render: () => {
    stubWithSession(
      {
        "GET /relationships": () =>
          jsonResponse({
            data: [
              {
                ...employmentRel,
                kind: "deal_stakeholder",
                deal_id: "d-1",
                company_id: undefined,
                role: "economic_buyer",
                is_current_primary: false,
              },
            ],
            page: { next_cursor: null, has_more: false },
          }),
      },
      { relationship: ["read", "update"] },
    );
    return (
      <StoryProviders locale="vi">
        <RelationshipRows scope={{ deal_id: "d-1" }} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const label = translate("vi", "co.roleLabel.economic_buyer");
    const table = await within(canvasElement).findByRole("table");
    await expect(
      await within(table).findByRole("cell", { name: label }),
    ).toBeVisible();
    await expect(within(table).queryByText("economic_buyer")).toBeNull();
    await userEvent.click(
      within(table).getByRole("button", {
        name: translate("vi", "record.edit"),
      }),
    );
    const form = within(await screen.findByRole("dialog"));
    await expect(
      form.getByRole("textbox", { name: translate("vi", "rel.role") }),
    ).toHaveValue(label);
  },
};
