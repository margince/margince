// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The three conversations a lead should have this morning.
//
// The board above says who is carrying what; it does not say what to DO about
// it. A lead reading five columns across six people is doing arithmetic before
// they can act, and the person most in trouble is not always the one with the
// biggest number — a rep with fourteen waiting customers and nothing overdue is
// having a busy week, and one with four waiting and six broken promises is
// losing trust.
//
// DERIVED, not asked for. Every move here comes from counts the board already
// fetched, so this adds no request and cannot disagree with the table it sits
// under. There is no model in it: a suggestion a lead cannot check is a
// suggestion they will stop reading, and each line carries the number it was
// drawn from.

import { Callout } from "../design-system/callout";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { TeamBoardMember } from "./worklist.queries";

// How many moves a morning gets.
//
// Three, because this is a list a lead acts on before their own day starts. A
// surface that named every teammate with a number over a threshold would be the
// board again, sorted differently — and a lead who cannot get through it learns
// to skip it, which costs them the one line that mattered.
export const COACHING_MOVES = 3;

// What each threshold means, and why it is where it is.
//
// These are the counts at which a number stops being a busy week and starts
// being a person who needs help. They are deliberately not configurable yet:
// one shipped set that a lead can argue with beats a settings page nobody fills
// in, and the argument is what tells us where they really belong.
const TOO_MANY_OVERDUE = 3;
const TOO_MANY_WAITING = 5;
const ANY_BROKEN_PROMISE = 1;

// A move names a person, what is happening, and the number behind it.
type CoachingMove = Readonly<{
  ownerId: string;
  name: string;
  kind: "overdue" | "waiting" | "promises";
  evidence: number;
}>;

// movesFor picks the moves worth making, most urgent kind first.
//
// ORDERED BY KIND, not by size, and that is the judgement in this file. A
// broken promise outranks a big queue because the customer already noticed: a
// rep behind on tasks is late, a rep who said "I'll send it Tuesday" and did
// not has spent something that has to be earned back.
//
// One move per person. A teammate over every threshold has one problem — too
// much work — and three lines about them would push two other people off a list
// of three.
export function movesFor(members: readonly TeamBoardMember[]): CoachingMove[] {
  const moves: CoachingMove[] = [];
  const named = new Set<string>();
  const take = (
    member: TeamBoardMember,
    kind: CoachingMove["kind"],
    evidence: number,
  ) => {
    if (named.has(member.user_id)) {
      return;
    }
    named.add(member.user_id);
    moves.push({
      ownerId: member.user_id,
      name: member.display_name,
      kind,
      evidence,
    });
  };
  // Promises first, then the queue, then the task list — the order above.
  for (const member of members) {
    if (member.counts.promises_due >= ANY_BROKEN_PROMISE) {
      take(member, "promises", member.counts.promises_due);
    }
  }
  for (const member of members) {
    if (member.counts.waiting > TOO_MANY_WAITING) {
      take(member, "waiting", member.counts.waiting);
    }
  }
  for (const member of members) {
    if (member.counts.overdue > TOO_MANY_OVERDUE) {
      take(member, "overdue", member.counts.overdue);
    }
  }
  return moves.slice(0, COACHING_MOVES);
}

// The moves, above the board they are drawn from.
//
// Nothing at all when there is nothing to say. An empty "no coaching needed"
// panel is a line a lead reads every morning to learn nothing, and the mornings
// it does have something to say are the ones it would then be skipped on.
export function CoachingMoves({
  members,
  onOwner,
}: Readonly<{
  members: readonly TeamBoardMember[];
  onOwner: (userId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const moves = movesFor(members);
  if (moves.length === 0) {
    return null;
  }
  return (
    <Callout tone="info" title={t("worklist.coaching.title")}>
      <ul className="worklist-coaching">
        {moves.map((move) => (
          <li key={move.ownerId}>
            <button
              type="button"
              className="link-button"
              onClick={() => onOwner(move.ownerId)}
            >
              {t(`worklist.coaching.${move.kind}` as const, {
                name: move.name,
                count: formatNumber(move.evidence, locale),
              })}
            </button>
          </li>
        ))}
      </ul>
    </Callout>
  );
}
