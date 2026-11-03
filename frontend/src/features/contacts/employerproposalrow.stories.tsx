// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { RecordDetails } from "../../design-system/recorddetails";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { useContactAboutSections } from "./contactdetails";
import "./contact360.css";

// The rail's About section for a contact who works nowhere yet and whose
// signature named a company, at the rail's 320px column.

type Contact360 = components["schemas"]["Contact360"];

function withSignatureCompany(company: string): Contact360 {
  return {
    as_of: "2026-09-01T00:00:00Z",
    sections_omitted: [],
    contact: {
      id: "01930000-0000-7000-8000-0000000000c1",
      full_name: "Paula Prüfer",
      title: "Head of Purchasing",
      primary_email: "paula.pruefer@company-a.example",
      writable: true,
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
        evidence_snippet: `Paula Prüfer\nHead of Purchasing\n${company}`,
        source: "capture_enrich",
        captured_by: "agent:enrich",
        captured_at: "2026-08-28T09:12:00Z",
        claim_key: "profile_field:company_name",
      },
    ],
  };
}

function About({ view }: Readonly<{ view: Contact360 }>) {
  return <RecordDetails sections={useContactAboutSections(view)} />;
}

function rail(view: Contact360, width: number | string = 320) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({
        contact: ["read", "update"],
        relationship: ["read", "create"],
        company: ["read", "create"],
      }),
    });
    return (
      <StoryProviders>
        <div style={{ maxWidth: width }}>
          <About view={view} />
        </div>
      </StoryProviders>
    );
  };
}

const meta: Meta = {
  title: "Records/Contact 360/Rail employer proposal",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

export const InRail: Story = {
  render: rail(withSignatureCompany("COMPANY A CORPORATION")),
};

export const InRailDark: Story = { ...InRail, globals: { theme: "dark" } };

export const InRailPhone: Story = {
  render: rail(withSignatureCompany("COMPANY A CORPORATION"), "100%"),
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const LongNameInRail: Story = {
  render: rail(
    withSignatureCompany(
      "Nordlicht Logistik und Lagerhaltung Gesellschaft mit beschränkter Haftung",
    ),
  ),
};

export const LongNameInRailPhone: Story = {
  ...LongNameInRail,
  render: rail(
    withSignatureCompany(
      "Nordlicht Logistik und Lagerhaltung Gesellschaft mit beschränkter Haftung",
    ),
    "100%",
  ),
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
