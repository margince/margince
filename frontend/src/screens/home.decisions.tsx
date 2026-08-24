// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { approvalDotTier, useAgentTierMap } from "../app/autonomy";
import { navigate } from "../app/router";
import type { DecisionCardLabels } from "../design-system/decisioncard";
import {
  decisionExpiryMs,
  decisionLapsed,
} from "../design-system/decisioncard";
import {
  DecisionDeck,
  type DecisionDeckItem,
  type DecisionDeckLabels,
  type StagedDecision,
} from "../design-system/decisiondeck";
import type { SectionState } from "../design-system/surfacestate";
import { AutonomyDot } from "../design-system/trust";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { approvalKindLabel } from "./approvalkind";
import {
  isAlreadyDecided,
  ProblemError,
  problemMessageOf,
  provenanceOf,
  throwProblem,
  useViewerId,
} from "./common";
import { confidenceLevel, RowStatusChip } from "./inbox";

// The decisions half of Home: the deck, its tray, and the one act that sends
// what is in it.
//
// Staging is local and the commit is separate, which is the whole design rather
// than a flourish: `approvals/service.go` states that a committed decision is
// deliberately un-undoable, and rejecting is not an undo either, so the tray IS
// the undo a swipe would otherwise not have.

type Approval = components["schemas"]["Approval"];

/** Every approval one deck item answers for. */
function approvalsOf(item: DecisionDeckItem): readonly Approval[] {
  return item.kind === "single" ? [item.approval] : item.members;
}

/** The deck's vocabulary, in this surface's own words. */
function deckLabels(
  t: (key: MessageKey, params?: Record<string, string | number>) => string,
  locale: Parameters<typeof formatDateTime>[1],
): DecisionDeckLabels {
  const card: DecisionCardLabels = {
    accept: t("trust.accept"),
    edit: t("trust.edit"),
    reject: t("inbox.reject"),
    // The deck is the one surface where "later" is a real answer: it is the top
    // of the pile, not the pile itself, and the full queue is one click away.
    skip: t("home.deck.later"),
    expired: t("inbox.expired"),
    draftSubject: t("inbox.draftSubject"),
    draftBody: t("inbox.draftBody"),
    showMore: t("home.deck.showMore"),
    showLess: t("home.deck.showLess"),
    noContent: t("common.empty"),
  };
  return {
    card,
    deckLabel: t("home.panel.decisions"),
    viewLabel: t("home.deck.view"),
    viewDeck: t("home.deck.viewDeck"),
    viewList: t("home.deck.viewList"),
    keys: t("home.deck.keys"),
    behind: (count) =>
      t(count === 1 ? "home.deck.behind.one" : "home.deck.behind.other", {
        count,
      }),
    staged: (count) =>
      t(count === 1 ? "home.deck.staged.one" : "home.deck.staged.other", {
        count,
      }),
    commit: t("home.deck.commit"),
    unstage: t("home.deck.unstage"),
    clearedTitle: t("home.deck.clearedTitle"),
    cleared: (count) =>
      t(count === 1 ? "home.deck.cleared.one" : "home.deck.cleared.other", {
        count,
      }),
    clearedTime: (atMs) =>
      t("home.deck.clearedTime", {
        at: formatDateTime(new Date(atMs).toISOString(), locale, viewerZone()),
      }),
    empty: t("home.deck.empty"),
    bundleSummary: (members) =>
      t("home.deck.bundleSummary", { count: members }),
    bundleMembers: (members) =>
      t("home.deck.bundleMembers", { count: members }),
  };
}

/** What a commit sent, and what came back for each item in it. */
type CommitResult = Readonly<{
  /** The first minted token, which is the one the modal can show. */
  token: { approvalId: string; token: string } | null;
  /** At least one item had already been decided by somebody else. */
  alreadyDecided: boolean;
  /** Items the reader staged for editing: the deck cannot edit, the queue can. */
  edits: number;
  /**
   * The first item that could not be sent, if any.
   *
   * Carried in the RESULT rather than thrown, and that is the whole point of it
   * being here: a throw from the third item took the first item's approval token
   * with it. The token is minted once, it is the only copy the reader gets, and
   * the effect it belongs to had already executed — so the commit reports what
   * landed AND what did not, instead of reporting the failure and losing the
   * rest.
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
): Promise<{ token: string | null; alreadyDecided: boolean }> {
  const path =
    verdict === "accept" ? "/approvals/{id}/approve" : "/approvals/{id}/reject";
  try {
    const { data, error } = await api.POST(path, {
      params: { path: { id: approval.id } },
      ...(verdict === "reject" ? { body: { reason: "" } } : {}),
    });
    if (error) {
      throwProblem(error);
    }
    return { token: data?.approval_token ?? null, alreadyDecided: false };
  } catch (error) {
    if (error instanceof ProblemError && isAlreadyDecided(error.problem)) {
      return { token: null, alreadyDecided: true };
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
): Promise<Pick<CommitResult, "token" | "alreadyDecided">> {
  let token: CommitResult["token"] = null;
  let alreadyDecided = false;
  for (const approval of approvals) {
    const outcome = await sendOne(approval, verdict);
    if (outcome.token !== null && token === null) {
      token = { approvalId: approval.id, token: outcome.token };
    }
    alreadyDecided = alreadyDecided || outcome.alreadyDecided;
  }
  return { token, alreadyDecided };
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
  // Decisions screen's; what Home needs from it is whether anything was already
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
): Promise<Pick<CommitResult, "token" | "alreadyDecided">> {
  if (item.kind === "bundle") {
    const outcome = await sendBundle(item.bundleId, verdict);
    return { token: null, alreadyDecided: outcome.alreadyDecided };
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
async function commitTray(input: {
  staged: readonly StagedDecision[];
  items: readonly DecisionDeckItem[];
}): Promise<CommitResult> {
  const byId = new Map(input.items.map((item) => [item.id, item]));
  let token: CommitResult["token"] = null;
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
      token = token ?? outcome.token;
      alreadyDecided = alreadyDecided || outcome.alreadyDecided;
    } catch (error) {
      // The FIRST failure is kept and the loop stops: these are outbound
      // effects, and carrying on after one refusal sends the rest against
      // whatever made this one fail. What already went, went — and its token
      // travels back with the failure rather than being thrown away with it.
      failure = error;
      break;
    }
  }
  return { token, alreadyDecided, edits, failure };
}

/** The deck and its tray: the staged verdicts, and the one act that sends them. */
export function DecisionsSection({
  items,
  nowMs,
  state,
  onApproved,
  onAlreadyDecided,
}: Readonly<{
  items: readonly DecisionDeckItem[];
  nowMs: number;
  /** What the read behind the queue says. `ready` defers to what the deck can
   *  see; a failure or a wait is the deck's to draw, not the column's. */
  state: SectionState;
  onApproved: (approvalId: string, token: string) => void;
  onAlreadyDecided: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const viewerId = useViewerId();
  const tierMap = useAgentTierMap();
  // What stopped a commit that had already sent something. Screen state rather
  // than the mutation's error, because the mutation SUCCEEDED — it carried the
  // outcomes of the items that went, and one of them may be a token.
  const [failure, setFailure] = useState<string | null>(null);

  const commit = useMutation({
    // The staged verdicts and the items they answer for BOTH arrive as
    // variables. A mutationFn that closed over them would run against whatever
    // the last render left behind, which on this page is a queue that has just
    // been invalidated.
    mutationFn: commitTray,
    onSuccess: (result) => {
      // The token goes up FIRST: the invalidation below re-renders the deck, and
      // a token shown by something that has just unmounted is a token nobody
      // ever reads.
      if (result.token) {
        onApproved(result.token.approvalId, result.token.token);
      }
      if (result.alreadyDecided) {
        onAlreadyDecided();
      }
      queryClient.invalidateQueries({ queryKey: ["approvals"] });
      if (result.failure) {
        // Reported after the outcomes that DID land, and as a failure: the tray
        // keeps what it still holds and the notice under it says what stopped.
        setFailure(problemMessageOf(result.failure, t));
        return;
      }
      setFailure(null);
      // The full queue is where an edit's form lives, so the reader is taken
      // there rather than told the deck cannot do it — but NOT over the top of a
      // token. A commit carrying both an approval and an edit mints a one-time
      // token that is shown once and cannot be fetched again, and navigating
      // unmounted the surface holding it before anybody could read it. With a
      // token on screen the reader keeps it and the edit keeps its place in the
      // queue, one click away in the sidebar: an edit deferred by a screen is
      // recoverable, a token taken off the screen is not.
      if (result.edits > 0 && !result.token) {
        navigate({ screen: "inbox" });
      }
    },
  });

  // A partial failure is a failed commit as far as the tray is concerned: the
  // verdicts it still holds have not gone anywhere, so the tray keeps them and
  // the control stays pressable. `commit.isError` covers a request that never
  // returned; `failure` covers one that returned for an earlier item and then
  // refused a later one.
  const commitState = commit.isPending
    ? "sending"
    : commit.isError || failure !== null
      ? "failed"
      : "idle";
  const notice = commit.isError ? problemMessageOf(commit.error, t) : failure;

  return (
    <section id="home-decisions" aria-label={t("home.panel.decisions")}>
      {/* The title goes THROUGH the deck: it shares the row the Deck/List
          toggle is on, so the column's first block says what it is on the same
          line that says how it is drawn. */}
      <DecisionDeck
        items={items}
        now={nowMs}
        title={t("home.panel.decisions")}
        labels={deckLabels(t, locale)}
        state={state}
        loadingLabel={t("home.panel.decisions")}
        commitState={commitState}
        notice={
          notice ? <p className="home-error t-caption">{notice}</p> : undefined
        }
        onCommit={(staged) => commit.mutate({ staged, items })}
        // The three facts a reader needs BEFORE they say yes, and none of them
        // is the deck's to know: which agent tier staged this, what kind of act
        // it is, and how long it has left. `RowStatusChip` is the same chip the
        // Decisions row draws, so the two surfaces cannot disagree about a
        // deadline.
        chips={(approval) => ({
          meta: (
            <>
              <AutonomyDot tier={approvalDotTier(approval.kind, tierMap)} />
              <span className="t-small">
                {approvalKindLabel(approval.kind, t)}
              </span>
              <RowStatusChip
                decided={false}
                status={approval.status}
                expiresAtMs={decisionExpiryMs(approval)}
                isExpired={decisionLapsed(approval, nowMs)}
                now={nowMs}
              />
            </>
          ),
          provenance: provenanceOf(approval.proposed_by, viewerId),
          confidence: confidenceLevel(approval.confidence) ?? undefined,
        })}
      />
    </section>
  );
}
