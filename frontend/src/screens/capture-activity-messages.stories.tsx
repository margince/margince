// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { Panel } from "../design-system/panel";
import { CaptureMessages } from "./capture-activity-messages";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

type TraceEntry = components["schemas"]["CaptureTraceEntry"];

const ENTRY = {
  id: "01930000-0000-7000-8000-00000000d001",
  connector: "gmail",
  outcome: "captured",
  outcome_now: "captured",
  reason: null,
  activity_id: null,
  counterparty: "dana@client.example",
  subject: "Q3 pricing",
  occurred_at: "2026-08-15T09:12:00Z",
} satisfies TraceEntry;

// One row per outcome the table draws differently, with a long address and a
// long subject so the truncation is on screen.
const ENTRIES: TraceEntry[] = [
  {
    ...ENTRY,
    counterparty:
      "maximilian.oberstleutnant-von-hohenzollern@procurement.example",
    subject:
      "Quarterly procurement review: revised framework agreement, annex C pricing schedule and open questions from legal",
  },
  {
    ...ENTRY,
    id: "01930000-0000-7000-8000-00000000d002",
    connector: "imap",
    outcome: "internal",
    outcome_now: "internal",
    reason: "internal_only",
  },
  {
    ...ENTRY,
    id: "01930000-0000-7000-8000-00000000d003",
    outcome: "deferred",
    outcome_now: "suppressed",
    resolution: {
      status: "noise",
      kind: "newsletter",
      resolved_at: "2026-08-15T09:30:00Z",
    },
  },
  {
    ...ENTRY,
    id: "01930000-0000-7000-8000-00000000d004",
    connector: "test_mailbox",
    outcome: "fault",
    outcome_now: "fault",
    reason: "derivation_failed",
    counterparty: null,
    subject: null,
  },
];

function story(payloads: boolean) {
  return () => {
    installFetchStub({
      "GET /channel-providers": () =>
        jsonResponse({
          data: [{ provider: "gmail", label: "Gmail" }],
          capture_sources: [],
        }),
    });
    return (
      <StoryProviders>
        <Panel title="Last 24 hours">
          <CaptureMessages
            entries={ENTRIES}
            payloads={payloads}
            onOpen={fn()}
          />
        </Panel>
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof CaptureMessages> = {
  title: "Settings/You/Capture activity/Messages",
  component: CaptureMessages,
};
export default meta;
type Story = StoryObj<typeof CaptureMessages>;

export const WithPayloads: Story = {
  render: story(true),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const table = await canvas.findByRole("table", { name: "Messages" });
    await userEvent.hover(within(table).getAllByRole("row")[1]);
    await expect(table).toBeVisible();
  },
};

// Without stored payloads the message column has nothing to say.
export const WithoutPayloads: Story = { render: story(false) };

export const WithPayloadsPhone: Story = {
  render: story(true),
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const WithPayloadsDark: Story = {
  render: story(true),
  globals: { theme: "dark" },
};
