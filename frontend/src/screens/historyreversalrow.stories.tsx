// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { formatDateTime } from "../format/format";
import type { PairRow } from "./historyreversal";
import { ReversalPairRow } from "./historyreversalrow";
import { StoryProviders } from "./story-utils";

// A reversal and the change it reverses, drawn as ONE line the reader can
// open. history.stories.tsx exercises this shape through the whole panel;
// this is the row's own story, so it reads directly rather than through
// RecordHistory's pairing pass.

type AuditHistoryEntry = components["schemas"]["AuditHistoryEntry"];

const reversed: AuditHistoryEntry = {
  id: "h9",
  actor_type: "human",
  actor_id: "u-sam",
  actor_name: "Sam Okafor",
  action: "update",
  occurred_at: "2026-07-14T12:00:00Z",
  summary: "Sam Okafor repriced the deal",
  before: { amount_minor: 2500000 },
  after: { amount_minor: 4150000 },
};

const reversal: AuditHistoryEntry = {
  id: "h10",
  actor_type: "human",
  actor_id: "u-tin",
  actor_name: "Tin Nguyen",
  action: "restore",
  occurred_at: "2026-07-14T12:04:00Z",
  summary: "Tin Nguyen restored the deal",
  undid_audit_log_id: "h9",
  before: { amount_minor: 4150000 },
  after: { amount_minor: 2500000 },
};

const pair: PairRow = {
  kind: "pair",
  reversal,
  reversed,
  atIso: reversal.occurred_at,
  whollyUndone: true,
  sameActor: false,
};

// The ordinary history row, for the two entries the expanded pair discloses:
// the same `timeline-plain` shape RecordHistory's own rows draw, a sentence,
// an inline date, nothing folded into a gutter.
function MemberRow({ entry }: Readonly<{ entry: AuditHistoryEntry }>) {
  return (
    <li>
      <span className="tl-body">
        <span className="tl-title">{entry.summary}</span>
        <span className="tl-meta">
          <span>{formatDateTime(entry.occurred_at, "en", "UTC")}</span>
        </span>
      </span>
    </li>
  );
}

function Row({ row }: Readonly<{ row: PairRow }>) {
  return (
    <StoryProviders>
      <ul className="timeline timeline-plain" style={{ maxWidth: 640 }}>
        <ReversalPairRow row={row} currency="EUR">
          <MemberRow entry={row.reversal} />
          <MemberRow entry={row.reversed} />
        </ReversalPairRow>
      </ul>
    </StoryProviders>
  );
}

const meta: Meta<typeof Row> = {
  title: "Records/Record history/Reversal pair row",
  component: Row,
};
export default meta;
type Story = StoryObj<typeof Row>;

// Closed: one line, the field's settled value, and the net caption, neither
// audit row shown yet.
export const Collapsed: Story = { render: () => <Row row={pair} /> };

// The same closed row in dark: the net caption and the settled value sit on
// the panel's own tint, so this is the story that shows whether either
// flattens against the dark ground.
export const CollapsedDark: Story = {
  ...Collapsed,
  globals: { theme: "dark" },
};

// Opened: the two member rows it collapsed, each the ordinary
// `timeline-plain` shape: its own actor, time and verb, not a second,
// thinner spelling of one.
export const Expanded: Story = {
  render: () => <Row row={pair} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: /show both changes/i }),
    );
  },
};
