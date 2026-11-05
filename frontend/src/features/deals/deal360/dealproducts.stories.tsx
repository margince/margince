// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  permissionDenied,
  StoryProviders,
} from "../../../testing/story-utils";
import { DealProductsCard } from "./dealproducts";

// The products a deal sells: from the accepted offer, set by hand, none yet,
// and hidden from a reader who may not see them. Then the editor that sets them.

const meta: Meta = {
  title: "Records/Deal 360/Products",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type DealProduct = components["schemas"]["DealProduct"];

const AT = "2026-10-01T09:00:00Z";

const FROM_OFFER: DealProduct[] = [
  {
    product_id: "p-erp",
    product_name: "ERP licence",
    source: "offer",
    amount_minor: 1_200_000,
    currency: "EUR",
    component_kind: "software",
    updated_at: AT,
  },
  {
    product_id: "p-impl",
    product_name: "Implementation",
    source: "offer",
    amount_minor: 450_000,
    currency: "EUR",
    component_kind: "implementation",
    updated_at: AT,
  },
];

const BY_HAND: DealProduct[] = [
  {
    product_id: "p-erp",
    product_name: "ERP licence",
    source: "manual",
    amount_minor: null,
    currency: null,
    component_kind: "software",
    updated_at: AT,
  },
];

type DealCommercial = components["schemas"]["DealCommercial"];

const PRICED_BY_LINES: DealCommercial = {
  deal_id: "d-1",
  mode: "lines",
  amount_basis: "lines_net_committed",
  commercial_revision: 2,
  currency: "EUR",
  complete: false,
  missing: ["unit_price"],
  money_withheld: false,
  writable: true,
  known_committed_net_minor: 20_000,
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
    {
      id: "l-2",
      position: 2,
      description: "Ripening service",
      unit: "piece",
      quantity: 1,
      currency: "EUR",
      discount_pct: 0,
      billing_model: "one_time",
      missing: ["unit_price"],
      created_at: AT,
      version: 1,
      updated_at: AT,
    },
  ],
};

function card(
  rows: DealProduct[] | "denied",
  lines: DealCommercial | null = null,
) {
  installFetchStub({
    "GET /deals/d-1/lines": () =>
      jsonResponse(
        lines ?? {
          ...PRICED_BY_LINES,
          mode: "estimate",
          commercial_revision: 0,
          lines: [],
        },
      ),
    "GET /deals/d-1/products": () =>
      rows === "denied"
        ? permissionDenied()
        : jsonResponse({ data: rows, tag: "story" }),
    "GET /products": () =>
      jsonResponse({
        data: [{ id: "p-support", name: "Support plan" }],
        next_cursor: null,
        has_more: false,
      }),
    "PUT /deals/d-1/products": () =>
      jsonResponse({ data: BY_HAND, tag: "story" }),
  });
  return (
    <StoryProviders>
      <DealProductsCard
        dealId="d-1"
        currency="EUR"
        typedAmount={lines ? null : "€16,500.00"}
      />
    </StoryProviders>
  );
}

/** Filled from the accepted offer, with the value allocated to each. */
export const FromTheOffer: Story = { render: () => card(FROM_OFFER) };

/** Set by hand, one product named without an amount. */
export const SetByHand: Story = { render: () => card(BY_HAND) };

/** No accepted offer and nothing set yet. */
export const NoneYet: Story = { render: () => card([]) };

/** A reader who may not see the products is told so. */
export const Denied: Story = { render: () => card("denied") };

export const FromTheOfferDark: Story = {
  globals: { theme: "dark" },
  render: () => card(FROM_OFFER),
};

// The editor opens in a dialog portalled to the body.
async function openEditor(
  canvasElement: HTMLElement,
  user: ReturnType<typeof userEvent.setup>,
) {
  await user.click(
    await within(canvasElement).findByRole("button", { name: "Set products" }),
  );
  return within(canvasElement.ownerDocument.body).findByRole("dialog", {
    name: "Set the products this deal sells",
  });
}

/** Editing the offer's products says saving replaces them. */
export const EditingTheOffer: Story = {
  render: () => card(FROM_OFFER),
  play: async ({ canvasElement }) => {
    const editor = await openEditor(canvasElement, userEvent.setup());
    await expect(
      within(editor).getByText(/The accepted offer set these products/),
    ).toBeTruthy();
    await expect(
      within(editor).getByRole("spinbutton", {
        name: "Amount for ERP licence",
      }),
    ).toBeTruthy();
  },
};

/** An emptied list says the accepted offer fills it again. */
export const EditingAnEmptyList: Story = {
  render: () => card([]),
  play: async ({ canvasElement }) => {
    const editor = await openEditor(canvasElement, userEvent.setup());
    await expect(within(editor).getByText(/Saving an empty list/)).toBeTruthy();
  },
};

/** Priced from its own lines, one of them still without a price. */
export const PricedByLines: Story = {
  render: () => card([], PRICED_BY_LINES),
};

export const PricedByLinesDark: Story = {
  globals: { theme: "dark" },
  render: () => card([], PRICED_BY_LINES),
};

/** The line editor, with its fields stacked on a phone. */
export const EditingTheLines: Story = {
  parameters: { viewport: { defaultViewport: "mobile1" } },
  render: () => card([], PRICED_BY_LINES),
  play: async ({ canvasElement }) => {
    await userEvent.setup().click(
      await within(canvasElement).findByRole("button", {
        name: "Edit products",
      }),
    );
    await expect(
      await within(canvasElement.ownerDocument.body).findByRole("dialog", {
        name: "Products and services",
      }),
    ).toBeTruthy();
  },
};
