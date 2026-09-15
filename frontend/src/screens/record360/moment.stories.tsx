// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { FileText, Send } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { StoryProviders } from "../story-utils";
import { MomentRow } from "./moment";

// The leading card's own verb, in the two shapes it draws: the server's own,
// whenever it names a real destination, and the page's own fallback
// otherwise (MomentRow's own doc says why). Both are the ONE `.today-actions`
// column the kit owns — these stories are that column at its two most
// different contents, task and reply.

const meta: Meta<typeof MomentRow> = {
  title: "Records/Record reading/The moment row",
  component: MomentRow,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof MomentRow>;
type ContactMoment = components["schemas"]["ContactMoment"];

function Pane({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <StoryProviders>
      <div style={{ maxWidth: 720 }}>{children}</div>
    </StoryProviders>
  );
}

const TASK_MOMENT: ContactMoment = {
  claim_key: "moment:overdue_promise",
  evidence_fingerprint: "fp-1",
  rule: "overdue_promise",
  headline: "Send the renewal paperwork",
  why_now: "Promised on the call last Tuesday, and nothing has gone out since.",
  confidence: "observed_fact",
  evidence: [
    {
      type: "activity",
      id: "a-1",
      label: "Renewal call",
      snippet: "I'll get the paperwork over to you this week.",
      observed_at: "2026-08-03T09:00:00Z",
    },
  ],
  recommended_action: {
    kind: "complete_task",
    label: "Open the task",
    state: "available",
    destination: {
      surface: "task",
      entity_type: "activity",
      entity_id: "a-2",
    },
  },
};

// The server named a real destination, so its own verb draws — one filled,
// ai-tinted button, alone in the column.
export const TaskCase: Story = {
  render: () => (
    <Pane>
      <MomentRow moment={TASK_MOMENT} onOpenRecord={() => {}} />
    </Pane>
  ),
};

const REPLY_MOMENT: ContactMoment = {
  claim_key: "moment:open_promise",
  evidence_fingerprint: "fp-2",
  rule: "open_promise",
  headline: "Confirm the server booking status",
  why_now: "Promised on Monday, and nothing has gone out since.",
  confidence: "observed_fact",
  evidence: [],
  recommended_action: {
    kind: "log_activity",
    label: "Log it",
    state: "will_confirm",
  },
};

// The server named no destination (its own action carries no place to go),
// so the page's own fallback draws instead — a reply verb and a place to log
// one, in the same column the server's own verb would have drawn in.
export const ReplyCase: Story = {
  render: () => (
    <Pane>
      <MomentRow
        moment={REPLY_MOMENT}
        action={
          <>
            <span className="today-verb">
              <Button small variant="ai">
                <Send size={15} aria-hidden="true" />
                Draft
              </Button>
            </span>
            <span className="today-verb">
              <Button small variant="ghost">
                <FileText size={15} aria-hidden="true" />
                Log activity
              </Button>
            </span>
          </>
        }
      />
    </Pane>
  ),
};
