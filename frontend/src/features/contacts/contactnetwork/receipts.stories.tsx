// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../../api/schema";
import { formatDate } from "../../../format/format.dates";
import { UTC_ZONE } from "../../../format/timezone";
import { StoryProviders } from "../../../testing/story-utils";
import { ReceiptList } from "./receipts";
import "../contactnetwork.css";

// The messages a relationship was read from: a mail takes the email citation,
// a meeting the plain line. A meeting logged for its date alone shows that
// date, never the noon it is stored at.

type Receipt = components["schemas"]["ContactGraphReceipt"];

const DAY_MEETING_AT = "2026-08-27T12:00:00Z";

const RECEIPTS: readonly Receipt[] = [
  {
    activity_id: "018f3a1b-0000-7000-8000-0000000000e1",
    subject: "Re: Q4 rollout — site list",
    occurred_at: "2026-08-28T09:00:00Z",
    occurred_precision: "instant",
    kind: "email",
  },
  {
    activity_id: "018f3a1b-0000-7000-8000-0000000000e2",
    subject: "Site walk",
    occurred_at: DAY_MEETING_AT,
    occurred_precision: "day",
    kind: "meeting",
  },
];

const meta: Meta<typeof ReceiptList> = {
  title: "Records/Contact 360/Network/Message receipts",
  component: ReceiptList,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ReceiptList>;

/** A mail and a meeting known only by its date. */
export const MailAndDayMeeting: Story = {
  render: () => (
    <StoryProviders>
      <ReceiptList receipts={RECEIPTS} />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const meeting = await canvas.findByText(/Site walk/);
    await expect(meeting.textContent).toContain(
      formatDate(DAY_MEETING_AT, "en", UTC_ZONE),
    );
  },
};

/** A notice the installation's relay sent for the reader, beside their own mail. */
export const SentByMarginceForYou: Story = {
  render: () => (
    <StoryProviders>
      <ReceiptList
        relayMember="self"
        receipts={[
          {
            activity_id: "018f3a1b-0000-7000-8000-0000000000e3",
            subject: "Information about your personal data",
            occurred_at: "2026-10-20T09:00:00Z",
            occurred_precision: "instant",
            kind: "email",
            controller_send: { from_address: "privacy@kps.test" },
          },
          ...RECEIPTS,
        ]}
      />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByText("Sent by Margince for you"),
    ).toBeVisible();
    await expect(canvas.getByText("From privacy@kps.test")).toBeVisible();
  },
};
