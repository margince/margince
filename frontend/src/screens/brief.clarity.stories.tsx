// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { BriefScreen } from "./brief";
import { ranked, readingsDay, taskRow } from "./brief.fixtures";
import { jsonResponse, StoryProviders, stubWithSession } from "./story-utils";
import type { WorklistItem } from "./worklist.queries";
import {
  automaticDateReceipt,
  automaticStageReceipt,
} from "./worklist.receiptreview.fixtures";

const dealID = "01a00000-0000-7000-8000-000000000011";
const conversationID = "01a00000-0000-7000-8000-000000000014";
const rows: WorklistItem[] = [
  ...Array.from(
    { length: 6 },
    (_, index): WorklistItem => ({
      ...taskRow(
        `task-${index}`,
        index === 0
          ? "Prepare the comparison of both product demonstrations for the customer"
          : `Follow up on the agreed next step ${index}`,
      ),
      due_at: "2026-09-10T08:30:00Z",
      overdue: true,
      urgent: true,
      subject: { type: "deal", id: dealID, label: "PIM Rollout phase 2" },
    }),
  ),
  {
    id: conversationID,
    source: "customer_waiting",
    category: "customer_waiting",
    level: 2,
    consequence: "buyer_waits",
    title: "PIM Rollout phase 2",
    detail: "Could you send the updated implementation schedule?",
    urgent: true,
    because: [],
    actions: ["open"],
    subject: { type: "deal", id: dealID, label: "PIM Rollout phase 2" },
    move: { action: "draft_reply", activity_id: conversationID },
  },
  {
    id: "risk-one",
    source: "deal_at_risk",
    category: "deals_at_risk",
    level: 4,
    consequence: "deal_drifts",
    title: "Northstar renewal",
    because: [{ kind: "quiet_days", value: { kind: "days", days: 21 } }],
    actions: ["open"],
    deal: {
      amount_minor: 8_900_000,
      currency: "EUR",
      quiet_days: 21,
      expected_close_date: "2026-09-27",
    },
    subject: {
      type: "deal",
      id: "01a00000-0000-7000-8000-000000000015",
      label: "Northstar renewal",
    },
    verdict: {
      source: "deal_status",
      standing: "drifting",
      line: "The buyer has not answered the proposal. Agree a next step before the forecast date.",
      as_of: "2026-09-13T08:00:00Z",
    },
  },
  {
    id: "lead-one",
    source: "lead_response",
    category: "leads",
    level: 5,
    consequence: "buyer_waits",
    title: "Kirsten Vogel asked about pricing",
    because: [],
    actions: ["open"],
    subject: {
      type: "lead",
      id: "01a00000-0000-7000-8000-000000000016",
      label: "Kirsten Vogel",
    },
  },
];

const meta: Meta<typeof BriefScreen> = {
  title: "Shell/Home clarity",
  component: BriefScreen,
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof BriefScreen>;

export const WorkingMorning: Story = {
  render: () => {
    const day = readingsDay(
      {
        prospecting: 1,
        buyer_replies: 1,
        review: 0,
        revenue_at_risk_minor: 8_900_000,
        revenue_currency: "EUR",
      },
      rows,
      [],
      { urgent: 7, due: 6 },
    );
    stubWithSession(
      {
        "GET /worklist": () =>
          jsonResponse({ ...day, as_of: "2026-09-13T08:00:00Z" }),
        "GET /brief": () =>
          jsonResponse({ ...ranked, generated_at: "2026-09-13T05:30:00Z" }),
        "GET /digest": () => jsonResponse({ title: "Not found" }, 404),
        "GET /worklist/handled": () =>
          jsonResponse({
            as_of: day.as_of,
            receipts: [automaticDateReceipt, automaticStageReceipt],
            truncated: false,
          }),
        [`GET /deals/${dealID}`]: () =>
          jsonResponse({ id: dealID, name: "PIM Rollout phase 2" }),
        "POST /reports/deals-by-stage": () =>
          jsonResponse({ rows: [], columns: [] }),
      },
      { deal: ["read", "update"], activity: ["read", "update"] },
    );
    return (
      <StoryProviders>
        <BriefScreen />
      </StoryProviders>
    );
  },
};
