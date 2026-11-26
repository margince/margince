// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { GrowthFitInputs } from "./companygrowthfitinputs";

// The checklist under the growth fit: each input with its value or its gap.

const meta: Meta = {
  title: "Records/Company 360/Growth fit inputs",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type Company = components["schemas"]["Company"];
type DataCompleteness = components["schemas"]["DataCompleteness"];

const company: Company = {
  id: "o-1",
  writable: true,
  display_name: "Brandt Automotive GmbH",
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

const completeness: DataCompleteness = {
  present: 5,
  expected: 7,
  needed: 4,
  inputs: [
    { input: "offer_summary", present: true },
    { input: "icp", present: true },
    { input: "industry", present: true },
    { input: "buying_center", present: false },
    { input: "buying_intents", present: true },
    { input: "employee_range", present: false },
    { input: "technology", present: true },
  ],
  missing: ["buying_center", "employee_range"],
};

function Inputs({ industry }: Readonly<{ industry?: Company["industry"] }>) {
  installFetchStub({
    "GET /me": meRoute({ company: ["read", "update"] }),
    "GET /companies/o-1/profile-fields": () =>
      jsonResponse({
        data: [
          {
            field: "offer_summary",
            value: "Electric delivery vans and fleet charging",
            source: "human",
            version: 1,
            updated_at: "2026-07-01T09:00:00Z",
          },
          {
            field: "industry",
            value: "Fleet logistics",
            source: "site_read",
            version: 1,
            updated_at: "2026-07-01T09:00:00Z",
          },
        ],
      }),
  });
  return (
    <StoryProviders>
      <GrowthFitInputs
        company={{ ...company, industry }}
        completeness={completeness}
      />
    </StoryProviders>
  );
}

// No industry of its own: the row shows the one the site read, which the fit counts.
export const IndustryFromTheSite: Story = {
  render: () => <Inputs />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText(/Fleet logistics/)).toBeVisible();
    await expect(canvas.getAllByText("Missing")).toHaveLength(2);
  },
};

// The record's own industry, named from the list.
export const IndustryOfItsOwn: Story = {
  render: () => <Inputs industry="automotive" />,
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByText("Automotive"),
    ).toBeVisible();
  },
};
