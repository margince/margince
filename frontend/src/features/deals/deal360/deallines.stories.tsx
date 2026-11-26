// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../../testing/story-utils";
import { DealLinesEditor } from "./deallines";
import type { DealCommercial } from "./deallinestotal";

const meta: Meta = {
  title: "Records/Deal 360/Line editor",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const AT = "2026-10-01T09:00:00Z";

const PRICED: DealCommercial = {
  deal_id: "d-1",
  mode: "lines",
  amount_basis: "lines_net_committed",
  commercial_revision: 1,
  currency: "EUR",
  complete: true,
  missing: [],
  money_withheld: false,
  writable: true,
  committed_net_minor: 20_000,
  known_committed_net_minor: 20_000,
  arr_minor: 0,
  lines: [
    {
      id: "l-1",
      position: 1,
      product_id: "p-banana",
      catalog_name: "Bananas",
      description: "Bananas",
      unit: "piece",
      quantity: 100,
      unit_price_minor: 200,
      currency: "EUR",
      discount_pct: 0,
      billing_model: "one_time",
      line_net_minor: 20_000,
      committed_net_minor: 20_000,
      missing: [],
      created_at: AT,
      version: 1,
      updated_at: AT,
    },
  ],
};

const ESTIMATE: DealCommercial = {
  ...PRICED,
  mode: "estimate",
  amount_basis: "estimate",
  commercial_revision: 0,
  lines: [],
  committed_net_minor: null,
  known_committed_net_minor: null,
};

function editor(state: DealCommercial, typedAmount: string | null) {
  installFetchStub({
    "GET /products": () =>
      jsonResponse({
        data: [{ id: "p-support", name: "Support plan" }],
        next_cursor: null,
        has_more: false,
      }),
    "POST /deals/d-1/lines": () => jsonResponse(PRICED),
  });
  return (
    <StoryProviders>
      <DealLinesEditor
        dealId="d-1"
        state={state}
        typedAmount={typedAmount}
        onClose={() => undefined}
      />
    </StoryProviders>
  );
}

const opens = async () => {
  await screen.findByRole("dialog", { name: "Products and services" });
};

/** The deal's own lines, open for a change. */
export const Lines: Story = {
  render: () => editor(PRICED, null),
  play: opens,
};

export const LinesDark: Story = {
  globals: { theme: "dark" },
  render: () => editor(PRICED, null),
  play: opens,
};

/** The first line replaces a value someone typed, and the dialog says so. */
export const ReplacesTypedValue: Story = {
  render: () => editor(ESTIMATE, "€1,000.00"),
  play: opens,
};

/** Enter in the custom item field keeps the dialog open with the text typed. */
export const EnterInCustomItem: Story = {
  render: () => editor(PRICED, null),
  play: async () => {
    const dialog = within(
      await screen.findByRole("dialog", { name: "Products and services" }),
    );
    const custom = dialog.getByRole("textbox", { name: "Custom item" });
    await userEvent.type(custom, "Ripening{Enter}");
    await expect(
      screen.getByRole("dialog", { name: "Products and services" }),
    ).toBeVisible();
    await expect(custom).toHaveValue("Ripening");
  },
};
