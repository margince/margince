// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../../api/schema";
import { recordNamesRoutes } from "../../testing/recordnames.testkit";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { CompanyProfileDetails } from "./companyprofiledetails";
import { CompanyRail } from "./companyrail";

// The account's own fields in the record's edit form: the details' About
// dialog, and the Profile tab's grouped rows that open the same form.

const meta: Meta = {
  title: "Records/Company 360/Edit form",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;
type Company = components["schemas"]["Company"];

const PARENT = "o-parent";

const company: Company = {
  writable: true,
  id: "o-1",
  workspace_id: "w-1",
  display_name: "Brandt Automotive GmbH",
  lifecycle: "customer",
  owner_id: "u-1",
  industry: "automotive",
  size_band: "51-200",
  parent_company_id: PARENT,
  address: {
    line1: "Werkstraße 12",
    line2: null,
    city: "Munich",
    region: "Bavaria",
    postal_code: "80331",
    country: "DE",
  },
  domains: [
    {
      id: "dom-1",
      domain: "brandt.example",
      is_primary: true,
      source: "manual",
      captured_by: "human:u1",
    },
  ],
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

function stubRoutes() {
  installFetchStub({
    ...recordNamesRoutes({ company: { [PARENT]: "Brandt Holding AG" } }),
    "GET /me": () =>
      jsonResponse({
        user: { id: "u-1", display_name: "Mira Voss" },
        authorization: { objects: { company: { update: true } } },
      }),
    "GET /users": () =>
      jsonResponse({
        data: [{ id: "u-1", display_name: "Mira Voss" }],
        page: { has_more: false, next_cursor: null },
      }),
    "GET /companies/o-1/profile-fields": () => jsonResponse({ data: [] }),
  });
}

function Details() {
  stubRoutes();
  return (
    <StoryProviders>
      <div style={{ maxWidth: 340 }}>
        <CompanyRail company={company} loading={false} />
      </div>
    </StoryProviders>
  );
}

// About's Edit opens one form with only About's fields, seeded from the record.
export const AboutDialog: Story = {
  render: () => <Details />,
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    await user.click(
      await within(canvasElement).findByRole("button", { name: "Edit About" }),
    );
    const dialog = await screen.findByRole("dialog", { name: "About" });
    await waitFor(() =>
      expect(within(dialog).getByDisplayValue("Werkstraße 12")).toBeVisible(),
    );
    await expect(
      within(dialog).getByDisplayValue("brandt.example"),
    ).toBeVisible();
  },
};

export const AboutDialogPhone: Story = {
  ...AboutDialog,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// The parent stays a link to its record; its pencil opens the same form.
export const ProfileParentLink: Story = {
  render: () => {
    stubRoutes();
    return (
      <StoryProviders>
        <CompanyProfileDetails
          company={company}
          edit={{ canEdit: true }}
          fields={[]}
          fieldsLoaded
        />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("link", { name: /Brandt Holding AG/ }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: /^Change Parent company/ }),
    ).toBeVisible();
  },
};
