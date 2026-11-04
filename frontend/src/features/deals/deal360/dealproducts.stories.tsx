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

function card(rows: DealProduct[] | "denied") {
  installFetchStub({
    "GET /deals/d-1/products": () =>
      rows === "denied" ? permissionDenied() : jsonResponse({ data: rows }),
    "GET /products": () =>
      jsonResponse({
        data: [{ id: "p-support", name: "Support plan" }],
        next_cursor: null,
        has_more: false,
      }),
    "PUT /deals/d-1/products": () => jsonResponse({ data: BY_HAND }),
  });
  return (
    <StoryProviders>
      <DealProductsCard dealId="d-1" currency="EUR" />
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
async function openEditor(canvasElement: HTMLElement) {
  await userEvent.click(
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
    const editor = await openEditor(canvasElement);
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
    const editor = await openEditor(canvasElement);
    await expect(
      within(editor).getByText(/If you save an empty list/),
    ).toBeTruthy();
  },
};
