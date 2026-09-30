// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { CompaniesScreen } from "./companies";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

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

export const CompaniesList: Story = {
  render: () => {
    globalThis.location.hash = "#/companies";
    installFetchStub({
      "GET /me": meRoute({ company: ["read", "update"] }),
      "GET /companies": () =>
        jsonResponse({
          data: [company],
          page: { next_cursor: null, has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <CompaniesScreen />
      </StoryProviders>
    );
  },
};
