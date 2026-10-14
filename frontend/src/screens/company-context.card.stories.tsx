// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { CompanyContextCard } from "./company-context";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// Two conditions, not one: a rollout flag says whether the surface exists, the
// company grant whether this reader may change it.
const PROFILE = {
  display_name: "Brandt Automotive GmbH",
  legal_name: "Brandt Automotive GmbH",
  website: "https://brandt-automotive.de",
  offer_summary: "Fleet retrofit programmes for mid-size logistics operators.",
  icp: "Operators running 50–400 vans on mixed-age fleets.",
  value_proposition:
    "Cuts downtime per vehicle by scheduling retrofits around depot windows.",
  usp: "The only provider that fits around an operator's own depot calendar.",
  customer_pains: "Vehicles off the road during peak weeks.",
  desired_outcomes: "Predictable retrofit slots and a fixed per-vehicle price.",
  buying_center: "Fleet manager decides; finance signs.",
  buying_intents: "Depot expansion, emissions deadlines.",
  common_objections: "Downtime risk, and whether the quote holds.",
  sales_motion: "Field, with a depot visit before the quote.",
  // The provenance sidecar, in the shape the server sends: one row per
  // confirmed statement, so a site-read value shows under its evidence mark.
  fields: [
    {
      field: "display_name",
      value: "Brandt Automotive GmbH",
      source: "human",
      captured_by: "human:u-7",
      updated_at: "2026-05-02T08:00:00Z",
    },
    {
      field: "offer_summary",
      value: "Fleet retrofit programmes for mid-size logistics operators.",
      source: "site_read",
      captured_by: "agent:site-read",
      evidence_snippet:
        "We run retrofit programmes for mid-size logistics operators.",
      source_url: "https://brandt-automotive.de/leistungen",
      confidence: 0.86,
      updated_at: "2026-08-01T09:00:00Z",
    },
    {
      field: "usp",
      value:
        "The only provider that fits around an operator's own depot calendar.",
      source: "site_read",
      captured_by: "agent:site-read",
      evidence_snippet: "We plan around your depot calendar, not ours.",
      source_url: "https://brandt-automotive.de/warum-wir",
      confidence: 0.62,
      updated_at: "2026-08-01T09:00:00Z",
    },
  ],
};

const CAPABILITIES = {
  read_enabled: true,
  write_enabled: true,
  rollout: "onboarding",
};

function story(
  capabilities: Record<string, unknown>,
  allow: Parameters<typeof meRoute>[0],
  company: Record<string, unknown> | null = PROFILE,
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /company/context/capabilities": () => jsonResponse(capabilities),
      "GET /company": () =>
        company ? jsonResponse(company) : jsonResponse({}, 404),
    });
    return (
      <StoryProviders>
        <CompanyContextCard />
      </StoryProviders>
    );
  };
}

const EDITOR = { company: ["read", "update"] } as const;
const READER = { company: ["read"] } as const;

const meta = {
  title: "Settings/Company/Company profile/Company profile",
  component: CompanyContextCard,
} satisfies Meta<typeof CompanyContextCard>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Filled: Story = { render: story(CAPABILITIES, EDITOR) };

export const ReadOnly: Story = { render: story(CAPABILITIES, READER) };

// The one cause that justifies the surface being absent rather than withheld.
export const NotEnabledHere: Story = {
  render: story({ ...CAPABILITIES, read_enabled: false }, EDITOR),
};

// Look at the logo tiles, group counts and dotted evidence marks.
export const FilledDark: Story = {
  globals: { theme: "dark" },
  render: story(CAPABILITIES, EDITOR),
};

// The website value is a full URL with no space to break at.
export const FilledPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(CAPABILITIES, EDITOR),
};

// Focus lands on the fact whose row was pressed.
export const EditingEssentials: Story = {
  render: story(CAPABILITIES, EDITOR),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Edit Products and services" }),
    );
  },
};

export const Sparse: Story = {
  render: story(CAPABILITIES, EDITOR, {
    display_name: "Brandt Automotive GmbH",
    offer_summary:
      "Fleet retrofit programmes for mid-size logistics operators.",
    icp: "Operators running 50–400 vans on mixed-age fleets.",
    fields: [],
  }),
};
