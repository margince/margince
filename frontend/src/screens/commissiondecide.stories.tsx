// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { CommissionDecision } from "./commissiondecide";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// One verb on one ledger row, and the confirm step a money decision deserves.
//
// The copy is the point of these stories: MARGINCE PAYS NOBODY, and the
// dialog has to say so where the decision is made rather than in a doc. So
// every story below is driven past the trigger into the open dialog — a
// resting button pictures none of that.
//
// `Modal` portals to document.body, so `#storybook-root` holds the trigger
// and nothing else however well the dialog renders. Every query for something
// the dialog shows scopes to the document, and the play is what makes the
// difference visible: a rejecting play IS a failure the render gate reports.

type CommissionEntry = components["schemas"]["CommissionEntry"];

const ENTRY: CommissionEntry = {
  id: "c-1",
  deal_id: "d-1",
  partner_company_id: "o-1",
  status: "accrued",
  attribution_at_accrual: "sourced",
  margin_tier_at_accrual: "tier2_20",
  rate_bps: 2000,
  basis_amount_minor: 4_500_000,
  currency: "EUR",
  amount_minor: 900_000,
  captured_by: "human:u1",
  version: 3,
  created_at: "2026-08-01T00:00:00Z",
  updated_at: "2026-08-01T00:00:00Z",
};

// The decide POST is the only route this control reaches: it holds the entry
// it was handed and asks nobody for a session, so a story that routed /me
// would be describing a gate that lives on the row above (partnercommissions).
function decision(
  args: Readonly<{
    status: CommissionEntry["status"];
    decision: "approve" | "pay" | "void";
    decide: RouteMap[string];
  }>,
) {
  return () => {
    installFetchStub({ "POST /commissions/c-1/decide": args.decide });
    return (
      <StoryProviders>
        <CommissionDecision
          entry={{ ...ENTRY, status: args.status }}
          decision={args.decision}
          companyId="o-1"
        />
      </StoryProviders>
    );
  };
}

const accepted = (over: Partial<CommissionEntry>) => () =>
  jsonResponse({ ...ENTRY, ...over }, 200);

const meta: Meta<typeof CommissionDecision> = {
  title: "Records/Partner/Commission decision",
  component: CommissionDecision,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof CommissionDecision>;

// Opens the dialog the trigger owns and hands back the document, which is
// where the dialog actually lives.
async function openDialog(canvasElement: HTMLElement, verb: string) {
  const canvas = within(canvasElement);
  await userEvent.click(await canvas.findByRole("button", { name: verb }));
  const page = within(canvasElement.ownerDocument.body);
  await page.findByRole("dialog");
  return page;
}

// The form ready: an accrued entry's forward verb, and the sentence that says
// approving records an agreement and moves no money.
export const ApproveReady: Story = {
  render: decision({
    status: "accrued",
    decision: "approve",
    decide: accepted({ status: "approved", version: 4 }),
  }),
  play: async ({ canvasElement }) => {
    const page = await openDialog(canvasElement, "Approve");
    await page.findByText(/does not pay anything/);
  },
};

// A reversal asks WHY, and refuses to send without it — but only once the
// reader has pressed, never while they are still typing.
//
// The decide route is stubbed even though nothing should reach it, because the
// stub's unrouted fallback answers a POST with an empty page and a 200: were
// the reason gate to stop refusing, the write would read as accepted, the
// dialog would close, and the play below is what reports it.
export const ReverseNeedsReason: Story = {
  render: decision({
    status: "paid",
    decision: "void",
    decide: accepted({ status: "void" }),
  }),
  play: async ({ canvasElement }) => {
    const page = await openDialog(canvasElement, "Reverse");
    await userEvent.click(page.getByTestId("commission-void-confirm"));
    await page.findByText(/needs a reason/);
  },
};

// Fills the reason and presses, so a story reaches the outcome the server
// gives it rather than the reason gate above.
async function reverse(canvasElement: HTMLElement, reason: string) {
  const page = await openDialog(canvasElement, "Reverse");
  await userEvent.type(page.getByTestId("commission-void-reason"), reason);
  await userEvent.click(page.getByTestId("commission-void-confirm"));
  return page;
}

const refusedRender = decision({
  status: "paid",
  decision: "void",
  decide: () =>
    jsonResponse(
      {
        title: "Unprocessable Entity",
        detail:
          "this entry was already reversed, and a reversal is never reversed twice",
        status: 422,
      },
      422,
    ),
});

async function refusalOnScreen({
  canvasElement,
}: Readonly<{ canvasElement: HTMLElement }>) {
  const page = await reverse(canvasElement, "Buyer cancelled the order");
  await page.findByText(/never reversed twice/);
}

// Refused by the server, in the server's own words: the dialog STAYS open,
// and the alert is the only thing on it that changed, so it is drawn in
// danger ink at text contrast rather than the fill hue.
export const ReverseRefused: Story = {
  render: refusedRender,
  play: refusalOnScreen,
};

// The same refusal in dark, because the danger ink is a lifted derivation of
// the same token there: this sentence is the only thing standing between a
// reader and believing the reversal landed, so it has to carry at reading
// contrast against the dark dialog and not only against the light one.
export const ReverseRefusedDark: Story = {
  globals: { theme: "dark" },
  render: refusedRender,
  play: refusalOnScreen,
};

// Somebody else moved the row while the dialog was open. The 409 is not shown
// as the server's "version skew" wording — that names our concurrency
// mechanism, not the reader's problem — so the same alert says reload instead.
export const ReverseStale: Story = {
  render: decision({
    status: "approved",
    decision: "void",
    decide: () =>
      jsonResponse(
        {
          code: "version_skew",
          title: "Conflict",
          detail: "apply commission decision: version skew",
          status: 409,
        },
        409,
      ),
  }),
  play: async ({ canvasElement }) => {
    const page = await reverse(canvasElement, "Duplicate accrual");
    await page.findByText(/reload and try again/);
  },
};
