// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { Locale } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { LeadsScreen } from "./leads.list";

// The lead list narrowed by the linked company's industry, country and size,
// and the same address opened by a seat refused those dials.

const meta: Meta = {
  title: "Records/Leads/Company filters",
  parameters: { layout: "padded" },
  beforeEach: () => () => globalThis.history.replaceState(null, "", "#/"),
};
export default meta;

type Story = StoryObj;

const lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  company_name: "Nordwind Logistik",
  company_id: "c-7",
  status: "contacted",
  score: 72,
  source: "manual",
  captured_by: "human:u1",
  version: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const ADDRESS =
  "#/leads?industry=logistics_transport&country=DE&size_band=51-200";

function list(refused: string[] | undefined, locale: Locale = "en") {
  return () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse(
          meFixture({
            roles: ["manager"],
            rowScope: "all",
            allow: { lead: ["read", "update"], company: ["read"] },
            refusedLeadFilters: refused,
          }),
        ),
      "GET /leads": () =>
        jsonResponse({
          data: [lead],
          page: { next_cursor: null, has_more: false },
        }),
    });
    globalThis.history.replaceState(null, "", ADDRESS);
    return (
      <StoryProviders locale={locale}>
        <LeadsScreen />
      </StoryProviders>
    );
  };
}

/** All three dials applied, each named in the reader's language. */
export const Applied: Story = {
  render: list(undefined),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("group", {
        name: "Industry: Logistics and Transport",
      }),
    ).toBeTruthy();
  },
};

export const AppliedGerman: Story = { render: list(undefined, "de") };

export const AppliedVietnamese: Story = { render: list(undefined, "vi") };

/** A seat refused the dials opens the same address: the list names them. */
export const Refused: Story = {
  render: list(["industry", "country", "size_band"]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("button", {
        name: "Remove Industry, Country, Company size",
      }),
    ).toBeTruthy();
  },
};

export const RefusedGerman: Story = {
  render: list(["industry", "country", "size_band"], "de"),
};

export const RefusedPhone: Story = {
  tags: ["uat-phone"],
  render: list(["industry", "country", "size_band"]),
};
