// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import type { GrantSpec } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { Employers } from "./contactemployers";
import "./contact360.css";

// The proposal drawn where a reader meets it: the contact's Company card,
// for a contact who works nowhere yet and whose signature named a company.

type Contact360 = components["schemas"]["Contact360"];

const CONTACT_ID = "01930000-0000-7000-8000-0000000000c1";
const HOLDER_ID = "01930000-0000-7000-8000-0000000000o2";

function withSignatureCompany(company: string, writable = true): Contact360 {
  return {
    as_of: "2026-09-01T00:00:00Z",
    sections_omitted: [],
    contact: {
      id: CONTACT_ID,
      full_name: "Dana Buyer",
      writable,
      source: "capture_enrich",
      captured_by: "agent:enrich",
      created_at: "2026-09-01T00:00:00Z",
      updated_at: "2026-09-01T00:00:00Z",
    },
    employments: { data: [], page: { has_more: false } },
    profile_fields: [
      {
        field: "company_name",
        value: company,
        evidence_snippet: `Dana Buyer\nHead of Fleet\n${company}`,
        source: "capture_enrich",
        captured_by: "agent:enrich",
        captured_at: "2026-08-28T09:12:00Z",
        claim_key: "profile_field:company_name",
      },
    ],
  };
}

const ALL_GRANTS: GrantSpec = {
  contact: ["read", "update"],
  relationship: ["read", "create", "update", "delete"],
  company: ["read", "create"],
};

function card(
  view: Contact360,
  grants: GrantSpec = ALL_GRANTS,
  width: number | string = 420,
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(grants),
      [`POST /contacts/${CONTACT_ID}/employer`]: () =>
        jsonResponse(
          {
            type: "about:blank",
            title: "Conflict",
            status: 409,
            code: "duplicate_domain",
            details: { existing_id: HOLDER_ID },
          },
          409,
        ),
      [`GET /companies/${HOLDER_ID}`]: () =>
        jsonResponse({ id: HOLDER_ID, display_name: "Nordlicht Logistik AG" }),
    });
    return (
      <StoryProviders>
        <div style={{ maxWidth: width }}>
          <Employers view={view} />
        </div>
      </StoryProviders>
    );
  };
}

const meta: Meta = {
  title: "Records/Contact 360/Employer proposal",
};
export default meta;

type Story = StoryObj;

export const Proposal: Story = {
  render: card(withSignatureCompany("Nordlicht Logistik GmbH")),
};

export const ProposalDark: Story = {
  ...Proposal,
  globals: { theme: "dark" },
};

export const ProposalPhone: Story = {
  render: card(
    withSignatureCompany("Nordlicht Logistik GmbH"),
    ALL_GRANTS,
    "100%",
  ),
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const LongCompanyName: Story = {
  render: card(
    withSignatureCompany(
      "Nordlicht Logistik und Lagerhaltung Gesellschaft mit beschränkter Haftung",
    ),
  ),
};

export const LongCompanyNamePhone: Story = {
  ...LongCompanyName,
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// The role may not link a company, so both links are refused with one reason.
export const LinkDenied: Story = {
  render: card(withSignatureCompany("Nordlicht Logistik GmbH"), {
    contact: ["read", "update"],
    company: ["read", "create"],
  }),
};

// The role may link an existing company but not create one.
export const CreateDenied: Story = {
  render: card(withSignatureCompany("Nordlicht Logistik GmbH"), {
    contact: ["read", "update"],
    relationship: ["read", "create"],
    company: ["read"],
  }),
};

export const ReadOnly: Story = {
  render: card(withSignatureCompany("Nordlicht Logistik GmbH", false)),
};

// Create and link meets a company that already holds the contact's domain.
export const DuplicateDomain: Story = {
  render: card(withSignatureCompany("Nordlicht Logistik GmbH")),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const user = userEvent.setup();
    await user.click(
      await canvas.findByRole("button", { name: "Create and link" }),
    );
    await expect(
      await canvas.findByRole("button", { name: "Link that company" }),
    ).toBeVisible();
  },
};

export const DuplicateDomainPhone: Story = {
  ...DuplicateDomain,
  render: card(
    withSignatureCompany("Nordlicht Logistik GmbH"),
    ALL_GRANTS,
    "100%",
  ),
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
