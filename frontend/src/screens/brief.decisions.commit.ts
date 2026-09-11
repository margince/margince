// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { api } from "../api/client";
import type { components } from "../api/schema";
import type {
  DecisionDeckItem,
  StagedDecision,
} from "../design-system/decisiondeck";
import { isAlreadyDecided, ProblemError, throwProblem } from "./common";

// Sending the Brief's staging tray: what one verdict does, and what a whole
// tray does.
//
// Its own module because it is the half that TALKS TO THE SERVER, and the
// surface above it is the half that talks to the reader. Every rule here is
// about an effect that cannot be taken back — `approvals/service.go` states
// that a committed decision is deliberately un-undoable — and a reader checking
// which of those rules the tray keeps should not have to read a panel's labels
// to find them.
export type Approval = components["schemas"]["Approval"];

/** Every approval one deck item answers for. */
function approvalsOf(item: DecisionDeckItem): readonly Approval[] {
  return item.kind === "single" ? [item.approval] : item.members;
}

/** What a commit sent, and what came back for each item in it. */
export type CommitResult = Readonly<{
  /** At least one item had already been decided by somebody else. */
  alreadyDecided: boolean;
  /** Items the reader staged for editing: the deck cannot edit, the queue can. */
  edits: number;
  /**
   * The first item that could not be sent, if any.
   *
   * Carried in the RESULT rather than thrown: the items before the failure were
   * decided, and their effects have already executed. A throw would report the
   * failure and lose that, leaving the reader to guess which half of their tray
   * landed.
   */
  failure: unknown | null;
}>;

/**
 * One approval's verdict, sent.
 *
 * An already-decided 409 is not a failure of the commit: somebody else answered
 * this one first, which is news the reader is owed, and the rest of the tray
 * still deserves to go.
 */
async function sendOne(
  approval: Approval,
  verdict: "accept" | "reject",
): Promise<{ alreadyDecided: boolean }> {
  const path =
    verdict === "accept" ? "/approvals/{id}/approve" : "/approvals/{id}/reject";
  try {
    const { error } = await api.POST(path, {
      params: { path: { id: approval.id } },
      ...(verdict === "reject" ? { body: { reason: "" } } : {}),
    });
    if (error) {
      throwProblem(error);
    }
    return { alreadyDecided: false };
  } catch (error) {
    if (error instanceof ProblemError && isAlreadyDecided(error.problem)) {
      return { alreadyDecided: true };
    }
    throw error;
  }
}

/**
 * Every approval one deck item answers for, sent one at a time.
 *
 * Sequential on purpose. These are outbound effects — a staged send goes when it
 * is approved — and firing a dozen at once makes the failure of the fourth
 * unattributable.
 */
async function sendVerdict(
  approvals: readonly Approval[],
  verdict: "accept" | "reject",
): Promise<Pick<CommitResult, "alreadyDecided">> {
  let alreadyDecided = false;
  for (const approval of approvals) {
    const outcome = await sendOne(approval, verdict);
    alreadyDecided = alreadyDecided || outcome.alreadyDecided;
  }
  return { alreadyDecided };
}

/**
 * A bundle is decided as a unit, through its own endpoint: every still-pending
 * member, one call, one outcome per member.
 */
async function sendBundle(
  bundleId: string,
  verdict: "accept" | "reject",
): Promise<{ alreadyDecided: boolean }> {
  const path =
    verdict === "accept"
      ? "/approval-bundles/{bundle_id}/approve"
      : "/approval-bundles/{bundle_id}/reject";
  // No body at all on a rejection: the deck takes no reason, and an empty
  // string would be recorded as one the reader gave.
  const { data, error } = await api.POST(path, {
    params: { path: { bundle_id: bundleId } },
  });
  if (error) {
    throwProblem(error);
  }
  // Deciding a bundle is not all-or-nothing: the response reports each member,
  // and a member somebody else answered first comes back `already_decided`
  // rather than as an error. Reading `false` here regardless — which this did —
  // meant the deck reported a conflict for a single proposal and said nothing
  // about the same conflict inside a bundle. The full per-outcome report is the
  // Decisions screen's; what Brief needs from it is whether anything was already
  // settled.
  const members = data?.data ?? [];
  return {
    alreadyDecided: members.some(
      (member) => member.outcome === "already_decided",
    ),
  };
}

/** One staged verdict, sent the way its item is decided. */
async function sendStaged(
  item: DecisionDeckItem,
  verdict: "accept" | "reject",
): Promise<Pick<CommitResult, "alreadyDecided">> {
  if (item.kind === "bundle") {
    const outcome = await sendBundle(item.bundleId, verdict);
    return { alreadyDecided: outcome.alreadyDecided };
  }
  return sendVerdict(approvalsOf(item), verdict);
}

/**
 * Send the tray.
 *
 * A `skip` sends nothing: later means later, and the item is offered again next
 * time. An `edit` sends nothing either — an edited payload re-enters the
 * admission gate on the server, which is a form rather than a swipe — and is
 * counted so the caller can take the reader to where that form lives.
 */
export async function commitTray(input: {
  staged: readonly StagedDecision[];
  items: readonly DecisionDeckItem[];
}): Promise<CommitResult> {
  const byId = new Map(input.items.map((item) => [item.id, item]));
  let alreadyDecided = false;
  let edits = 0;
  let failure: unknown | null = null;
  for (const decision of input.staged) {
    const item = byId.get(decision.id);
    if (!item || decision.verdict === "skip") {
      continue;
    }
    if (decision.verdict === "edit") {
      edits += 1;
      continue;
    }
    try {
      const outcome = await sendStaged(item, decision.verdict);
      alreadyDecided = alreadyDecided || outcome.alreadyDecided;
    } catch (error) {
      // The FIRST failure is kept and the loop stops: these are outbound
      // effects, and carrying on after one refusal sends the rest against
      // whatever made this one fail. What already went, went, and the result
      // says so rather than the throw erasing it.
      failure = error;
      break;
    }
  }
  return { alreadyDecided, edits, failure };
}
