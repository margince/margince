// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
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

function renderCompanies(rows: readonly (typeof company)[]) {
  globalThis.location.hash = "#/companies";
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
      <CompaniesScreen />
    </StoryProviders>
  );
}

export const CompaniesList: Story = {
  render: () => renderCompanies([company]),
};

const longNames = [
  "Brandt Automotive Fleet Services International Holdings Alpha",
  "Brandt Automotive Fleet Services International Holdings Bravo",
  "Brandt Automotive Fleet Services International Holdings Charlie",
].map((display_name, i) => ({
  ...company,
  id: `o-long-${i}`,
  display_name,
  archived_at: i === 2 ? "2026-06-20T09:00:00Z" : undefined,
}));

export const LongNames: Story = {
  name: "long names end in an ellipsis",
  render: () => renderCompanies(longNames),
};

export const LongNamesPhone: Story = {
  name: "long names wrap in the card — on a phone",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => renderCompanies(longNames),
};
