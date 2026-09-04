// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import {
  ContractCancelModal,
  ContractRenewModal,
  ContractStatusModal,
} from "./contractlifecycle";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// margince#3286: the three transitions a signed agreement goes through after
// it is first recorded. The backend chain (Store.Renew / ChangeStatus /
// Cancel) predates this — these modals are the first frontend door onto it.

const meta: Meta = {
  title: "Records/Company 360/Contract lifecycle",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type Contract = components["schemas"]["Contract"];

const AGREEMENT = {
  id: "c-1",
  organization_id: "o-1",
  title: "Pallet pooling framework",
  contract_number: "SM-2026-014",
  status: "active",
  under_contract: true,
  auto_renew: false,
  value_basis: "annualized_12m",
  value_minor: 14850000,
  currency: "EUR",
  starts_on: "2026-11-01",
  version: 3,
  created_at: "2026-10-20T00:00:00Z",
  updated_at: "2026-10-20T00:00:00Z",
} as unknown as Contract;

function routes() {
  installFetchStub({
    "GET /me": meRoute({ contract: ["read", "create", "update"] }),
    "GET /installation/settings": () => jsonResponse({ base_currency: "EUR" }),
  });
}

/** Renewing an agreement: the successor's own terms, prefilled from the
 * predecessor's title and basis — nothing else, since a renewal is usually a
 * fresh negotiation. */
export const RenewOpen: Story = {
  render: () => {
    routes();
    return (
      <StoryProviders>
        <ContractRenewModal contract={AGREEMENT} open onClose={() => {}} />
      </StoryProviders>
    );
  },
};

/** Asserting a new status — draft, active, expired or cancelled. Superseded
 * is never offered here: a contract only arrives at it through renewal. */
export const StatusChangeOpen: Story = {
  render: () => {
    routes();
    return (
      <StoryProviders>
        <ContractStatusModal contract={AGREEMENT} open onClose={() => {}} />
      </StoryProviders>
    );
  },
};

/** Recording a cancellation: notice given and when it takes effect. The
 * status does not move — the customer stays under contract until the
 * effective date. */
export const CancelOpen: Story = {
  render: () => {
    routes();
    return (
      <StoryProviders>
        <ContractCancelModal contract={AGREEMENT} open onClose={() => {}} />
      </StoryProviders>
    );
  },
};
