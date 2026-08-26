import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

// Where each person at the account stands with us, and which of them is worth
// writing to next.
//
// The people list on its own answered "who works there". Before reaching out
// the rep is asking something else: who here has ever answered, who has gone
// quiet on me, and who have I never tried. Those are three different next
// moves and the list rendered them identically.

type Contact = NonNullable<
  components["schemas"]["Organization360"]["people"]
>["data"][number];

/**
 * Reach is one contact's state, in the order a rep triages them.
 *
 *   answered   — they have written back inside the window. The way in.
 *   silent     — we have written and had nothing back. Following up again is
 *                a decision, not a default.
 *   untried    — nobody has written to them at all. Free to approach, and the
 *                most commonly missed opportunity on a stalled account.
 *
 * `untried` is deliberately not merged into `silent`: "no reply" and "never
 * asked" look the same in a contact list and call for opposite actions.
 */
export type Reach = "answered" | "silent" | "untried";

export function reachOf(contact: Contact): Reach {
  const strength = contact.strength;
  if ((strength.inbound_90d ?? 0) > 0) {
    return "answered";
  }
  return (strength.outbound_90d ?? 0) > 0 ? "silent" : "untried";
}

const REACH_LABELS: Record<Reach, MessageKey> = {
  answered: "co.reach.answered",
  silent: "co.reach.silent",
  untried: "co.reach.untried",
};

export function reachLabelKey(reach: Reach): MessageKey {
  return REACH_LABELS[reach];
}

/**
 * REACH_ORDER puts the people worth acting on first.
 *
 * Whoever has answered leads, because they are the way in. Untried comes next:
 * on an account where everyone has gone quiet, the person nobody has written
 * to is the only move left that is not a fourth follow-up.
 */
const REACH_ORDER: Record<Reach, number> = {
  answered: 0,
  untried: 1,
  silent: 2,
};

/** byReach sorts contacts into triage order, then by strength within a state. */
export function byReach(a: Contact, b: Contact): number {
  const rank = REACH_ORDER[reachOf(a)] - REACH_ORDER[reachOf(b)];
  return rank !== 0 ? rank : b.strength.score - a.strength.score;
}

/**
 * ROLES_WORTH_NAMING is the part of a buying committee whose absence is worth
 * reporting, in the order a deal needs them.
 *
 * Not the whole stakeholder vocabulary: `user` and `influencer` are useful to
 * record and unremarkable to be missing, and a gap list that names everything
 * names nothing.
 */
const ROLES_WORTH_NAMING = ["champion", "economic_buyer"] as const;

export type CommitteeRole = (typeof ROLES_WORTH_NAMING)[number];

const ROLE_LABELS: Record<CommitteeRole, MessageKey> = {
  champion: "co.role.champion",
  economic_buyer: "co.role.economic_buyer",
};

export function roleLabelKey(role: CommitteeRole): MessageKey {
  return ROLE_LABELS[role];
}

/** One open deal, and the committee roles nobody holds ON IT. */
export type DealGap = {
  dealId: string;
  dealName: string;
  missing: CommitteeRole[];
};

/**
 * missingRolesByDeal reports, for each of the account's OPEN deals, which of
 * the named committee roles nobody holds on THAT deal.
 *
 * Per deal, because a role is missing from the deal that lacks it. Taking the
 * union across the account answered a question nobody asked: deal A has a
 * champion but no economic buyer, deal B has an economic buyer but no champion,
 * and the union covers both roles — so a two-deal account with a gap on each
 * reported no gap at all, and the callout that reads this said nothing on
 * exactly the accounts that needed it.
 *
 * Scoped to open deals because a champion on a deal that closed last year says
 * nothing about the one running now.
 *
 * `incomplete` says the caller could not see the whole picture — contacts past
 * the first page, contacts withheld by the reader's grants, or open deals past
 * their own first page. It returns nothing at all in that case, because "nobody
 * is champion" is a claim about EVERY contact on the deal: the twenty-sixth
 * contact is exactly where the champion would be, and a role held on a deal
 * this page did not list reads as a role nobody holds. A partial answer here is
 * worse than none — the reader cannot tell which one they got.
 */
export function missingRolesByDeal(
  contacts: readonly Contact[],
  openDeals: readonly { id: string; name: string }[],
  incomplete: boolean,
): DealGap[] {
  if (incomplete) {
    return [];
  }
  const heldByDeal = new Map<string, Set<string>>();
  for (const contact of contacts) {
    for (const role of contact.deal_roles) {
      const held = heldByDeal.get(role.deal_id) ?? new Set<string>();
      held.add(role.role);
      heldByDeal.set(role.deal_id, held);
    }
  }
  const gaps: DealGap[] = [];
  for (const deal of openDeals) {
    const held = heldByDeal.get(deal.id) ?? new Set<string>();
    const missing = ROLES_WORTH_NAMING.filter((role) => !held.has(role));
    if (missing.length > 0) {
      gaps.push({ dealId: deal.id, dealName: deal.name, missing });
    }
  }
  return gaps;
}

/**
 * countGaps is what the coverage summary line reports: how many (deal, role)
 * pairs are unfilled.
 *
 * The number the summary used to carry was how many ROLE TYPES were missing
 * account-wide, which is at most two however many deals are short of them.
 * Counting the pairs is the only reading under which "4 role gaps" on a
 * two-deal account is true.
 */
export function countGaps(gaps: readonly DealGap[]): number {
  return gaps.reduce((total, gap) => total + gap.missing.length, 0);
}
