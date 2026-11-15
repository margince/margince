// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import { UTC_ZONE } from "../../format/timezone";
import { LocaleProvider, translate } from "../../i18n";
import { leadColumns } from "./leads.columns";

const meta: Meta = {
  title: "Records/Leads/Table names",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type Lead = components["schemas"]["Lead"];

const leads: Lead[] = [
  { id: "named", full_name: "Vera Lindqvist", email: "vera@example.test" },
  { id: "email", full_name: " ", email: "  buyer@example.test  " },
  { id: "unnamed", full_name: null, email: null },
].map<Lead>((identity) => ({
  ...identity,
  status: "new",
  score: 0,
  source: "manual",
  captured_by: "human:demo",
  version: 1,
  created_at: "2026-10-01T00:00:00Z",
  updated_at: "2026-10-01T00:00:00Z",
}));

export const NameFallbacks: Story = {
  render: () => {
    const name = leadColumns(
      (key) => translate("en", key),
      "en",
      UTC_ZONE,
      [],
    ).find((column) => column.key === "name");
    return (
      <LocaleProvider initial="en">
        {leads.map((lead) => (
          <div key={lead.id}>{name?.cell(lead)}</div>
        ))}
      </LocaleProvider>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Vera Lindqvist")).toBeVisible();
    await expect(canvas.getByText("buyer@example.test")).toBeVisible();
    await expect(canvas.getByText("Unnamed lead")).toBeVisible();
  },
};
