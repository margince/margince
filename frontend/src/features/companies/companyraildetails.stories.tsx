// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { CompanyRail } from "./companyrail";

// The company's details under its head: set fields as rows, every empty one
// folded behind one link, and writability gating the verbs, not the values.

const meta: Meta = {
  title: "Records/Company 360/Details",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type Company = components["schemas"]["Company"];
type ProfileField = components["schemas"]["CompanyProfileField"];

const page = { has_more: false, next_cursor: null };

const company: Company = {
  // Absent reads as NOT writable, which is the fail-closed default a real
  // response never relies on: the server answers this per row.
  writable: true,
  id: "o-1",
  workspace_id: "w-1",
  display_name: "Brandt Automotive GmbH",
  legal_name: "Brandt Automotive GmbH",
  lifecycle: "customer",
  owner_id: "u-1",
  industry: "automotive",
  size_band: "51-200",
  linkedin_url: "https://linkedin.com/company/brandt",
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
  description: "Fleet electrification pilot, renewing in Q3.",
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

// The two sidecar claims the grid reads, as a crawl that found an imprint
// leaves them. Stories that want the unstated case pass an empty list.
const sidecarFields: ProfileField[] = [
  {
    id: "pf-vat",
    field: "register_vat",
    value: "DE811907980",
    source: "site_read",
    captured_by: "agent:deepread",
    updated_at: "2026-06-01T08:00:00Z",
  },
  {
    id: "pf-addr",
    field: "registered_address",
    value: "Kaiserdamm 1, 14057 Berlin",
    source: "site_read",
    captured_by: "agent:deepread",
    updated_at: "2026-06-01T08:00:00Z",
  },
];

function Details({
  company,
  profileFields = sidecarFields,
}: Readonly<{
  company: Company;
  profileFields?: readonly ProfileField[];
}>) {
  installFetchStub({
    "GET /me": () =>
      jsonResponse({
        user: { id: "u-1", display_name: "Mira Voss" },
        authorization: { objects: { company: { update: true } } },
      }),
    "GET /users": () =>
      jsonResponse({
        data: [{ id: "u-1", display_name: "Mira Voss" }],
        page,
      }),
    "GET /companies/o-1/profile-fields": () =>
      jsonResponse({ data: profileFields }),
  });
  return (
    <StoryProviders>
      <div style={{ maxWidth: 340 }}>
        <CompanyRail company={company} loading={false} />
      </div>
    </StoryProviders>
  );
}

export const Editable: Story = {
  render: () => <Details company={company} />,
};

// The address a crawled record actually has: none of it. The row folds behind
// "Show N empty fields" with the other blanks.
export const AddressAbsent: Story = {
  render: () => (
    <Details
      company={{
        ...company,
        address: {
          line1: null,
          line2: null,
          city: null,
          region: null,
          postal_code: null,
          country: null,
        },
      }}
    />
  ),
};

// Any part set shows the row, as "City, CC".
export const AddressPartlyFilled: Story = {
  render: () => (
    <Details
      company={{ ...company, address: { city: "Munich", country: "DE" } }}
    />
  ),
};

// Archived: every row still shows its value, none offers the edit affordance
// — the one state that exercises the grid's `readOnlyReason` half, which no
// amount of RBAC grant in the Editable story above can reach.
export const Archived: Story = {
  render: () => (
    <Details company={{ ...company, archived_at: "2026-07-15T00:00:00Z" }} />
  ),
};

// The legal identity nobody has stated: the VAT row invites a number, and the
// registered address waits behind the empty-fields link.
export const LegalIdentityUnstated: Story = {
  render: () => <Details company={company} profileFields={[]} />,
};

export const EmptyFields: Story = {
  render: () => (
    <Details
      profileFields={[]}
      company={{
        ...company,
        legal_name: null,
        industry: null,
        size_band: null,
        linkedin_url: null,
        description: null,
        address: {
          line1: null,
          line2: null,
          city: null,
          region: null,
          postal_code: null,
          country: null,
        },
        domains: [],
      }}
    />
  ),
};
