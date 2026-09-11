// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type DecisionApproval, decisionLapsed } from "./decisioncard";
import type { DecisionDeckItem, DeckVerdict } from "./decisiondeck";

// What an INPUT means, and what an ITEM can truthfully say about itself.
//
// Two readings the deck makes before it draws anything: a gesture or a key is
// turned into one verdict here, and a bundle is turned into the one member the
// card answers for plus the facts every member agrees on. Split from the deck
// because they are the vocabulary rather than the surface — `dragVerdict` and
// `keyVerdict` are the reason four inputs cannot disagree, and a reader
// checking that claim should not have to walk a 700-line component to do it.
/** How far a drag must travel before it is a verdict rather than a nudge. */
const DRAG_THRESHOLD_PX = 72;

/**
 * The verdict a finished drag means, or null for one that did not travel far
 * enough and springs back. The DOMINANT axis decides, so a diagonal drag is
 * whichever direction it mostly went rather than two verdicts at once.
 */
export function dragVerdict(dx: number, dy: number): DeckVerdict | null {
  const horizontal = Math.abs(dx) >= Math.abs(dy);
  const travel = horizontal ? Math.abs(dx) : Math.abs(dy);
  if (travel < DRAG_THRESHOLD_PX) {
    return null;
  }
  if (horizontal) {
    return dx > 0 ? "accept" : "reject";
  }
  return dy < 0 ? "edit" : "skip";
}

/**
 * The verdict a key means, or null for a key this deck does not claim. Exported
 * so the keyboard and the pointer are provably the same vocabulary rather than
 * two lists that happen to agree today.
 */
export function keyVerdict(key: string): DeckVerdict | null {
  if (key === "ArrowRight") {
    return "accept";
  }
  if (key === "ArrowLeft") {
    return "reject";
  }
  if (key === "ArrowUp") {
    return "edit";
  }
  return key === "ArrowDown" ? "skip" : null;
}

/**
 * The approval a card draws for one item.
 *
 * A bundle shows the first member that has NOT lapsed, and that choice is what
 * keeps the card's reading and the deck's Accept guard from contradicting each
 * other: the API decides every still-pending member in one call, so a bundle
 * whose oldest member ran out of time is still answerable, and a card drawing
 * that member would say "expired" over a decision the reader may still make.
 * With every member lapsed there is nothing to choose, and the first one is as
 * honest as any other.
 */
export function representative(
  item: DecisionDeckItem,
  now: number,
): DecisionApproval {
  if (item.kind === "single") {
    return item.approval;
  }
  return (
    item.members.find((member) => !decisionLapsed(member, now)) ??
    item.members[0]
  );
}

/** Whether there is anything left on this item to accept. */
export function itemLapsed(item: DecisionDeckItem, now: number): boolean {
  return decisionLapsed(representative(item, now), now);
}

/**
 * The facts a card may state about the WHOLE item — never the representative's
 * alone.
 *
 * Drawing a bundle from one member is right for what the card DECIDES and wrong
 * for what it CLAIMS. A ten-recipient send staged by two agents at two
 * confidences read as one agent at one confidence, and the reader answered all
 * ten on that reading. So a fact its members do not share is absent here rather
 * than sampled from one of them: an omitted chip is honest, a wrong one is not.
 *
 * Absent is also what a fact nobody recorded looks like, and that collapse is
 * deliberate — both mean "this card cannot say", which is the whole of what a
 * chip could truthfully report either way.
 */
export type DecisionSharedFacts = Readonly<{
  kind?: string;
  proposedBy?: string;
  confidence?: number;
}>;

/**
 * What every member of an item agrees on. A single agrees with itself, so it
 * carries its own facts whole.
 *
 * The chips are built from THIS rather than from the drawn approval, which is
 * what keeps the rule from being one a caller has to remember: a fact the
 * members disagree on is not merely discouraged as a chip, it is not there to
 * draw one from.
 */
export function sharedFacts(item: DecisionDeckItem): DecisionSharedFacts {
  const members = item.kind === "single" ? [item.approval] : item.members;
  return {
    kind: agreed(members, (member) => member.kind),
    proposedBy: agreed(members, (member) => member.proposed_by),
    confidence: agreed(members, (member) => member.confidence),
  };
}

/**
 * One fact across the members, or undefined where any two disagree.
 *
 * A member that never carried the fact needs no arm of its own: it reads as
 * absent, a set holding one absence and one value does not agree, and a set of
 * absences agrees on nothing a chip could print.
 */
function agreed<T>(
  members: readonly DecisionApproval[],
  read: (member: DecisionApproval) => T | null | undefined,
): T | undefined {
  const values = members.map(read);
  const first = values[0];
  return first != null && values.every((value) => value === first)
    ? first
    : undefined;
}
