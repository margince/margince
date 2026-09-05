// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { jsonResponse, StoryProviders } from "./story-utils";
import { TeamBoard } from "./worklist.board";

// WHO ON THE TEAM IS CARRYING WHAT — counts, not rows, because the queue below
// ranks one person's day and cannot answer "who is drowning".
//
// The frames are about the three honest answers a board can give. A team with
// somebody carrying more than the rest is the one a lead opens it for. A board
// that could not be READ says so rather than drawing zeros, which would be the
// same lie one lane further out. And a count read to its bound is a FLOOR: the
// line under the table saying so is the one direction this surface must not
// get wrong, because a lead told "3" over a figure that is really 3-or-more
// will not go looking.
//
// Read every frame in BOTH themes with the toolbar's Theme control.

type TeamBoardData = components["schemas"]["TeamBoard"];

const aLoadedTeam: TeamBoardData = {
  as_of: "2026-08-31T09:00:00Z",
  members: [
    {
      user_id: "00000000-0000-4000-8000-000000000001",
      display_name: "Lena Fischer",
      counts: { waiting: 14, at_risk: 3, overdue: 6, promises_due: 2 },
    },
    {
      user_id: "00000000-0000-4000-8000-000000000002",
      display_name: "Marc Weber",
      counts: { waiting: 2, at_risk: 0, overdue: 0, promises_due: 0 },
    },
    {
      user_id: "00000000-0000-4000-8000-000000000003",
      display_name: "Sofia Ruiz",
      counts: { waiting: 0, at_risk: 1, overdue: 0, promises_due: 4 },
    },
  ],
  unassigned: { waiting: 3, at_risk: 0, overdue: 1, promises_due: 0 },
  truncated: false,
};

function stubBoard(answer: () => Promise<Response>) {
  globalThis.fetch = (async (input: RequestInfo | URL): Promise<Response> => {
    const url = String(input instanceof Request ? input.url : input);
    return url.includes("/worklist/team")
      ? answer()
      : jsonResponse({ data: [] });
  }) as typeof fetch;
}

function frame(answer: () => Promise<Response>) {
  stubBoard(answer);
  return (
    <StoryProviders>
      <TeamBoard onOwner={() => {}} onUnassigned={() => {}} />
    </StoryProviders>
  );
}

const meta: Meta<typeof TeamBoard> = {
  title: "Records/Worklist/Team board",
  component: TeamBoard,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof TeamBoard>;

/** A team with somebody carrying more than the rest, and a pile nobody owns
 *  under them — the unowned work rides as a row rather than beside the table,
 *  because it is the same question asked of the one holder who is not a
 *  person. */
export const ALoadedTeam: Story = {
  render: () => frame(async () => jsonResponse(aLoadedTeam)),
};

/** Every count read to its bound, so the figures above are floors. */
export const CountedToTheBound: Story = {
  render: () =>
    frame(async () => jsonResponse({ ...aLoadedTeam, truncated: true })),
};

/** The board could not be read. It says so and offers the retry; it never
 *  reads as a team carrying nothing. */
export const Unavailable: Story = {
  render: () =>
    frame(async () => jsonResponse({ title: "Not permitted" }, 403)),
};
