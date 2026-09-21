// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { RequestReceipts, type RightsCaseReceipt } from "./confirmreceipts";
import { StoryProviders } from "./story-utils";
import "./confirm.css";

// What the subject is left holding after a confirm link opened a case.
//
// The reference is the whole point of the section: without it a contact who
// asks after their own request a fortnight later has nothing to quote, and no
// way to tell whether the answer Art. 12(3) owes them ever came. So the states
// worth a picture are the ones where the list has to stay READABLE — a
// correction naming its field beside an erasure naming none, and a single
// correction on its own.
//
// The third state, no receipts at all, renders nothing by design: a marketing
// answer opens no case, and a heading over an empty list would promise an
// answer nobody owes. There is no picture to review, so there is no story.

const meta: Meta<typeof RequestReceipts> = {
  title: "Signed out/Confirm your details/Request receipts",
  component: RequestReceipts,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <div className="pref-page">
          <Story />
        </div>
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof RequestReceipts>;

const receipt = (over: Partial<RightsCaseReceipt>): RightsCaseReceipt => ({
  kind: "rectify",
  reference: "RC-2026-0914-0001",
  ...over,
});

/**
 * Two corrections and a removal from one submission.
 *
 * Each correction names its field, because a subject who fixed a title and a
 * phone number receives two references and cannot otherwise tell which answer
 * is about which. The erasure names none: there is no field to name.
 */
export const CorrectionsAndARemoval: Story = {
  args: {
    receipts: [
      receipt({ field: "title" }),
      receipt({ reference: "RC-2026-0914-0002", field: "phone" }),
      receipt({ kind: "erasure", reference: "RC-2026-0914-0003" }),
    ],
  },
};

/** One correction, which is what most submissions produce. */
export const OneCorrection: Story = {
  args: { receipts: [receipt({ field: "phone" })] },
};
