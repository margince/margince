// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { Panel } from "../../design-system/panel";
import { StoryProviders } from "../story-utils";
import { MomentRow } from "./moment";

// THE MOMENT as the lead row of the needs list, in the two shapes its
// evidence takes — and the difference is whether the row carries a "What
// this rests on" disclosure at all.
//
// A promise read out of a CONVERSATION can quote the sentence it was made
// in, and the disclosure opens on that quote: words a doubting reader can
// check. A promise somebody filed as a TASK has only its own subject for
// evidence, and a disclosure that opens on the headline restated answers
// nothing — so that row draws none. Both states are here side by side
// because the absence is the design, not a row that failed to load.

type ContactMoment = components["schemas"]["ContactMoment"];

const meta: Meta<typeof MomentRow> = {
  title: "Records/Record reading/The moment",
  component: MomentRow,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof MomentRow>;

// A promise an extractor read out of a message: the disclosure opens on the
// verbatim sentence.
const quotedPromise: ContactMoment = {
  claim_key: "moment:overdue_promise",
  evidence_fingerprint: "story-claim",
  rule: "overdue_promise",
  headline: "You owe Dana Buyer: the signed contract",
  why_now: "Promised for a date that passed 3 days ago.",
  confidence: "observed_fact",
  evidence: [
    {
      type: "activity",
      id: "00000000-0000-7000-8000-000000000001",
      label: "They will send the contract",
      snippet: "We'll get the contract over to you by Friday.",
      observed_at: "2026-08-03T09:00:00Z",
    },
  ],
  recommended_action: {
    kind: "open_record",
    label: "Open the contact",
    state: "available",
    destination: {
      surface: "record",
      entity_type: "contact",
      entity_id: "00000000-0000-7000-8000-000000000002",
    },
  },
};

// A promise somebody filed as a task: its only evidence is its own subject,
// so the row carries no disclosure.
const filedPromise: ContactMoment = {
  claim_key: "moment:open_promise",
  evidence_fingerprint: "story-task",
  rule: "open_promise",
  headline: "You owe them: Send the workshop agenda",
  why_now: "Due in 2 days.",
  confidence: "observed_fact",
  evidence: [
    {
      type: "task",
      id: "00000000-0000-7000-8000-000000000003",
      label: "Send the workshop agenda",
    },
  ],
  recommended_action: {
    kind: "complete_task",
    label: "Open it from the task list",
    state: "blocked",
  },
};

export const QuotedAndFiled: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 720 }}>
        <Panel tone="ai" title="What needs you">
          <MomentRow moment={quotedPromise} onOpenRecord={() => {}} />
          <MomentRow moment={filedPromise} onOpenRecord={() => {}} />
        </Panel>
      </div>
    </StoryProviders>
  ),
};

export const Narrow: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 390 }}>
        <Panel tone="ai" title="What needs you">
          <MomentRow moment={quotedPromise} onOpenRecord={() => {}} />
        </Panel>
      </div>
    </StoryProviders>
  ),
};
