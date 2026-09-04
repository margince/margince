// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { jsonResponse, StoryProviders } from "./story-utils";
import { WorklistPane } from "./worklist.pane";
import type { WorklistItem } from "./worklist.queries";

// WHAT THE SELECTED ROW IS ABOUT, beside the queue.
//
// The frames are about the question the queue leaves open. A row says a
// customer is waiting; what it cannot say is how long the silence has run in
// BOTH directions, and a rep who last wrote yesterday answers differently from
// one who has not written since March. So the pane is two facts, and "never" is
// one of the readings rather than a gap — a customer nobody has ever answered
// is the strongest case on the page for answering now, and an em dash would
// leave the reader guessing whether the fact was missing or the silence real.
//
// The last frame is the row that gets NO pane. A deal-bearing row already
// carries its figures on the row itself, so a pane repeating them would be a
// second spelling of the same facts; the component draws nothing rather than an
// empty frame.
//
// Read every frame in BOTH themes with the toolbar's Theme control.

const PERSON = "01a05500-0000-7000-8000-0000000000aa";

function personRow(): WorklistItem {
  return {
    id: "one",
    title: "Kirsten Vogel is waiting on an answer",
    subject: { type: "person", id: PERSON, label: "Kirsten Vogel" },
  } as WorklistItem;
}

function stub360(answer: () => Promise<Response>) {
  globalThis.fetch = (async (input: RequestInfo | URL): Promise<Response> => {
    const url = String(input instanceof Request ? input.url : input);
    return url.includes("/360") ? answer() : jsonResponse({ data: [] });
  }) as typeof fetch;
}

function frame(item: WorklistItem, answer: () => Promise<Response>) {
  stub360(answer);
  return (
    <StoryProviders>
      <div style={{ maxWidth: 360 }}>
        <WorklistPane item={item} />
      </div>
    </StoryProviders>
  );
}

const meta: Meta<typeof WorklistPane> = {
  title: "Records/Worklist/Context pane",
  component: WorklistPane,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof WorklistPane>;

/** Both directions answered: the rep wrote last, and the customer has been
 *  quiet since. */
export const BothSidesSpoken: Story = {
  render: () =>
    frame(personRow(), async () =>
      jsonResponse({
        person: { id: PERSON, full_name: "Kirsten Vogel" },
        last_inbound_at: "2026-03-14T08:12:00Z",
        last_outbound_at: "2026-08-30T16:40:00Z",
      }),
    ),
};

/** Nobody has ever answered them. "Never" is the reading, not a blank. */
export const NeverAnswered: Story = {
  render: () =>
    frame(personRow(), async () =>
      jsonResponse({
        person: { id: PERSON, full_name: "Kirsten Vogel" },
        last_inbound_at: "2026-08-30T16:40:00Z",
        last_outbound_at: null,
      }),
    ),
};

/** The read failed. The pane says so and offers the retry rather than drawing
 *  a person with no history. */
export const CouldNotBeRead: Story = {
  render: () =>
    frame(personRow(), async () =>
      jsonResponse({ title: "Upstream failed" }, 502),
    ),
};

/** A row about a deal rather than a person draws NOTHING — not an empty frame,
 *  which would read as a pane that failed to arrive. */
export const NoPaneForThisRow: Story = {
  render: () =>
    frame(
      {
        id: "two",
        title: "Fleet retrofit closes on Friday",
        subject: { type: "deal", id: "d-fleet", label: "Fleet retrofit" },
      } as WorklistItem,
      async () => jsonResponse({ data: [] }),
    ),
};
