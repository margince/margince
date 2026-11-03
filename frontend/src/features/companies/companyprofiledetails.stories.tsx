// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { CompanyProfileDetails } from "./companyprofiledetails";
import type { Edit } from "./companyrail";

// The company's filed fields the details column leaves out, each one a value
// that opens its own edit.

const meta: Meta = {
  title: "Records/Company 360/Profile details",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;
type Company = components["schemas"]["Company"];
type ProfileField = components["schemas"]["CompanyProfileField"];

const PARENT = "o-parent";
const LINKEDIN = "https://linkedin.com/company/brandt";

const company: Company = {
  writable: true,
  id: "o-1",
  workspace_id: "w-1",
  display_name: "Brandt Automotive GmbH",
  legal_name: "Brandt Automotive GmbH",
  relationship_types: ["customer", "partner"],
  parent_company_id: PARENT,
  linkedin_url: LINKEDIN,
  phone: "+49 89 1234567",
  email: "info@brandt.example",
  lifecycle: "customer",
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

const registeredAddress: ProfileField = {
  id: "pf-addr",
  field: "registered_address",
  value: "Kaiserdamm 1, 14057 Berlin",
  source: "site_read",
  captured_by: "agent:deepread",
  updated_at: "2026-06-01T08:00:00Z",
};

function Details({
  record = company,
  edit = { canEdit: true },
}: Readonly<{ record?: Company; edit?: Edit }>) {
  installFetchStub({
    "GET /me": meRoute({ company: ["read", "update"] }),
    "GET /companies": () =>
      jsonResponse({
        data: [{ id: PARENT, display_name: "Brandt Holding AG" }],
        page: { has_more: false, next_cursor: null },
      }),
    [`GET /records/company/${record.id}/tags`]: () =>
      jsonResponse({ data: [], withheld: false }),
  });
  return (
    <StoryProviders>
      <div style={{ maxWidth: 420 }}>
        <CompanyProfileDetails
          company={record}
          edit={edit}
          fields={[registeredAddress]}
          fieldsLoaded
        />
      </div>
    </StoryProviders>
  );
}

// Every value editable; the LinkedIn profile and the parent stay links.
export const Editable: Story = {
  render: () => <Details />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("link", { name: /linkedin\.com/ }),
    ).toHaveAttribute("href", LINKEDIN);
    await expect(
      canvas.getByRole("button", { name: /^Change LinkedIn URL/ }),
    ).toBeVisible();
    await expect(
      await canvas.findByRole("link", { name: /Brandt Holding AG/ }),
    ).toBeVisible();
  },
};

export const EditableDark: Story = {
  render: () => <Details />,
  globals: { theme: "dark" },
};

// Archived: every value plain, none opens an edit.
export const ReadOnly: Story = {
  render: () => (
    <Details
      record={{ ...company, archived_at: "2026-07-15T00:00:00Z" }}
      edit={{ canEdit: false, reason: "This company is archived." }}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("link", { name: /linkedin\.com/ }),
    ).toBeVisible();
    await expect(
      canvas.queryAllByRole("button", { name: /^Change / }),
    ).toHaveLength(0);
  },
};

// A set of relationship types edits in the record's form, one field of it.
export const RelationshipTypesDialog: Story = {
  render: () => <Details />,
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: /^Change Relationship type/,
      }),
    );
    const dialog = await screen.findByRole("dialog", {
      name: "Relationship type",
    });
    await waitFor(() => expect(dialog).toBeVisible());
  },
};

export const RelationshipTypesDialogPhone: Story = {
  ...RelationshipTypesDialog,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
