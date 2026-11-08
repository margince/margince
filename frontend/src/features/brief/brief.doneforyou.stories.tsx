// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { Receipt } from "../../lib/handled";
import { OpenSourceProvider } from "../../lib/opensource";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { automaticStageReceipt } from "../../testing/worklist.receiptreview.fixtures";
import { DoneForYou } from "./brief.doneforyou";
import "./brief.css";

const meta: Meta<typeof DoneForYou> = {
  title: "Shell/Home/Done for you",
  component: DoneForYou,
};
export default meta;
type Story = StoryObj<typeof DoneForYou>;

const deal = (n: number) => `01a00000-0000-7000-8000-0000000003${10 + n}`;
const audit = (n: number, c = 0) =>
  `01a00000-0000-7000-8000-00000000${400 + n}${c}`;

function change(
  n: number,
  name: string,
  headline: string,
  patch: Partial<Receipt> = {},
  c = 0,
): Receipt {
  const id = audit(n, c);
  return {
    ...automaticStageReceipt,
    id,
    headline,
    summary: headline,
    subject: { type: "deal", id: deal(n), label: name },
    undo: { audit_log_id: id, version: 7, reversed: false },
    ...patch,
  };
}

const REVIEW = {
  kind: "stage" as const,
  accepted: false,
  reversed: false,
  version: 7,
  can_undo: true,
  can_accept: true,
  writable: true,
};

const RECEIPTS: Receipt[] = [
  change(
    1,
    "Acme — ERP implementation",
    'Closed "Acme — ERP implementation" as lost',
    {
      reason:
        "The buyer confirmed the project was cancelled after the budget was withdrawn.",
      customer_quote:
        "We have to cancel the ERP project; the budget was withdrawn last week.",
      source_record: {
        type: "activity",
        id: deal(9),
        label: "Re: ERP rollout timeline",
      },
    },
  ),
  change(
    2,
    "Nordlicht Logistik und Lagerhaltung — Warehouse management system, phase two of three, Hamburg and Bremen sites",
    'Moved "Nordlicht" from Proposal to Negotiation. Probability now 60%',
    {
      reason: "Procurement asked for contract terms and named a signing date.",
      occurred_at: "2026-09-10T09:00:00Z",
    },
  ),
  change(
    3,
    "PIM Rollout",
    'Changed the amount on "PIM Rollout": 40,000.00 EUR → 45,000.00 EUR',
    {
      reason: "The latest quote names a higher total.",
      kind: "deal_amount",
      review: { ...REVIEW, kind: "amount" },
    },
  ),
  change(
    3,
    "PIM Rollout",
    'Changed the win probability on "PIM Rollout": 40% → 55%',
    {
      kind: "deal_probability",
      reason: undefined,
      review: { ...REVIEW, kind: "probability", accepted: true },
    },
    1,
  ),
];

function pending(receipts: readonly Receipt[]) {
  const deals = [...new Set(receipts.map((r) => r.subject?.id ?? ""))];
  return {
    as_of: "2026-09-13T08:00:00Z",
    deals: deals.length,
    closed: 1,
    redated: 0,
    revalued: 1,
    probabilities: 1,
    moved: 1,
    opened: 0,
    revived: 0,
    base_currency: "EUR",
    truncated: false,
    bundles: deals.map((id) => ({
      deal_id: id,
      deal_name:
        receipts.find((r) => r.subject?.id === id)?.subject?.label ?? "",
      version: 7,
      audit_log_ids: receipts
        .filter((r) => r.subject?.id === id && !r.review?.accepted)
        .map((r) => r.id),
      kinds: ["close_lost"],
      latest_at: "2026-09-13T07:15:00Z",
    })),
  };
}

// Accepting a change answers as the server does: the receipt reads as
// reviewed, and the deal's pending list no longer names it.
function stub(initial: readonly Receipt[]) {
  const accepted = new Set<string>();
  const current = () =>
    initial.map((r) =>
      accepted.has(r.id) && r.review
        ? { ...r, review: { ...r.review, accepted: true } }
        : r,
    );
  const accepts = Object.fromEntries(
    initial.map((r) => [
      `POST /deals/${r.subject?.id}/applied-changes/${r.id}/accept`,
      () => {
        accepted.add(r.id);
        return new Response(null, { status: 204 });
      },
    ]),
  );
  stubWithSession(
    {
      ...accepts,
      "GET /worklist/handled": () =>
        jsonResponse({
          as_of: "2026-09-13T08:00:00Z",
          receipts: current(),
          truncated: false,
        }),
      "GET /deal-corrections/pending": () => jsonResponse(pending(current())),
      "GET /magic": () =>
        jsonResponse({
          since: "2026-09-12T18:00:00Z",
          as_of: "2026-09-13T08:00:00Z",
          needs_you: [],
          could_not_complete: [],
          watching: [],
          done: [],
          sources_unavailable: [],
        }),
    },
    { deal: ["read", "update"] },
  );
}

/**
 * Three deals: a quote, a long name, and a deal with two changes, one already
 * reviewed. The server no longer lists the reviewed one, so it does not wait
 * here. Marking the other reviewed keeps its row, now saying so.
 */
export const ChangedDeals: Story = {
  render: () => {
    stub(RECEIPTS);
    return (
      <StoryProviders>
        <OpenSourceProvider openSource={() => undefined}>
          <DoneForYou completed={null} />
        </OpenSourceProvider>
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByText(
        "The buyer confirmed the project was cancelled after the budget was withdrawn.",
      ),
    ).toBeTruthy();
    const row = (
      await canvas.findByText("The latest quote names a higher total.")
    ).closest("li");
    if (!(row instanceof HTMLElement)) throw new Error("no row for the amount");
    await userEvent.click(
      await within(row).findByRole("button", { name: "Mark reviewed" }),
    );
    await expect(await within(row).findByText("Reviewed")).toBeVisible();
  },
};

/** More than five deals: the preview stops at five and the drawer holds all. */
export const ReviewAllDrawer: Story = {
  render: () => {
    stub(
      Array.from({ length: 7 }, (_, i) =>
        change(i + 1, `Deal ${i + 1}`, `Closed "Deal ${i + 1}" as lost`, {
          reason: `The customer stopped answering after the call on day ${i + 1}.`,
        }),
      ),
    );
    return (
      <StoryProviders>
        <DoneForYou completed={null} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Review all 7 deals" }),
    );
  },
};

/** A change with no reason a current server could send says so. */
export const NoReasonRecorded: Story = {
  render: () => {
    stub([
      change(1, "PIM Rollout", 'Closed "PIM Rollout" as lost', {
        reason: undefined,
      }),
    ]);
    return (
      <StoryProviders>
        <DoneForYou completed={null} />
      </StoryProviders>
    );
  },
};
