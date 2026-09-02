// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { navigate } from "../app/router";
import { TeamBoard } from "./worklist.board";
import { UNASSIGNED } from "./worklist.queries";

// Who on the team is carrying what, on the page a lead opens first.
//
// The SAME component the Worklist draws, on the same query key — not a second
// table over the same counts. What differs is only where a row goes: the
// Worklist can narrow itself in place because the owner is its own state, and
// Home has no queue to narrow, so a row hands the reader to the queue that does.

/**
 * The team board on Home, for a reader whose scope reaches a team.
 *
 * `offered` is read off the worklist's `scope_options` by the caller, which is
 * the same gate the Worklist's own board uses. The board is drawn on the tier
 * the server admits it on, so the control and the refusal cannot disagree.
 */
export function HomeTeamBoard({ offered }: Readonly<{ offered: boolean }>) {
  if (!offered) {
    return null;
  }
  return (
    <TeamBoard
      // Whose day, in the address. `#/worklist/<userId>` opens that person's
      // queue directly — without it a row could only reach the Worklist and
      // leave the reader to pick the same person a second time, which is a row
      // that answers a question by asking it again.
      onOwner={(userId) => navigate({ screen: "worklist", id: userId })}
      // The unassigned pile has no person to open, so the same segment carries
      // the scope word instead. Both rows are doors: one to a colleague's day,
      // one to the work that reached nobody.
      onUnassigned={() => navigate({ screen: "worklist", id: UNASSIGNED })}
    />
  );
}
