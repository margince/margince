// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { BriefTeamBoard } from "./brief.teamboard";
import { jsonResponse, StoryProviders } from "./story-utils";

// THE TEAM BOARD AS BRIEF DRAWS IT — the same panel the Worklist renders, on
// the page a lead opens first.
//
// What is worth reading here is not the table, which the Worklist's own frames
// already document. It is the two things this wrapper decides: that the board
// is drawn at all only where the reader's scope reaches a team, and that a row
// hands the reader to a queue rather than narrowing one in place, because Brief
// has no queue of its own to narrow.
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

function frame(offered: boolean, answer: () => Promise<Response>) {
  stubBoard(answer);
  return (
    <StoryProviders>
      <BriefTeamBoard offered={offered} />
    </StoryProviders>
  );
}

const meta: Meta<typeof BriefTeamBoard> = {
  title: "Shell/Brief team board",
  component: BriefTeamBoard,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof BriefTeamBoard>;

/** A lead's morning: an open titled panel among the panels around it, with the
 *  pile nobody owns riding as a row of the same table. */
export const ALoadedTeam: Story = {
  render: () => frame(true, async () => jsonResponse(aLoadedTeam)),
};

/** The board could not be read. It says so and offers the retry; it never reads
 *  as a team carrying nothing. */
export const Unavailable: Story = {
  render: () =>
    frame(true, async () => jsonResponse({ title: "Not permitted" }, 403)),
};

/** A reader whose scope reaches no team. Nothing is drawn and nothing is asked
 *  for — a control on a tier the server refuses is a control that exists to
 *  fail — so this frame is deliberately blank. */
export const NoTeamToRead: Story = {
  render: () => frame(false, async () => jsonResponse(aLoadedTeam)),
};
