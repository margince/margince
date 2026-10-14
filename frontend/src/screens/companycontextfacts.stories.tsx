// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { CompanyFactsCard } from "./companycontextfacts";
import { StoryProviders } from "./story-utils";

type CompanyProfile = components["schemas"]["CompanyProfile"];
type CompanyInput = components["schemas"]["CompanyProfileInput"];

const SOME_SET: CompanyInput = {
  display_name: "Brandt Automotive GmbH",
  website: "brandt-automotive.de",
  offer_summary: "Fleet retrofit programmes for mid-size logistics operators.",
  icp: "Operators running 50–400 vans on mixed-age fleets.",
  value_proposition:
    "Cuts downtime per vehicle by scheduling retrofits around depot windows.",
  usp: "The only provider that fits around an operator's own depot calendar.",
  legal_name: "Brandt Automotive GmbH",
};

const ALL_SET: CompanyInput = {
  ...SOME_SET,
  customer_pains: "Vehicles off the road during peak weeks.",
  desired_outcomes: "Predictable retrofit slots and a fixed per-vehicle price.",
  buying_center: "Fleet manager decides; finance signs.",
  buying_intents: "Depot expansion, emissions deadlines.",
  common_objections: "Downtime risk, and whether the quote holds.",
  sales_motion: "Field, with a depot visit before the quote.",
  registered_address: "Werkstraße 4, 70435 Stuttgart",
  register_vat: "HRB 734512 · DE811234567",
  industry: "Automotive services",
  history:
    "Founded 1998 as a single workshop in Stuttgart, retrofitting delivery vans for local bakeries. Moved into fleet programmes in 2012, and has run depot-side retrofits for logistics operators across the DACH region since 2019.",
};

function profileOf(form: CompanyInput): CompanyProfile {
  return {
    company_id: "00000000-0000-4000-8000-000000000010",
    ...form,
    fields: [
      {
        field: "offer_summary",
        value: form.offer_summary ?? "",
        source: "site_read",
        captured_by: "agent:site-read",
        evidence_snippet:
          "We run retrofit programmes for mid-size logistics operators.",
        source_url: "https://brandt-automotive.de/leistungen",
        confidence: 0.86,
        updated_at: "2026-08-01T09:00:00Z",
      },
    ],
  };
}

function loaded(form: CompanyInput) {
  return {
    isPending: false,
    isError: false,
    error: null,
    data: profileOf(form),
    refetch: () => undefined,
  };
}

const meta = {
  title: "Settings/Company/Company profile/Business context",
  component: CompanyFactsCard,
  parameters: { layout: "padded" },
  args: {
    company: loaded(SOME_SET),
    form: SOME_SET,
    canEdit: true,
    readOnly: false,
    saved: false,
    onEdit: () => undefined,
  },
  render: (args) => (
    <StoryProviders>
      <CompanyFactsCard {...args} />
    </StoryProviders>
  ),
} satisfies Meta<typeof CompanyFactsCard>;
export default meta;

type Story = StoryObj<typeof meta>;

export const SomeSet: Story = {};

export const AllSet: Story = {
  args: { company: loaded(ALL_SET), form: ALL_SET },
};

// A long answer reads as its first two lines.
export const GroupOpen: Story = {
  args: { company: loaded(ALL_SET), form: ALL_SET },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByText("Identity and legal details"),
    );
    await expect(
      await canvas.findByRole("button", { name: "Edit Company history" }),
    ).toBeVisible();
  },
};

export const SomeSetDark: Story = {
  globals: { theme: "dark" },
};

export const SomeSetPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const ReadOnly: Story = {
  args: { canEdit: false, readOnly: true },
};
