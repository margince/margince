// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { CompanyTable } from "./analytics.company";

// The company report's table on its own. Each count opens that company's open
// deals only on a complete answer; a capped one draws every count plainly.
const meta: Meta = { title: "Records/Reports/Company table" };
export default meta;

type Story = StoryObj;

const rows = [
  { company_id: "o-1", raw_minor: 2_500_000, deal_count: 4, currency: "EUR" },
  { company_id: "o-2", raw_minor: 870_000, deal_count: 2, currency: "EUR" },
  {
    company_id: "o-2",
    raw_minor: 4_500_000_000,
    deal_count: 1,
    currency: "VND",
  },
  { company_id: null, raw_minor: 120_000, deal_count: 3, currency: "EUR" },
];

function table(complete: boolean) {
  installFetchStub({
    "GET /companies/o-1": () =>
      jsonResponse({ id: "o-1", display_name: "BÄR Pharma GmbH" }),
    "GET /companies/o-2": () =>
      jsonResponse({ id: "o-2", display_name: "Brandt Systemtechnik" }),
  });
  return (
    <StoryProviders>
      <CompanyTable rows={rows} complete={complete} locale="en" />
    </StoryProviders>
  );
}

export const Complete: Story = { render: () => table(true) };

export const Capped: Story = {
  render: () => table(false),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("4");
    await expect(canvas.queryByRole("link", { name: "4" })).toBeNull();
  },
};

export const CappedDark: Story = { ...Capped, globals: { theme: "dark" } };
