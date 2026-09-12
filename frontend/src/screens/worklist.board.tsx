// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Who on the team is carrying what.
//
// The queue below this ranks ONE contact's day, so it cannot answer "who is
// drowning": its per-user sources were never read for anybody else. This is
// counts instead, and pressing a row opens that contact's own day — which is the
// whole point of showing counts rather than rows. The board is where a lead
// decides who to look at; the queue is where they look.

import { DataTable } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { useT } from "../i18n";
import { CoachingMoves } from "./worklist.coaching";
import { AFTER_THE_DAY } from "./worklist.layout";
import { type TeamBoardMember, useTeamBoard } from "./worklist.queries";

// A row of the board, with the unassigned pile carried as one of them.
//
// It rides in the same shape rather than beside the table, because it is the
// same question — how much work is sitting there — asked of the one holder who
// is not a contact. Its id is empty, and pressing it opens the `unassigned`
// scope rather than a contact's day: the work is real and somebody has to pick
// it up, so the row leads to the queue that shows it.
type BoardRow = Readonly<{
  id: string;
  name: string;
  waiting: number;
  atRisk: number;
  overdue: number;
  promises: number;
}>;

function rowsOf(
  members: readonly TeamBoardMember[],
  unassigned: {
    waiting: number;
    at_risk: number;
    overdue: number;
    promises_due: number;
  },
  unassignedLabel: string,
): BoardRow[] {
  const rows = members.map((member) => ({
    id: member.user_id,
    name: member.display_name,
    waiting: member.counts.waiting,
    atRisk: member.counts.at_risk,
    overdue: member.counts.overdue,
    promises: member.counts.promises_due,
  }));
  // Last, and only when it holds something. An always-drawn zero row would put
  // a permanent line under every team that has nothing unowned, and the reader
  // would learn to skip the place the unowned customer eventually appears.
  // Summed through the same normaliser the cells render through, so a figure
  // the server did not send behaves identically in both places. A raw addition
  // gives NaN, and NaN > 0 is false — one missing column would silently take
  // the whole unassigned pile off a lead's screen, which is the work with
  // nobody on it.
  const nobody =
    [
      unassigned.waiting,
      unassigned.at_risk,
      unassigned.overdue,
      unassigned.promises_due,
    ].reduce((total, value) => total + held(value), 0) > 0;
  return nobody
    ? [
        ...rows,
        {
          id: "",
          name: unassignedLabel,
          waiting: unassigned.waiting,
          atRisk: unassigned.at_risk,
          overdue: unassigned.overdue,
          promises: unassigned.promises_due,
        },
      ]
    : rows;
}

// How much a count actually holds, with a missing figure read as none.
//
// A count is required on the wire, so an absent one is version skew rather than
// a state the server means. Reading it as zero is the honest fallback for a
// TABLE OF NUMBERS: the alternative — refusing the whole board because one
// column of one row is missing — takes away four columns that arrived fine, on
// a surface whose job is to tell a lead where to look.
//
// It is a floor, and `truncated` already tells the reader the figures may be.
function held(value: number | undefined) {
  return value ?? 0;
}

// A count of zero reads as a dash.
//
// "0" and "—" say different things at a glance down a column: a reader scanning
// for who is loaded wants the numbers to be the only digits on the page, and a
// column of zeros hides the one 14 among them.
//
// A MISSING count reads as a dash too, through held: "undefined" in a numeric
// column is the one rendering that tells a lead nothing and looks like a bug.
function count(value: number | undefined) {
  return held(value) === 0 ? "—" : String(held(value));
}

// The team's load, above the reader's own queue.
//
// A titled panel, open, on both surfaces that draw it. This is a reading a lead
// came to either page FOR, so it stands open like the panels beside it rather
// than behind a control — a table of counts folded away reads as a block that
// failed to load, not as one waiting to be asked for.
export function TeamBoard({
  onOwner,
  onUnassigned,
}: Readonly<{
  onOwner: (userId: string) => void;
  onUnassigned: () => void;
}>) {
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
    <Panel className={AFTER_THE_DAY} title={t("worklist.board.title")}>
      {/* The table carries its own cell padding but not the panel's inset, so
          it sits in a `PanelBody` with the coaching lines and the floor caveat
          rather than full-bleed against the panel's own edges. */}
      <PanelBody>
        <SurfaceState
          state={state}
          emptyLabel={t("worklist.board.empty")}
          loadingLabel={t("worklist.board.loading")}
          detail={{ onRetry: () => void board.refetch() }}
        >
          {board.data && (
            <>
              {/* WHAT to do about the table, above the table. A lead reading
                  five columns across six contacts is doing arithmetic before they
                  can act; these are the same numbers with the arithmetic done.
                  Drawn from board.data, so they add no request and cannot
                  disagree with the rows beneath them. */}
              <CoachingMoves members={board.data.members} onOwner={onOwner} />
              {/* A wrapper the stylesheet can reach, and the reason is the
                  phone: five uppercase headings each end up narrower than
                  their own longest word at 390px, and a word wider than its
                  column breaks mid-word. `DataTable` takes no class, so the
                  screen's rule needs an element of its own to hang from. */}
              <div className="worklist-board-table">
                <DataTable
                  label={t("worklist.board.title")}
                  rows={rowsOf(
                    board.data.members,
                    board.data.unassigned,
                    t("worklist.board.nobody"),
                  )}
                  rowKey={(row) => row.id || "unassigned"}
                  // Every row goes somewhere: a contact's row opens their day, and
                  // the unassigned row opens the scope that holds unowned work.
                  //
                  // DataTable draws every row as pressable once onRowClick is set —
                  // it has no per-row opt-out — so a row that led nowhere would look
                  // exactly like one that led somewhere and do nothing when pressed.
                  onRowClick={(row) => {
                    if (row.id === "") {
                      onUnassigned();
                      return;
                    }
                    onOwner(row.id);
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
                    // The column the coaching lines above are drawn from. Without
                    // it a lead reads "Ana owes 3 promises" with nowhere on the
                    // page to check it — a suggestion they cannot verify is one
                    // they stop trusting.
                    {
                      key: "promises_due",
                      header: t("worklist.board.promises"),
                      render: (row) => count(row.promises),
                    },
                  ]}
                />
              </div>
              {/* A count read to its bound is a FLOOR, and saying so is the
                  whole reason the server sends the flag. A lead told "3" over a
                  figure that is really 3-or-more will not go looking, which is
                  the one direction this surface must not get wrong. */}
              {board.data.truncated && (
                <p className="t-caption">{t("worklist.board.truncated")}</p>
              )}
            </>
          )}
        </SurfaceState>
      </PanelBody>
    </Panel>
  );
}
