// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { jsonResponse, StoryProviders } from "./story-utils";
import { HandledForYouPanel } from "./worklist.handled";

// WHAT THE PRODUCT DID on this reader's behalf.
//
// Every other panel on the page asks for something. This one asks for nothing:
// it is the receipt a reader checks, and the reason the acts above it are safe
// to take at all.
//
// NO VERBS, and that is what to look for in the first frame rather than an
// omission to overlook. A row here offering "complete" would ask the reader to
// redo work that is already done, on the one surface that exists to tell them
// they need not.
//
// The other three frames are the states that must not look alike:
//
//   - NOTHING was done, which is the common day and one sentence. It drew a
//     table's three column names over no rows before, which reports neither a
//     quiet day nor a broken read;
//   - a read cut short is not the whole of what was done, so the caveat is
//     drawn and the footer's count withheld — a reader who took a floor for a
//     total would close the page believing they had seen everything, which is
//     the one thing a receipt surface must not cause;
//   - a FAILED read says so and offers the retry. Reported as "nothing was
//     done" it would be the product claiming it had acted on nothing at the
//     moment it could not say what it had acted on.
//
// Read every frame in BOTH themes with the toolbar's Theme control.

type HandledData = components["schemas"]["HandledForYou"];

// Three acts of three kinds, one of them about no record at all: not every act
// names one, and an absent subject is a real state rather than a missing field.
const aBusyMorning: HandledData = {
  as_of: "2026-09-05T09:00:00Z",
  truncated: false,
  receipts: [
    {
      id: "00000000-0000-4000-8000-0000000000e1",
      kind: "email_sent",
      summary: "Sent the confirmation to Kirsten",
      occurred_at: "2026-09-05T08:12:00Z",
      subject: {
        type: "person",
        id: "00000000-0000-4000-8000-0000000000a1",
        label: "Kirsten Bauer",
      },
    },
    {
      id: "00000000-0000-4000-8000-0000000000e2",
      kind: "deal_updated",
      summary: "Moved Northstar renewal to Negotiation",
      occurred_at: "2026-09-05T07:40:00Z",
      subject: {
        type: "deal",
        id: "00000000-0000-4000-8000-0000000000b2",
        label: "Northstar renewal",
      },
    },
    {
      id: "00000000-0000-4000-8000-0000000000e3",
      kind: "rule_ran",
      summary: "Reordered the follow-up queue",
      occurred_at: "2026-09-05T06:05:00Z",
      // No subject. The row reads as its summary alone rather than inventing
      // something to point at.
    },
  ],
};

// The panel fetches, so each frame answers its own read.
function stubHandled(answer: () => Promise<Response>) {
  globalThis.fetch = (async (input: RequestInfo | URL): Promise<Response> => {
    const url = String(input instanceof Request ? input.url : input);
    return url.includes("/worklist/handled")
      ? answer()
      : jsonResponse({ data: [] });
  }) as typeof fetch;
}

function frame(answer: () => Promise<Response>) {
  stubHandled(answer);
  return (
    <StoryProviders>
      <HandledForYouPanel />
    </StoryProviders>
  );
}

const meta: Meta<typeof HandledForYouPanel> = {
  title: "Records/Worklist/Handled for you",
  component: HandledForYouPanel,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof HandledForYouPanel>;

/** A morning the product worked through: what happened, what it was about, and
 *  when — with the count in the footer and nothing anywhere to press. */
export const ABusyMorning: Story = {
  render: () => frame(async () => jsonResponse(aBusyMorning)),
};

/** The common day. One sentence, no table, and no count of nothing in the
 *  footer band. */
export const NothingWasDone: Story = {
  render: () =>
    frame(async () =>
      jsonResponse({ ...aBusyMorning, receipts: [], truncated: false }),
    ),
};

/** The read stopped short, so this is a floor rather than everything. The
 *  caveat is drawn and the footer's count is withheld. */
export const MoreWasDoneThanWasRead: Story = {
  render: () =>
    frame(async () =>
      jsonResponse({
        ...aBusyMorning,
        receipts: aBusyMorning.receipts.slice(0, 2),
        truncated: true,
      }),
    ),
};

/** The read failed. It says so and offers the retry; it never reads as a
 *  morning nothing happened in. */
export const CouldNotBeRead: Story = {
  render: () =>
    frame(async () =>
      jsonResponse({ title: "The receipts could not be read" }, 502),
    ),
};
