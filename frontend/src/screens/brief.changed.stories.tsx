// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ChangedSinceBrief } from "./brief.changed";
import { meetingRow, readingsDay } from "./brief.fixtures";
import { StoryProviders } from "./story-utils";
import type { WorklistItem } from "./worklist.queries";

// WHAT HAS HAPPENED SINCE THE NIGHT LOOKED.
//
// The morning is assembled overnight and read hours later, and this line is the
// only thing on the page that says which rows moved in the gap. It is one of the
// two notices that stand between the greeting and the readings strip, so read it
// beside `Brief coverage`: the two are the same kind of statement — a fact about
// the whole page rather than about any one row — and they carry the same
// `brief-notice` rhythm on the screen that draws them.
//
// The frames are the two shapes it takes. Up to three rows are NAMED, because a
// rep can act on a name; past that it counts, because a list of eleven titles is
// the queue again and nobody reads it twice. There is deliberately no frame for
// the quiet morning: the strip renders null both when nothing changed and when
// there was no run to compare against, and those are held in
// `brief.changed.test.tsx` rather than as an empty story root.
//
// Read both frames in BOTH themes with the toolbar's Theme control.

/** A row the overnight run has already seen move. */
function moved(id: string, title: string): WorklistItem {
  return { ...meetingRow(id, true), title, changed_since_brief: true };
}

/** A row the run saw and nothing has touched since. */
function still(id: string, title: string): WorklistItem {
  return { ...meetingRow(id, true), title, changed_since_brief: false };
}

const meta: Meta<typeof ChangedSinceBrief> = {
  title: "Shell/Brief changed since",
  component: ChangedSinceBrief,
};
export default meta;

type Story = StoryObj<typeof ChangedSinceBrief>;

// Two rows moved and one did not. The unchanged row is the point of the frame:
// the line names what the server flagged and nothing else, so a rep can trust
// that a title absent from it has not moved.
export const AFewRowsNamed: Story = {
  render: () => (
    <StoryProviders>
      <ChangedSinceBrief
        day={readingsDay({}, [
          moved("m1", "Weber GmbH · quarterly review"),
          moved("m2", "Aster Handel · pricing call"),
          still("m3", "Nordwind AG · kickoff"),
        ])}
      />
    </StoryProviders>
  ),
};

// More than the three it will name. The count carries the rest to the queue
// that holds them, which is where a rep goes to read eleven rows anyway.
export const MoreThanItWillName: Story = {
  render: () => (
    <StoryProviders>
      <ChangedSinceBrief
        day={readingsDay(
          {},
          Array.from({ length: 7 }, (_, at) =>
            moved(`m${at}`, `Account ${at + 1} · renewal`),
          ),
        )}
      />
    </StoryProviders>
  ),
};
