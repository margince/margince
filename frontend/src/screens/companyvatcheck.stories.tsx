// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { VatCheckCard } from "./companyvatcheck";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The four states worth reviewing are the four answers a reader can get, and
// they differ in what they let a business CLAIM rather than in how they look:
// a valid number with a receipt is evidence, the same number without one is
// only a reading, an invalid one is a finding, and a register that did not
// answer says nothing about the company at all.

const meta: Meta<typeof VatCheckCard> = {
  title: "Records/Company 360/VAT check",
  component: VatCheckCard,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof VatCheckCard>;
type VatCheck = components["schemas"]["OrganizationVatCheck"];

const ORG_ID = "00000000-0000-7000-8000-0000000000a1";
const ROUTE = `GET /organizations/${ORG_ID}/vat-check`;
const ASK = `POST /organizations/${ORG_ID}/vat-check`;

// The grant the ask button gates on. Without it every story renders as a viewer
// who may not write, and the button is absent for the correct reason — which is
// exactly the omission story-utils' own comment warns is invisible.
const CAN_WRITE = meRoute({ organization: ["read", "update"] });

const CHECKED: VatCheck = {
  organization_id: ORG_ID,
  vat_number: "DE811907980",
  status: "valid",
  consultation_number: "WAPIAAAAXk3rN2p9",
  registered_name: "Muster Handels GmbH",
  registered_address: "Musterstraße 1, 10115 Berlin",
  checked_at: "2026-08-14T09:12:00Z",
};

function card() {
  return (
    <StoryProviders locale="de">
      <VatCheckCard orgId={ORG_ID} />
    </StoryProviders>
  );
}

/** The whole point of the feature: a verdict WITH the receipt that makes it
 * evidence a tax authority accepts. */
export const ValidWithReceipt: Story = {
  render: () => {
    installFetchStub({
      "GET /me": CAN_WRITE,
      [ROUTE]: () => jsonResponse(CHECKED),
      [ASK]: () => new Response(null, { status: 202 }),
    });
    return card();
  },
};

/** The same answer from an installation that has not recorded its own VAT ID.
 * The check ran; the register issued no receipt, so this proves nothing to a
 * tax authority and the card has to say which of the two it is. */
export const ValidWithoutReceipt: Story = {
  render: () => {
    installFetchStub({
      "GET /me": CAN_WRITE,
      [ROUTE]: () =>
        jsonResponse({
          organization_id: ORG_ID,
          vat_number: CHECKED.vat_number,
          status: "valid",
          registered_name: CHECKED.registered_name,
          checked_at: CHECKED.checked_at,
        }),
    });
    return card();
  },
};

/** A number the register rejects. The most common cause is an imprint copied
 * from another company's site, which is why the registered name matters more
 * here than the verdict. */
export const Invalid: Story = {
  render: () => {
    installFetchStub({
      "GET /me": CAN_WRITE,
      [ROUTE]: () =>
        jsonResponse({
          organization_id: ORG_ID,
          vat_number: "DE999999999",
          status: "invalid",
          checked_at: CHECKED.checked_at,
        }),
    });
    return card();
  },
};

/** The register declined to answer. A fact about the lookup, never about the
 * company — so it is stated without a warning tone. */
export const RegisterSilent: Story = {
  render: () => {
    installFetchStub({
      "GET /me": CAN_WRITE,
      [ROUTE]: () =>
        jsonResponse({
          organization_id: ORG_ID,
          vat_number: CHECKED.vat_number,
          status: "unavailable",
          checked_at: CHECKED.checked_at,
        }),
    });
    return card();
  },
};

/** Never consulted. The server answers 404, and the card offers the honest
 * absence rather than reporting a failure nobody caused. */
export const NeverConsulted: Story = {
  render: () => {
    installFetchStub({
      "GET /me": CAN_WRITE,
      [ROUTE]: () => jsonResponse({ title: "not found" }, 404),
      [ASK]: () => new Response(null, { status: 202 }),
    });
    return card();
  },
};

/** Asked again too soon. The floor exists because the register is a shared
 * public service consulted on one worker, and the reader is told to wait rather
 * than that something is wrong — the answer on the card still stands. */
export const AskedTooSoon: Story = {
  render: () => {
    installFetchStub({
      "GET /me": CAN_WRITE,
      [ROUTE]: () => jsonResponse(CHECKED),
      [ASK]: () =>
        jsonResponse(
          {
            type: "about:blank",
            title: "Too Many Requests",
            status: 429,
            detail: "Diese Nummer wurde vor weniger als 5 Minuten abgefragt.",
          },
          429,
        ),
    });
    return card();
  },
};

/** A viewer who may read the company and not change it. The verdict is theirs
 * to read; consulting the register is not — withholding the ask is not
 * withholding the record. */
export const WithoutTheGrantToAsk: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ organization: ["read"] }),
      [ROUTE]: () => jsonResponse(CHECKED),
    });
    return card();
  },
};
