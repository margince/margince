// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Who on the team is carrying what.
//
// The queue below this ranks ONE person's day, so it cannot answer "who is
// drowning": its per-user sources were never read for anybody else. This is
// counts instead, and pressing a row opens that person's own day — which is the
// whole point of showing counts rather than rows. The board is where a lead
// decides who to look at; the queue is where they look.

import { DataTable, Disclosure, SurfaceState } from "../design-system/atoms";
import { useT } from "../i18n";
import { useTeamBoard, type TeamBoardMember } from "./worklist.queries";

// A row of the board, with the unassigned pile carried as one of them.
//
// It rides in the same shape rather than beside the table, because it is the
// same question — how much work is sitting there — asked of the one holder who
// is not a person. Its id is empty, which is what tells the press there is
// nobody's day to open.
type BoardRow = Readonly<{
  id: string;
  name: string;
  waiting: number;
  atRisk: number;
  overdue: number;
}>;

function rowsOf(
  members: readonly TeamBoardMember[],
  unassigned: { waiting: number; at_risk: number; overdue: number },
  unassignedLabel: string,
): BoardRow[] {
  const rows = members.map((member) => ({
    id: member.user_id,
    name: member.display_name,
    waiting: member.counts.waiting,
    atRisk: member.counts.at_risk,
    overdue: member.counts.overdue,
  }));
  // Last, and only when it holds something. An always-drawn zero row would put
  // a permanent line under every team that has nothing unowned, and the reader
  // would learn to skip the place the unowned customer eventually appears.
  const nobody =
    unassigned.waiting + unassigned.at_risk + unassigned.overdue > 0;
  return nobody
    ? [
        ...rows,
        {
          id: "",
          name: unassignedLabel,
          waiting: unassigned.waiting,
          atRisk: unassigned.at_risk,
          overdue: unassigned.overdue,
        },
      ]
    : rows;
}

// A count of zero reads as a dash.
//
// "0" and "—" say different things at a glance down a column: a reader scanning
// for who is loaded wants the numbers to be the only digits on the page, and a
// column of zeros hides the one 14 among them.
function count(value: number) {
  return value === 0 ? "—" : String(value);
}

// The team's load, above the reader's own queue.
//
// Behind a Disclosure rather than always open: it is the lead's second
// question. Their own day is still what they came for, and a table of five
// colleagues above it would push the work they are answerable for off the top
// of the screen every morning.
export function TeamBoard({
  onOwner,
}: Readonly<{ onOwner: (userId: string) => void }>) {
  const t = useT();
  const board = useTeamBoard(true);
  // A board that could not be read says so. It never reads as an empty team:
  // the server refuses rather than answering zeros, and a surface that drew the
  // refusal as "nobody is carrying anything" would be the same lie one lane
  // further out.
  const state = board.isPending
    ? "loading"
    : board.isError
      ? "unavailable"
      : "ready";
  return (
    <Disclosure summary={t("worklist.board.title")}>
      <SurfaceState
        state={state}
        emptyLabel={t("worklist.board.empty")}
        loadingLabel={t("worklist.board.loading")}
        detail={{ onRetry: () => void board.refetch() }}
      >
        {board.data && (
          <>
            <DataTable
              label={t("worklist.board.title")}
              rows={rowsOf(
                board.data.members,
                board.data.unassigned,
                t("worklist.board.nobody"),
              )}
              rowKey={(row) => row.id || "unassigned"}
              // Pressing a row opens that person's day. The unassigned row has
              // no day to open, so it is not a link — a row that looks
              // pressable and does nothing is worse than a plain one.
              onRowClick={(row) => {
                if (row.id !== "") {
                  onOwner(row.id);
                }
              }}
              columns={[
                {
                  key: "name",
                  header: t("worklist.board.member"),
                  render: (row) => row.name,
                },
                {
                  key: "waiting",
                  header: t("worklist.board.waiting"),
                  render: (row) => count(row.waiting),
                },
                {
                  key: "at_risk",
                  header: t("worklist.board.atRisk"),
                  render: (row) => count(row.atRisk),
                },
                {
                  key: "overdue",
                  header: t("worklist.board.overdue"),
                  render: (row) => count(row.overdue),
                },
              ]}
            />
            {/* A count read to its bound is a FLOOR, and saying so is the whole
                reason the server sends the flag. A lead told "3" over a figure
                that is really 3-or-more will not go looking, which is the one
                direction this surface must not get wrong. */}
            {board.data.truncated && (
              <p className="t-meta">{t("worklist.board.truncated")}</p>
            )}
          </>
        )}
      </SurfaceState>
    </Disclosure>
  );
}
