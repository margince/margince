// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
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
} from "../design-system/decisiondeck";
import { Panel } from "../design-system/panel";
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
import { commitTray } from "./brief.decisions.commit";
import { problemMessageOf, provenanceOf, useViewerId } from "./common";
import { worklistLaneHref } from "./worklist.header";

// The decisions half of Brief: the deck, its tray, and the one act that sends
// what is in it.
//
// Staging is local and the commit is separate, which is the whole design rather
// than a flourish: `approvals/service.go` states that a committed decision is
// deliberately un-undoable, and rejecting is not an undo either, so the tray IS
// the undo a swipe would otherwise not have.

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
    skip: t("brief.deck.later"),
    expired: t("decision.expired"),
    draftSubject: t("decision.draftSubject"),
    draftBody: t("decision.draftBody"),
    showMore: t("brief.deck.showMore"),
    showLess: t("brief.deck.showLess"),
    noContent: t("common.empty"),
    loading: t("brief.panel.decisions"),
  };
  return {
    card,
    deckLabel: t("brief.panel.decisions"),
    viewLabel: t("brief.deck.view"),
    viewDeck: t("brief.deck.viewDeck"),
    viewList: t("brief.deck.viewList"),
    keys: t("brief.deck.keys"),
    behind: (count) =>
      plural("brief.deck.behind", count, {
        count: formatNumber(count, locale),
      }),
    staged: (count) =>
      plural("brief.deck.staged", count, {
        count: formatNumber(count, locale),
      }),
    commit: t("brief.deck.commit"),
    unstage: t("brief.deck.unstage"),
    clearedTitle: t("brief.deck.clearedTitle"),
    cleared: (count) =>
      plural("brief.deck.cleared", count, {
        count: formatNumber(count, locale),
      }),
    clearedTime: (atMs) =>
      t("brief.deck.clearedTime", {
        at: formatDateTime(new Date(atMs).toISOString(), locale, viewerZone()),
      }),
    empty: t("brief.deck.empty"),
    bundleSummary: (members) =>
      t("brief.deck.bundleSummary", { count: formatNumber(members, locale) }),
    bundleMembers: (members) =>
      t("brief.deck.bundleMembers", { count: formatNumber(members, locale) }),
    // The words are the switch: Brief's list is a LINE per decision, because
    // this page opens with the decisions and goes on to the day's own work — a
    // reader is passing through, not working a queue to its end.
    compactRow: {
      detail: t("brief.deck.rowDetail"),
      more: t("brief.deck.rowMore"),
    },
  };
}

// How many decisions the line-per-row list draws before it hands over to the
// approvals lane. Three is what fits above the day's own work without becoming
// the page: this is the block that says what is blocked ON somebody, and the
// morning underneath it is what they can get on with.
const LISTED = 3;

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
    // No `aria-label` here. The panel inside is a titled region and names
    // itself; a wrapper repeating that name puts one zone in a screen reader's
    // list twice. The id is the page's — the head's counts link to it.
    <section id="brief-decisions">
      {/* A PANEL, with the same header band Today wears: one band, one height,
          one interval down the column, so the page reads as zones rather than
          as a heading here and a card there. The Deck/List toggle is the band's
          `titleAction`, which is where a zone keeps the control that changes
          how it is drawn. */}
      <DecisionDeck
        items={items}
        now={nowMs}
        frame={({ toggle, content }) => (
          <Panel title={t("brief.panel.decisions")} titleAction={toggle}>
            {content}
          </Panel>
        )}
        listCap={LISTED}
        listRest={(hidden) => (
          <p className="ddeck-list-rest">
            <a className="entity-link" href={worklistLaneHref("decisions")}>
              {plural("brief.deck.rest", hidden, {
                count: formatNumber(hidden, locale),
              })}
            </a>
          </p>
        )}
        labels={deckLabels(t, plural, locale)}
        state={state}
        loadingLabel={t("brief.panel.decisions")}
        commitState={commitState}
        notice={
          notice ? <p className="brief-error t-caption">{notice}</p> : undefined
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
                  <span className="t-caption">
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
