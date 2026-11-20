// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import type { Offer } from "./offers";
import {
  AcceptOfferAction,
  RegenerateOfferAction,
  RejectOfferAction,
  SendOfferAction,
} from "./offers.actions";

const meta: Meta = {
  title: "Records/Offers/Lifecycle actions",
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
  net_minor: 1_200_000,
  tax_minor: 228_000,
  gross_minor: 1_428_000,
  template_id: null,
  line_items: [],
  source: "manual",
  ai_generated: false,
  captured_by: "human:u1",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

const sentOffer: Offer = { ...draftOffer, status: "sent", version: 2 };

function Actions({ offer }: Readonly<{ offer: Offer }>) {
  return (
    <StoryProviders>
      <div>
        {offer.status === "draft" ? (
          <SendOfferAction offer={offer} />
        ) : (
          <>
            <AcceptOfferAction offer={offer} />
            <RejectOfferAction offer={offer} />
            <RegenerateOfferAction offer={offer} />
          </>
        )}
      </div>
    </StoryProviders>
  );
}

const reader = { "GET /me": meRoute({ deal: ["read", "update"] }) };

export const Draft: Story = {
  render: () => {
    installFetchStub(reader);
    return <Actions offer={draftOffer} />;
  },
};

export const Sent: Story = {
  render: () => {
    installFetchStub(reader);
    return <Actions offer={sentOffer} />;
  },
};

/** A revision the server refuses keeps the reader on the sent offer, saying why. */
export const RegenerateRefused: Story = {
  render: () => {
    installFetchStub({
      ...reader,
      "POST /offers/o-1/regenerate": () =>
        jsonResponse(
          {
            status: 409,
            code: "conflict",
            title: "Conflict",
            detail: "A newer revision of this offer already exists.",
          },
          409,
        ),
    });
    return <Actions offer={sentOffer} />;
  },
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", { name: /Regenerate revision/ }),
    );
    await screen.findByRole("alert");
  },
};

/** A draft past its valid-until day says where to change it. */
export const SendLapsed: Story = {
  render: () => {
    installFetchStub({
      ...reader,
      "POST /offers/o-1/send": () =>
        jsonResponse(
          {
            status: 422,
            code: "validation_error",
            detail: "the offer was valid until 2026-11-15",
            details: {
              errors: [
                {
                  field: "valid_until",
                  code: "offer_lapsed",
                  message: "the offer was valid until 2026-11-15",
                },
              ],
            },
          },
          422,
        ),
    });
    return <Actions offer={draftOffer} />;
  },
  play: async () => {
    await userEvent.click(await screen.findByTestId("send-offer"));
    const dialog = within(await screen.findByRole("dialog"));
    await userEvent.click(dialog.getByRole("button", { name: "Send" }));
    await dialog.findByText(/^The Valid until date of this offer has passed\./);
  },
};

export const SentDark: Story = { ...Sent, globals: { theme: "dark" } };
