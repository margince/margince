// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { Receipt } from "../../lib/handled";
import { StoryProviders, stubWithSession } from "../../testing/story-utils";
import { automaticStageReceipt } from "../../testing/worklist.receiptreview.fixtures";
import { ChangeRow } from "./brief.doneforyou.change";
import "./brief.css";

// One change Margince made to a deal, as Home's "Done for you" lists it.
const meta: Meta<typeof ChangeRow> = {
  title: "Shell/Home/Done for you change",
  component: ChangeRow,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ChangeRow>;

const AS_OF = "2026-09-13T08:00:00Z";

function rows(receipts: readonly Receipt[]) {
  stubWithSession({}, { deal: ["read", "update"] });
  return (
    <StoryProviders>
      <ul className="brief-done-changes">
        {receipts.map((receipt) => (
          <ChangeRow
            key={receipt.id}
            receipt={receipt}
            asOf={AS_OF}
            onSettled={() => undefined}
          />
        ))}
      </ul>
    </StoryProviders>
  );
}

const closed: Receipt = {
  ...automaticStageReceipt,
  id: "01a00000-0000-7000-8000-000000000801",
  kind: "deal_close_lost",
  headline: 'Closed "Acme — ERP implementation" as lost',
  summary: 'Closed "Acme — ERP implementation" as lost',
  reason:
    "The buyer confirmed the project was cancelled after the budget was withdrawn.",
  customer_quote:
    "We have to cancel the ERP project; the budget was withdrawn last week.",
  undo: {
    audit_log_id: "01a00000-0000-7000-8000-000000000801",
    version: 7,
    reversed: false,
  },
};

// Older than the morning it is read on, so it shows its day, not its hour.
const reviewed: Receipt = {
  ...automaticStageReceipt,
  id: "01a00000-0000-7000-8000-000000000802",
  occurred_at: "2026-09-10T09:00:00Z",
  reason: "Procurement asked for contract terms and named a signing date.",
  undo: {
    audit_log_id: "01a00000-0000-7000-8000-000000000802",
    version: 7,
    reversed: false,
  },
  review: {
    kind: "stage",
    accepted: true,
    reversed: false,
    version: 7,
    can_undo: true,
    can_accept: false,
    writable: true,
  },
};

/** A closure: marked lost, its reason, the customer's words, its verbs. */
export const Closure: Story = {
  render: () => rows([closed]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText(
        "The buyer confirmed the project was cancelled after the budget was withdrawn.",
      ),
    ).toBeVisible();
    await expect(
      canvas.getByText(
        "We have to cancel the ERP project; the budget was withdrawn last week.",
      ),
    ).toBeVisible();
    await expect(
      await canvas.findByRole("button", { name: "Undo" }),
    ).toBeVisible();
  },
};

export const ClosureDark: Story = { ...Closure, globals: { theme: "dark" } };

/** A change from an earlier day, accepted before Margince stopped asking. */
export const Accepted: Story = {
  render: () => rows([reviewed]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("button", { name: "Undo" }),
    ).toBeVisible();
    await expect(canvas.queryByText(/review/i)).toBeNull();
  },
};

export const AcceptedDark: Story = { ...Accepted, globals: { theme: "dark" } };
