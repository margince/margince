// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import {
  approvalDotTier,
  KIND_TO_VERB,
  useAgentTierMap,
} from "../app/autonomy";
import { navigate } from "../app/router";
import type {
  DecisionCardLabels,
  DecisionStatusLabels,
} from "../design-system/decisioncard";
import {
  DecisionStatusChip,
  DecisionToolChip,
} from "../design-system/decisioncard";
import {
  DecisionDeck,
  type DecisionDeckItem,
  type DecisionDeckLabels,
  type StagedDecision,
} from "../design-system/decisiondeck";
import type { SectionState } from "../design-system/surfacestate";
import { AutonomyDot, confidenceLevel } from "../design-system/trust";
import { formatDateTime, formatNumber } from "../format/format";
import { formatCountdown } from "../format/now";
import { viewerZone } from "../format/timezone";
import {
  type Locale,
  type PluralTranslator,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  approvalKindLabel,
  resolveDisplay,
  stagedDayFormatter,
} from "./approvalkind";
import {
  isAlreadyDecided,
  ProblemError,
  problemMessageOf,
  provenanceOf,
  throwProblem,
  useViewerId,
} from "./common";

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

// The status chip's words, in this surface's own voice. Spelled here rather
// than shared with the Decisions row for the same reason `deckLabels` is: what a
// countdown is CALLED belongs to the screen showing it, and the deck's own
// vocabulary is already divergent ("Later" is a verdict only it offers).
function statusLabels(t: Translator, locale: Locale): DecisionStatusLabels {
  return {
    expiresIn: (msRemaining) =>
      t("decision.expiresIn", {
        countdown: formatCountdown(msRemaining, t, locale),
      }),
    approved: t("decision.status.approved"),
    rejected: t("decision.status.rejected"),
    expired: t("decision.status.expired"),
  };
}

/** The deck's vocabulary, in this surface's own words. */
function deckLabels(
  t: (key: MessageKey, params?: Record<string, string>) => string,
  plural: PluralTranslator,
  locale: Parameters<typeof formatDateTime>[1],
): DecisionDeckLabels {
  const card: DecisionCardLabels = {
    accept: t("trust.accept"),
    edit: t("trust.edit"),
    reject: t("decision.reject"),
    // The deck is the one surface where "later" is a real answer: it is the top
    // of the pile, not the pile itself, and the full queue is one click away.
    skip: t("home.deck.later"),
    expired: t("decision.expired"),
    draftSubject: t("decision.draftSubject"),
    draftBody: t("decision.draftBody"),
    showMore: t("home.deck.showMore"),
    showLess: t("home.deck.showLess"),
    noContent: t("common.empty"),
    loading: t("home.panel.decisions"),
  };
  return {
    card,
    deckLabel: t("home.panel.decisions"),
    viewLabel: t("home.deck.view"),
    viewDeck: t("home.deck.viewDeck"),
    viewList: t("home.deck.viewList"),
    keys: t("home.deck.keys"),
    behind: (count) =>
      plural("home.deck.behind", count, {
        count: formatNumber(count, locale),
      }),
    staged: (count) =>
      plural("home.deck.staged", count, {
        count: formatNumber(count, locale),
      }),
    commit: t("home.deck.commit"),
    unstage: t("home.deck.unstage"),
    clearedTitle: t("home.deck.clearedTitle"),
    cleared: (count) =>
      plural("home.deck.cleared", count, {
        count: formatNumber(count, locale),
      }),
    clearedTime: (atMs) =>
      t("home.deck.clearedTime", {
        at: formatDateTime(new Date(atMs).toISOString(), locale, viewerZone()),
      }),
    empty: t("home.deck.empty"),
    bundleSummary: (members) =>
      t("home.deck.bundleSummary", { count: formatNumber(members, locale) }),
    bundleMembers: (members) =>
      t("home.deck.bundleMembers", { count: formatNumber(members, locale) }),
  };
}

/** What a commit sent, and what came back for each item in it. */
type CommitResult = Readonly<{
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
async function commitTray(input: {
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

/** The deck and its tray: the staged verdicts, and the one act that sends them. */
export function DecisionsSection({
  items,
  nowMs,
  state,
  onAlreadyDecided,
}: Readonly<{
  items: readonly DecisionDeckItem[];
  nowMs: number;
  /** What the read behind the queue says. `ready` defers to what the deck can
   *  see; a failure or a wait is the deck's to draw, not the column's. */
  state: SectionState;
  onAlreadyDecided: () => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const stagedDay = stagedDayFormatter(locale, viewerZone());
  const queryClient = useQueryClient();
  const viewerId = useViewerId();
  const tierMap = useAgentTierMap();
  // What stopped a commit that had already sent something. Screen state rather
  // than the mutation's error, because the mutation SUCCEEDED — it carried the
  // outcomes of the items that did go.
  const [failure, setFailure] = useState<string | null>(null);

  const commit = useMutation({
    // The staged verdicts and the items they answer for BOTH arrive as
    // variables. A mutationFn that closed over them would run against whatever
    // the last render left behind, which on this page is a queue that has just
    // been invalidated.
    mutationFn: commitTray,
    onSuccess: (result) => {
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
      // The full queue is where an edit's form lives, so a tray carrying one
      // takes the reader there rather than telling them the deck cannot do it.
      if (result.edits > 0) {
        navigate({ screen: "worklist" });
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
        labels={deckLabels(t, plural, locale)}
        state={state}
        loadingLabel={t("home.panel.decisions")}
        commitState={commitState}
        notice={
          notice ? <p className="home-error t-caption">{notice}</p> : undefined
        }
        onCommit={(staged) => commit.mutate({ staged, items })}
        // The four facts a reader needs BEFORE they say yes, and none of them
        // is the deck's to know: which agent tier staged this, what kind of act
        // it is, which tool produced it, and how long it has left. The chips are
        // the design system's, so the deck and the Decisions row cannot disagree
        // about a deadline.
        //
        // Every one of them is read off `shared` — what the item's members
        // AGREE on — rather than off the card's drawn approval, so a bundle
        // states one member's kind, tier, tool or provenance as the act's only
        // when it is every member's. The deadline is the exception and stays on
        // the drawn approval, because that is the member the deck's Accept
        // guard answers for.
        //
        // The body below is the drawn member's too, and honestly so: it is that
        // member's own summary and payload, standing beside a count and a list
        // of the siblings it was staged with. A chip makes a claim about the
        // ACT; the body shows one proposal in the set.
        chips={(approval, shared) => ({
          meta: (
            <>
              {shared.kind !== undefined && (
                <>
                  <AutonomyDot tier={approvalDotTier(shared.kind, tierMap)} />
                  <span className="t-small">
                    {approvalKindLabel(shared.kind, t)}
                  </span>
                  <DecisionToolChip
                    verb={KIND_TO_VERB[shared.kind]}
                    label={(verb) => t("decision.viaTool", { verb })}
                  />
                </>
              )}
              <DecisionStatusChip
                approval={approval}
                decided={false}
                now={nowMs}
                labels={statusLabels(t, locale)}
              />
            </>
          ),
          // Not `provenanceOf(undefined)`: that answers "nobody recorded a
          // source", which is a claim of its own and a false one here — the
          // members each recorded one and they differ.
          provenance:
            shared.proposedBy === undefined
              ? undefined
              : provenanceOf(shared.proposedBy, viewerId),
          confidence: confidenceLevel(shared.confidence) ?? undefined,
          display: resolveDisplay(
            approval.kind,
            (approval.proposed_change ?? {}) as Record<string, unknown>,
            t,
            stagedDay,
          ),
        })}
      />
    </section>
  );
}
