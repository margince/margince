// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import { StoryProviders } from "../../testing/story-utils";
import { WaitingEmailLine } from "./worklist.emailtitle";
import { row } from "./worklist.fixtures";

type EmailSummary = components["schemas"]["EmailSummary"];

const MESSAGE_ID = "01a05500-0000-7000-8000-0000000000e1";

function waiting(over: Partial<EmailSummary>) {
  return row({
    id: MESSAGE_ID,
    source: "customer_waiting",
    category: "customer_waiting",
    level: 1,
    consequence: "buyer_waits",
    title: "Re: the renewal quote",
    email_summary: {
      activity_id: MESSAGE_ID,
      occurred_at: "2026-08-29T09:15:00Z",
      occurred_precision: "instant",
      version: 4,
      subject: "Re: the renewal quote",
      preview: "Can you hold the price until Friday?",
      counterparty: "Dana Buyer",
      direction: "inbound",
      display_status: "team",
      move: "needs_reply",
      attachment_count: 0,
      ...over,
    },
  });
}

const meta = {
  title: "Records/Worklist/Waiting email line",
  parameters: { layout: "padded" },
} satisfies Meta;
export default meta;
type Story = StoryObj;

const line = (canvasElement: HTMLElement) =>
  within(canvasElement).findByRole("button", { name: /the renewal quote/ });

export const ReceivedAtAnInstant: Story = {
  render: () => (
    <StoryProviders>
      <WaitingEmailLine item={waiting({})} onOpen={() => undefined} />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    await expect((await line(canvasElement)).textContent).toMatch(
      /\d{1,2}:\d{2}/,
    );
  },
};

// A message logged by its day alone names the date and no clock time.
export const OnlyItsDayKnown: Story = {
  render: () => (
    <StoryProviders>
      <WaitingEmailLine
        item={waiting({
          occurred_at: "2026-08-29T12:00:00Z",
          occurred_precision: "day",
        })}
        onOpen={() => undefined}
      />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    await expect((await line(canvasElement)).textContent).not.toMatch(
      /\d{1,2}:\d{2}/,
    );
  },
};
