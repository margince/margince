// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../../api/schema";
import { en as englishCopy } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { LeadScreen } from "./leads";
import { SIGNAL_CATALOG } from "./leadsignals.fixture";

// The rows a lead changes through a picker of their own: project and product in About, tags in Registration.

const meta: Meta = {
  title: "Records/Leads/About flows",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type Lead = components["schemas"]["Lead"];

const tags: components["schemas"]["RecordTagsResponse"] = {
  data: [
    {
      tag_id: "t-1",
      name: "Fleet",
      archived: false,
      assigned_at: "2026-01-02T00:00:00Z",
    },
  ],
  withheld: false,
};

const lead: Lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  email: "jonas@nordwind.example",
  company_name: "Nordwind Logistik",
  status: "contacted",
  score: 72,
  product_id: "p-suite",
  source: "manual",
  captured_by: "human:u1",
  writable: true,
  version: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

function Page() {
  installFetchStub({
    "GET /me": meRoute(
      { lead: ["read", "update"], product: ["read"], tag: ["read", "create"] },
      { roles: ["rep"] },
    ),
    "GET /leads/l-1": () => jsonResponse(lead),
    "GET /leads/l-1/consent/stops": () => jsonResponse({ stops: [] }),
    "GET /lead-signal-catalog": () => jsonResponse(SIGNAL_CATALOG),
    "GET /products/p-suite": () =>
      jsonResponse({ id: "p-suite", name: "Suite" }),
    "GET /records/lead/l-1/tags": () => jsonResponse(tags),
  });
  return (
    <StoryProviders>
      <LeadScreen id="l-1" />
    </StoryProviders>
  );
}

async function openSection(name: string) {
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: `Edit ${name}` }));
  const dialog = within(await screen.findByRole("dialog", { name }));
  return { user, dialog };
}

// Registration reads the lead's tags, and Tags opens their own editor.
export const TagsFlow: Story = {
  render: () => <Page />,
  play: async () => {
    const { user, dialog } = await openSection("Registration");
    const reading = await dialog.findByText("Fleet");
    await waitFor(() => expect(reading).toBeVisible());
    await user.click(dialog.getByRole("button", { name: "Change Tags" }));
    const tags = await screen.findByRole("dialog", { name: "Tags" });
    await waitFor(() => expect(tags).toBeVisible());
  },
};

export const TagsFlowPhone: Story = { ...TagsFlow, tags: ["uat-phone"] };

// Product reads the product's name and opens the product search.
export const ProductFlow: Story = {
  render: () => <Page />,
  play: async () => {
    const { user, dialog } = await openSection("About");
    const reading = await dialog.findByText("Suite");
    await waitFor(() => expect(reading).toBeVisible());
    await user.click(dialog.getByRole("button", { name: "Change Product" }));
    const search = await screen.findByRole("dialog", {
      name: englishCopy["lead.product.change"],
    });
    await waitFor(() => expect(search).toBeVisible());
    await expect(screen.queryByRole("dialog", { name: "About" })).toBeNull();
  },
};
