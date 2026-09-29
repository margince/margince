// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { useT } from "../i18n";
import type { Employment } from "./contactemployers";
import { EmploymentLogo } from "./contactemployersummary";
import { StoryProviders } from "./story-utils";
import "./contact360.css";

type Company = components["schemas"]["Company"];

// An employment row's mark in its three readings: the company's own logo, the
// monogram keyed on the company's id when no logo resolved, and the empty slot
// a second row for the same employer keeps so the names still line up.
const meta: Meta = {
  title: "Records/Contact record/Employment logo",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj;

const employment: Employment = {
  relationship_id: "held",
  company_id: "o-1",
  company_name: "Brandt Automotive GmbH",
  role: "Head of Procurement",
  is_current_primary: true,
  employment_status: "current",
};

const company: Company = {
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  captured_by: "human:u-1",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

const LOGO =
  "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'%3E%3Crect x='8' y='8' width='48' height='48' rx='10' fill='%230b7a53'/%3E%3C/svg%3E";

function EmploymentLogoReadings() {
  const t = useT();
  return (
    <div style={{ display: "flex", gap: "var(--space-4)" }}>
      <EmploymentLogo
        employment={employment}
        company={{ ...company, logo_url: LOGO }}
        show
        t={t}
      />
      <EmploymentLogo employment={employment} company={company} show t={t} />
      <EmploymentLogo
        employment={{ ...employment, company_id: "o-2", company_name: null }}
        company={null}
        show
        t={t}
      />
      <EmploymentLogo
        employment={employment}
        company={company}
        show={false}
        t={t}
      />
    </div>
  );
}

export const Readings: Story = {
  render: () => <EmploymentLogoReadings />,
};
