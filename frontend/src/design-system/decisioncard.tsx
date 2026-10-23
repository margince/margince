// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId, useRef } from "react";
import { DecisionContent, DecisionEvidence } from "./decisioncard.content";
import { type DecisionDisplay, decisionEvidence } from "./decisioncard.payload";
import {
  DecisionAnswers,
  type DecisionHeldRejection,
  takesFocus,
} from "./decisioncard.verbs";
import { Popover } from "./popover";
import type { SectionState } from "./surfacestate";
import { SurfaceState } from "./surfacestate";
import type { ConfidenceLevel, Provenance } from "./trust";
import "./decisioncard.css";

import {
  cardName,
  type DecisionCardLabels,
  type DecisionCompactWords,
  DecisionHead,
  readPayload,
} from "./decisionhead";
// DecisionCard — the ONE way this product asks a colleague to decide something an
// automation staged.
//
// It was `ApprovalRow` in a screen file, imported by seven screens, which
// is the shape the design-system README names as a defect: "a screen file is not
// a place to keep a primitive". Two things changed in the move, and both are the
// reason the move was worth making.
//
// It RENDERS THE PROPOSED CONTENT. The old row said "an automation drafted a
// reply to <them>" and stopped — the summary, the provenance and the countdown,
// which are everything about the proposal EXCEPT the proposal. A contact cannot
// answer a question they have not been shown, so the drafted subject and body,
// and the old→new sides of a field change, are on the card itself now.
//
// And EXPIRY IS PRESSURE. A deadline rendered as grey caption text is a fact the
// reader has to go looking for; here it tints the card's own edge and, once it
// has passed, takes the Accept control away rather than letting somebody press a
// button that can only be refused.
//
// It holds no mutation, no query and no copy: the verbs arrive as callbacks, the
// words arrive as `labels`, and the two layouts are one component because a
// decision that reads one way in a queue and another way on the Brief is
// two decisions to the contact answering it.
import {
  type DecisionApproval,
  type DecisionUrgency,
  decisionExpiryMs,
  decisionLapsed,
  decisionUrgency,
} from "./decisionstatus";

export type { DecisionHeldRejection } from "./decisioncard.verbs";
export type { DecisionCardLabels, DecisionCompactWords } from "./decisionhead";
export {
  type DecisionApproval,
  type DecisionDeadline,
  DecisionStatusChip,
  type DecisionStatusLabels,
  DecisionToolChip,
  type DecisionUrgency,
  type DecisionVerdict,
  decisionExpiryMs,
  decisionLapsed,
  decisionUrgency,
  decisionUrgencyTone,
} from "./decisionstatus";

export type DecisionCardProps = Readonly<{
  approval: DecisionApproval;
  /**
   * `deck` is the tall form — one card at a time, the whole payload on it.
   * `row` is the compact list form the inbox and the six record surfaces draw.
   * ONE component, because it is one decision either way.
   */
  layout?: "deck" | "row";
  /**
   * The instant the deadline is judged against, in wall-clock ms. A prop rather
   * than a `Date.now()` inside, so a story pins it and a test does not depend
   * on what time the suite happens to run at.
   */
  now: number;
  labels: DecisionCardLabels;
  /**
   * Who staged it, and how sure they were. Both computed by the caller: the
   * provenance reading needs the signed-in viewer's id, and the confidence
   * banding has one home in `design-system/trust.tsx` that eight surfaces read.
   */
  provenance?: Provenance;
  confidence?: ConfidenceLevel;
  /** History rather than a question: no verbs, no urgency, no lapse notice. */
  decided?: boolean;
  /** Whether a verdict this card sent is still in flight. */
  pending?: boolean;
  /**
   * What this card KNOWS about its own content. Defaults to `ready` when the
   * payload carries something to read and `empty` when it carries nothing at
   * all — a caller whose read failed passes the honest state instead.
   */
  state?: SectionState;
  onAccept?: () => void;
  onEdit?: () => void;
  onReject?: () => void;
  onSkip?: () => void;
  /**
   * Drawn in place of the verbs while set. The caller owns the window and the
   * send; the card moves focus to its words and, once it leaves, back to Reject.
   */
  heldRejection?: DecisionHeldRejection;
  /**
   * The chips the CALLER owns on the meta line — the autonomy dot, the kind, the
   * originating tool, the countdown or status badge. They are the caller's
   * because each of them needs something this tier does not hold: the agent
   * tier map, the kind catalog, the reader's own locale and zone.
   */
  meta?: ReactNode;
  /** The trailing affordance on that line: the inbox's way through to the detail. */
  aside?: ReactNode;
  /**
   * Content the caller adds UNDER the proposal and above the verbs: a bundle's
   * per-recipient expander, a record link, whatever this particular decision
   * needs beside the payload. Above the verbs on purpose — everything a reader
   * has to weigh belongs before the controls that answer it.
   */
  detail?: ReactNode;
  /** An editor the caller owns, drawn INSTEAD of the verbs while it is open. */
  editor?: ReactNode;
  /** What the last verdict did — a refusal, a re-stage prompt. Under the verbs. */
  notice?: ReactNode;
  /** Anything that must live inside this element: a dialog the caller owns. */
  children?: ReactNode;
  className?: string;
  testId?: string;
  /**
   * The `row` layout's DENSE form: one line per decision — what is asked, when
   * it runs out, who staged it, and the verdicts — with the proposal itself
   * behind a disclosure. Absent draws the full row.
   *
   * Ignored in `deck`, where the whole point is the whole payload on one plate:
   * a tall card that hid its proposal would be a question with the answer taken
   * out.
   */
  compact?: DecisionCompactWords;
  /**
   * What THIS kind of proposal shows, resolved into the reader's language by
   * the caller. Empty leaves the generic reading — wire keys as written — which
   * is what a kind carrying untyped tool arguments honestly has.
   */
  display?: readonly DecisionDisplay[];
}>;

/**
 * The card's urgency band, or undefined for a proposal with no deadline and for
 * one that is already history. Absence is not a state: an undefined band leaves
 * the card its ordinary edge rather than claiming calm.
 */
function urgencyOf(
  approval: DecisionApproval,
  now: number,
  decided: boolean,
): DecisionUrgency | undefined {
  const expiresAtMs = decisionExpiryMs(approval);
  if (decided || expiresAtMs == null) {
    return undefined;
  }
  return decisionUrgency(expiresAtMs - now);
}

// A decided card draws no verbs, so the card itself takes focus back from
// a held rejection's bar.
function heldFocusTarget(decided: boolean): -1 | undefined {
  return decided ? -1 : undefined;
}

export function DecisionCard({
  approval,
  layout = "deck",
  now,
  labels,
  provenance,
  confidence,
  decided = false,
  pending,
  state,
  onAccept,
  onEdit,
  onReject,
  onSkip,
  heldRejection,
  meta,
  aside,
  detail,
  editor,
  notice,
  children,
  className,
  testId,
  compact,
  display = [],
}: DecisionCardProps) {
  const headingId = useId();
  const focusNextRef = useRef<"label" | "reject" | null>(null);
  const payload = readPayload(approval, layout, display);
  const lapsed = !decided && decisionLapsed(approval, now);
  // The SAME question the headline asks, so the article's accessible name and
  // the line a sighted reader sees cannot come apart: a card named only by its
  // target_label rendered a headline and pointed aria-labelledby at nothing.
  const named = cardName(approval, payload.draft) ?? approval.summary ?? null;
  // The dense form belongs to the row. A deck card that folded its proposal
  // away would be the one thing a tall plate exists not to do.
  const dense = layout === "row" ? compact : undefined;
  const proposal = (
    <>
      <div className="dcard-body">
        <SurfaceState
          state={state ?? (payload.hasContent ? "ready" : "empty")}
          emptyLabel={labels.noContent}
          loadingLabel={labels.loading}
        >
          <DecisionContent
            draft={payload.draft}
            diffs={payload.diffs}
            lead={payload.lead}
            rest={payload.rest}
            labels={labels}
          />
        </SurfaceState>
      </div>
      <div className="dcard-evidence">
        <DecisionEvidence
          evidence={decisionEvidence(approval)}
          collapsed={
            layout === "row" && approval.preview?.evidence === undefined
          }
        />
      </div>
      {detail}
      {/* An open editor closes with the deadline. Left drawn, a proposal that
          lapsed while somebody was editing it still offered "approve edited" —
          a write the server can only refuse, and the one control this card is
          careful never to draw (the verbs go for the same reason, below). */}
      {!lapsed && editor}
    </>
  );
  // A lapsed proposal says so where its verbs used to be. Offering Accept on it
  // would be a control whose only possible answer is a refusal.
  const ranOut = lapsed && (
    <p
      className="dcard-lapsed"
      tabIndex={-1}
      ref={takesFocus(focusNextRef, "reject")}
    >
      {labels.expired}
    </p>
  );
  const verbs = (
    <DecisionAnswers
      decided={decided}
      focusNextRef={focusNextRef}
      held={heldRejection}
      answering={!lapsed && !editor}
      labels={labels}
      compact={dense}
      pending={pending}
      onAccept={onAccept}
      onEdit={onEdit}
      onReject={onReject}
      onSkip={onSkip}
    />
  );
  const head = (
    <DecisionHead
      approval={approval}
      draft={payload.draft}
      labels={labels}
      headingId={headingId}
      meta={meta}
      aside={aside}
      provenance={provenance}
      confidence={confidence}
      compact={dense !== undefined}
    />
  );

  return (
    <article
      // `.staging-card` as well as its own class, and that is reuse rather than
      // decoration: the AI-tinted, dashed "this is not real yet" ground is
      // declared once with the rest of the panel-ai family (panel.css) and is
      // the same claim this card makes. Re-spelling it here would be a second
      // copy of the one signal separating a proposal from a persisted fact.
      className={["staging-card dcard", className ?? ""]
        .filter(Boolean)
        .join(" ")}
      data-layout={layout}
      data-urgency={urgencyOf(approval, now, decided)}
      data-lapsed={lapsed ? "" : undefined}
      data-approval={approval.id}
      data-testid={testId}
      data-density={dense ? "compact" : undefined}
      aria-labelledby={named ? headingId : undefined}
      tabIndex={heldFocusTarget(decided)}
      ref={takesFocus(focusNextRef, "reject")}
    >
      {dense ? (
        <>
          {/* ONE LINE, and the verbs end it. The question, its chips and the
              answers keep one x down a queue, which is what makes a list of
              these scannable — everything a reader must WEIGH is behind the
              line's own control, off to the side of it rather than under it. */}
          <div className="dcard-line">
            {head}
            {ranOut}
            {/* A POPOVER rather than a disclosure, which is the catalog's own
                direction for this case: opening a fold in a list pushes every
                row under it down, so the reader who wanted to compare two
                proposals moved the second one out from under their eye — and
                the verbs with it. Escape closes it and hands the trigger back
                its focus. */}
            <Popover label={dense.detail} className="dcard-detail">
              {/* The sentence that says WHY, which the line gave up to stay
                  one line. It leads here for the same reason it led the card. */}
              {named && approval.summary && (
                <p className="approval-why">{approval.summary}</p>
              )}
              {proposal}
            </Popover>
            {verbs}
          </div>
        </>
      ) : (
        <>
          {head}
          {proposal}
          {ranOut}
          {verbs}
        </>
      )}
      {notice}
      {children}
    </article>
  );
}

// The payload vocabulary is re-exported rather than moved out of reach. A
// caller resolving a kind's display policy is talking to the card. A second
// import path for the type it hands over would say the two were separate tiers.
export type { DecisionDisplay } from "./decisioncard.payload";
