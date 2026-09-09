// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { CompanyRejectAction } from "./companyreject";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";

// The states worth reviewing are what a reader is OFFERED, because that is what
// the first attempt at this verb got wrong. It differs by authority and by the
// record — a seat holding only half the write saw a control whose second call
// was always going to be refused — so the stories are grants and records rather
// than visual variants of one button.
//
// The dialog is the other half. It is the last thing a reader sees before a
// record is archived and a domain is refused for good, so it has to say both
// halves plainly and ask for the sentence that makes the refusal reviewable.

const meta: Meta<typeof CompanyRejectAction> = {
  title: "Records/Company header/Not a company",
  component: CompanyRejectAction,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof CompanyRejectAction>;
type Company = components["schemas"]["Company"];

// Both halves of the write. The archive is `company:delete` and the
// standing domain decision is `company:update`, and the control asks for
// the pair before it draws.
const CAN_REJECT = meRoute({ company: ["read", "update", "delete"] });

const ORG: Company = {
  writable: true,
  id: "00000000-0000-7000-8000-0000000000c1",
  display_name: "Expensify Ltd",
  owner_id: "00000000-0000-7000-8000-0000000000u1",
  captured_by: "connector:gmail",
  source: "capture",
  version: 4,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-30T08:00:00Z",
  domains: [
    {
      id: "00000000-0000-7000-8000-0000000000d1",
      domain: "expensify.test",
      is_primary: true,
      source: "capture",
      captured_by: "connector:gmail",
    },
  ],
};

function inMenu(company: Company, me: ReturnType<typeof meRoute>) {
  installFetchStub({ "GET /me": me });
  return (
    <StoryProviders>
      <div style={{ display: "flex", gap: "var(--space-2)", maxWidth: 340 }}>
        <CompanyRejectAction company={company} />
      </div>
    </StoryProviders>
  );
}

/** The offer, on a company capture minted from mail. Press it for the dialog:
 * it names the domain the refusal will cover and asks why, because the refusal
 * outlives the record and somebody reviewing the blocked-domain list months
 * later has only that sentence. */
export const Offered: Story = {
  render: () => inMenu(ORG, CAN_REJECT),
};

/** A seat holding the archive and not the standing domain decision. Nothing is
 * drawn — this is the case that used to draw a control, block the domain, and
 * take a 403 on the archive. Absent rather than disabled: STATE-4a sorts by
 * cause, and a control the reader has no authority for reports no fact about
 * this account. */
export const WithoutBothGrants: Story = {
  render: () => inMenu(ORG, meRoute({ company: ["read", "delete"] })),
};

/** A company somebody typed in by hand. It was never derived from mail, so
 * there is no domain to refuse and nothing a refusal would stop — Archive is
 * the verb for this record, and it is next door in the same menu. */
export const NoDomainToRefuse: Story = {
  render: () => inMenu({ ...ORG, domains: [] }, CAN_REJECT),
};
