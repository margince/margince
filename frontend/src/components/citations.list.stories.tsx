// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen } from "storybook/test";
import type { components } from "../api/schema";
import { StoryProviders } from "../testing/story-utils";
import type { Cited } from "./citations";
import { Citations } from "./citations.list";

// A cited email carries the date a claim is checked against. A message logged
// with only its day names that day and no clock, since nobody said one.

type EmailSummary = components["schemas"]["EmailSummary"];

function citedEmail(overrides: Partial<EmailSummary>): Cited {
  const email: EmailSummary = {
    activity_id: "act-1",
    subject: "Translation fallback decision",
    preview: "Could you confirm which locale wins when the string is missing?",
    occurred_at: "2026-09-03T09:15:00Z",
    occurred_precision: "instant",
    direction: "inbound",
    counterparty: "Frédéric de Gombert",
    attachment_count: 0,
    move: "needs_reply",
    display_status: "team",
    version: 1,
    ...overrides,
  };
  return {
    entity_type: "activity",
    entity_id: email.activity_id,
    email_summary: email,
  };
}

const meta = {
  title: "Records/Record 360/Cited email",
  component: Citations,
  parameters: { layout: "padded" },
} satisfies Meta<typeof Citations>;
export default meta;

type Story = StoryObj<typeof meta>;

export const SentAtATime: Story = {
  args: { evidence: [citedEmail({})], onOpenEmail: () => undefined },
  render: (args) => (
    <StoryProviders>
      <Citations {...args} />
    </StoryProviders>
  ),
};

export const KnownByItsDay: Story = {
  args: {
    evidence: [
      citedEmail({
        occurred_at: "2026-09-03T12:00:00Z",
        occurred_precision: "day",
      }),
    ],
    onOpenEmail: () => undefined,
  },
  render: SentAtATime.render,
  play: async () => {
    const email = await screen.findByRole("button", {
      name: /Translation fallback decision/,
    });
    await expect(email).toHaveTextContent("2026");
    await expect(email.textContent).not.toMatch(/\d{1,2}:\d{2}/);
  },
};
