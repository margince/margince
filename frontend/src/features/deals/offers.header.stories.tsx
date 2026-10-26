// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import {
  emptyPage,
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import type { Offer } from "./offers";
import { EditOfferHeaderModal } from "./offers.header";

const meta: Meta = {
  title: "Records/Offers/Header form",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

const draftOffer: Offer = {
  id: "o-1",
  deal_id: "d-1",
  offer_number: "ANG-2026-0007",
  revision: 1,
  status: "draft",
  currency: "EUR",
  buyer_company_id: null,
  valid_until: "2026-08-01",
  intro_text: null,
  terms_text: null,
  net_minor: 0,
  tax_minor: 0,
  gross_minor: 0,
  template_id: null,
  line_items: [],
  source: "manual",
  ai_generated: false,
  captured_by: "human:u1",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

export const Open: Story = {
  render: () => {
    installFetchStub({
      "GET /offer-templates": () => jsonResponse(emptyPage),
    });
    return (
      <StoryProviders>
        <EditOfferHeaderModal open onClose={() => {}} offer={draftOffer} />
      </StoryProviders>
    );
  },
  play: async () => {
    await screen.findByRole("textbox", { name: "Valid until" });
  },
};

/** The calendar behind "Valid until", open on the day the offer runs to. */
export const PickingValidUntil: Story = {
  ...Open,
  play: async () => {
    await screen.findByRole("textbox", { name: "Valid until" });
    await userEvent.click(screen.getByRole("button", { name: /^Choose date/ }));
    await screen.findByRole("dialog", { name: /^Choose date/ });
  },
};
