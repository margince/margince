// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, within } from "storybook/test";
import { phoneRow } from "../../testing/phonerow";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { CompaniesScreen } from "./companies";

const meta: Meta = {
  title: "Records/Companies",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const company = {
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  industry: "Automotive",
  size_band: "201-500",
  domains: [{ domain: "brandt.example", is_primary: true }],
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

function renderCompanies(
  rows: readonly (typeof company)[],
  startCreating = false,
) {
  globalThis.location.hash = startCreating ? "#/companies/new" : "#/companies";
  installFetchStub({
    "GET /me": meRoute({ company: ["read", "update"] }),
    "GET /companies": () =>
      jsonResponse({
        data: rows,
        page: { next_cursor: null, has_more: false },
      }),
  });
  return (
    <StoryProviders>
      <CompaniesScreen startCreating={startCreating} />
    </StoryProviders>
  );
}

export const CompaniesList: Story = {
  render: () => renderCompanies([company]),
};

/** `#/companies/new`: the list with its create form open, as `C` opens it. */
export const CreateFormOpen: Story = {
  render: () => renderCompanies([company], true),
  play: async () => {
    await expect(
      await screen.findByRole("dialog", { name: "New company" }),
    ).toBeInTheDocument();
  },
};

const LONG_NAMES = [
  "Brandt Automotive Fleet Services International Holdings Alpha",
  "Brandt Automotive Fleet Services International Holdings Bravo",
  "Brandt Automotive Fleet Services International Holdings Charlie",
] as const;

const longNames = LONG_NAMES.map((display_name, i) => ({
  ...company,
  id: `o-long-${i}`,
  display_name,
  lifecycle: i === 1 ? "unknown" : "customer",
  website_url: i === 2 ? null : `https://www.brandt-${i}.example/fleet`,
  archived_at: i === 2 ? "2026-06-20T09:00:00Z" : undefined,
}));

export const LongNames: Story = {
  name: "long names end in an ellipsis",
  render: () => renderCompanies(longNames),
};

// Line one ends in the lifecycle, line two is the website's host; a fact the
// company lacks is left out.
export const LongNamesPhone: Story = {
  name: "long names wrap in the card — on a phone",
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => renderCompanies(longNames),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const [alpha, bravo, charlie] = LONG_NAMES;
    const rowOf = async (name: string) =>
      phoneRow(await canvas.findByText(name));
    const first = await rowOf(alpha);
    const second = await rowOf(bravo);
    const third = await rowOf(charlie);
    await expect([first.end, first.caption]).toEqual([
      ["Customer"],
      ["brandt-0.example"],
    ]);
    await expect([second.end, second.caption]).toEqual([
      [],
      ["brandt-1.example"],
    ]);
    await expect([third.end, third.caption]).toEqual([["Customer"], []]);
    await expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
      document.documentElement.clientWidth,
    );
  },
};
