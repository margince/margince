// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Pencil, RotateCcwClock, Trash2 } from "lucide-react";
import { type ReactNode, useId } from "react";
import type { components } from "../api/schema";
import { ActionRow } from "./actionrow";
import { Badge, Button, OverflowMenu } from "./atoms";
import { DecisionContent, DecisionEvidence } from "./decisioncard.content";
import {
  type DecisionDiff,
  type DecisionDisplay,
  type DecisionDraft,
  diffsOf,
  draftOf,
  type PayloadField,
  restFields,
} from "./decisioncard.payload";
import { IconAction } from "./iconaction";
import { Popover } from "./popover";
import type { SectionState } from "./surfacestate";
import { SurfaceState } from "./surfacestate";
import {
  type ConfidenceLevel,
  ConfidenceMeter,
  type Provenance,
  ProvenanceTag,
} from "./trust";
import "./decisioncard.css";

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

export type DecisionApproval = components["schemas"]["Approval"];

// The payload vocabulary is re-exported rather than moved out of reach: a
// caller resolving a kind's display policy is talking to the CARD, and a second
// import path for the type it hands over would say the two were separate tiers.
export type { DecisionDisplay } from "./decisioncard.payload";

/**
 * How near the deadline is — the card's own reading of `expires_at`, and the
 * one spelling of these thresholds in the product. A countdown badge takes its
 * tone from this function rather than carrying a second copy of the
 * hours: a deadline that reads urgent on the card and calm in the chip beside
 * it is worse than either reading on its own.
 */
export type DecisionUrgency = "calm" | "soon" | "urgent" | "lapsed";

const HOUR_MS = 60 * 60 * 1000;

/** Urgency from the milliseconds left. At or past zero the proposal has lapsed. */
export function decisionUrgency(msRemaining: number): DecisionUrgency {
  if (msRemaining <= 0) {
    return "lapsed";
  }
  if (msRemaining < HOUR_MS) {
    return "urgent";
  }
  return msRemaining < 6 * HOUR_MS ? "soon" : "calm";
}

/**
 * The deadline half of a staged proposal — all the readings below actually
 * touch, and named as its own subject for the sake of ONE field: `status` is
 * `string` here, not the generated wire union.
 *
 * The status vocabulary lives on the server and grows there, so a build is
 * always one deploy from meeting a status it has no word for. This tree already
 * settled that argument for `kind` (`screens/approvalkind.ts`), and the
 * answer was the same: render what arrived rather than drop it. Held to the
 * closed union, the fallback in `verdictOf` would be unreachable to the compiler
 * and dead by the letter of the rule, while still being the branch that runs the
 * day the server adds a sixth status — and a test could not reach it at all.
 *
 * A full `Approval` satisfies this shape, so every existing caller passes one.
 */
export type DecisionDeadline = Readonly<{
  status: string;
  expires_at?: string | null;
}>;

/** When a staged proposal lapses, in wall-clock ms — null for one that never does. */
export function decisionExpiryMs(approval: DecisionDeadline): number | null {
  return approval.expires_at ? new Date(approval.expires_at).getTime() : null;
}

/**
 * Lapsed either because the wire already stamped it (the server expires lazily
 * at read time) or because the live clock crossed `expires_at` since the list
 * was fetched. Both mean the same thing to a reader: it is no longer
 * approvable, so Accept is not drawn at all.
 */
export function decisionLapsed(
  approval: DecisionDeadline,
  now: number,
): boolean {
  const expiresAtMs = decisionExpiryMs(approval);
  return (
    approval.status === "expired" || (expiresAtMs != null && now >= expiresAtMs)
  );
}

/**
 * The countdown's tone, from the milliseconds left: warn under six hours,
 * danger under one, and nothing beyond — never inert grey text. The HOURS are
 * not spelled here either; `decisionUrgency` owns them, and the card the chip
 * sits on tints its own edge from the same reading. Two copies of the
 * thresholds is how one deadline comes to read urgent on the card and calm in
 * the badge beside it, which is worse than either reading alone.
 */
export function decisionUrgencyTone(
  msRemaining: number,
): "danger" | "warn" | undefined {
  return URGENCY_TONE[decisionUrgency(msRemaining)];
}

const URGENCY_TONE: Readonly<
  Record<DecisionUrgency, "danger" | "warn" | undefined>
> = {
  lapsed: "danger",
  urgent: "danger",
  soon: "warn",
  calm: undefined,
};

/** What a decided proposal ended up as. */
export type DecisionVerdict = "approved" | "rejected" | "expired";

const VERDICT_TONE: Readonly<
  Record<DecisionVerdict, "success" | "danger" | "warn">
> = {
  approved: "success",
  rejected: "danger",
  expired: "warn",
};

// A wire status this tier has not learned yet reads as `expired`. Reachable —
// see `DecisionDeadline` — because the status vocabulary grows on the server,
// and a decided card with no badge says less about itself than a slightly
// imprecise one: whatever the contract adds to the decided set, it is still
// something nobody can act on any more.
function verdictOf(status: string): DecisionVerdict {
  if (status === "approved") {
    return "approved";
  }
  return status === "rejected" ? "rejected" : "expired";
}

/**
 * The status chip's words, on the same terms as `DecisionCardLabels`: this tier
 * knows when a deadline has passed, not what the reader's locale calls an hour.
 */
export type DecisionStatusLabels = Readonly<{
  /**
   * How long is left. It takes the SPAN rather than an already-rendered
   * countdown because the units are copy too — "3h 0m" comes out of the message
   * catalogue like every other string — and because the chip is the thing that
   * knows whether a countdown is being drawn at all, so a caller formatting one
   * up front would be formatting spans nobody sees.
   */
  expiresIn: (msRemaining: number) => string;
  /** The verdict word, one per state a decided proposal can be in. */
  approved: string;
  rejected: string;
  expired: string;
}>;

/**
 * The decision's header chip: a verdict badge once it has been answered, else
 * the live countdown to its deadline.
 *
 * Every decision surface draws this one — the inbox's Decisions row and Brief's
 * deck both — because a second countdown is a second answer to one deadline,
 * and the two drift the first time either moves.
 */
export function DecisionStatusChip({
  approval,
  decided,
  now,
  labels,
}: Readonly<{
  /** The deadline and the status; the chip reads nothing else off a proposal. */
  approval: DecisionDeadline;
  /** History rather than a question: the verdict, not the time it had left. */
  decided: boolean;
  now: number;
  labels: DecisionStatusLabels;
}>) {
  if (decided) {
    const verdict = verdictOf(approval.status);
    return <Badge tone={VERDICT_TONE[verdict]}>{labels[verdict]}</Badge>;
  }
  const expiresAtMs = decisionExpiryMs(approval);
  if (expiresAtMs == null) {
    return null;
  }
  // A lapsed pending decision carries NO chip: the card it sits on already says
  // it ran out of time, where its verbs would have been, and a badge repeating
  // that word one line above reads as two separate facts about one deadline.
  // Returning null here also keeps the countdown below off a negative span.
  if (decisionLapsed(approval, now)) {
    return null;
  }
  const remaining = expiresAtMs - now;
  return (
    <Badge tone={decisionUrgencyTone(remaining)}>
      {labels.expiresIn(remaining)}
    </Badge>
  );
}

/**
 * Names the tool that actually staged the proposal. The kind is meta on the
 * line above; this caption is what lets a reader tell "send_email" (the tool)
 * from "advance_deal" (the kind) without opening the detail.
 *
 * The verb arrives already looked up, and the words to wrap it in arrive with
 * it: the kind→verb catalogue is the product's vocabulary, not this tier's, and
 * a primitive that held a copy of it would be a second author of it. An unmapped
 * kind gives no verb and the chip stays silent rather than naming a tool nobody
 * can check.
 */
export function DecisionToolChip({
  verb,
  label,
}: Readonly<{
  verb: string | undefined;
  label: (verb: string) => string;
}>) {
  if (!verb) {
    return null;
  }
  return <span className="t-caption">{label(verb)}</span>;
}

/**
 * The words. Every one of them is the caller's, translated with `t()` at the
 * call site — a primitive that carried its own copy would be the second author
 * of the product's vocabulary, and this one has no way to know whether "Accept"
 * releases an email or promotes a lead.
 */
export type DecisionCardLabels = Readonly<{
  /** The three verdicts every decision surface offers. A verb is drawn only
   *  where its callback is given as well. */
  accept: string;
  edit: string;
  reject: string;
  /**
   * "Later". Optional because it is not a verdict every surface HAS: the inbox
   * is a queue somebody works to the end, and "later" there only moves work
   * sideways. Omitting it withholds the control even where `onSkip` is passed —
   * an unnamed button is a button nobody can act on.
   */
  skip?: string;
  /** What a lapsed card says in place of the verbs it can no longer offer. */
  expired: string;
  /** The drafted message's two halves, where a send-shaped payload carries them. */
  draftSubject: string;
  draftBody: string;
  /**
   * Opens and closes a clamped body — both, because the control's name changes
   * with its state. Optional as a PAIR, and only for a surface that offers
   * another way to the whole text: the inbox row's `aside` opens the full
   * payload one line above, and a second affordance for the same words is two
   * answers to one question. Omit them and the body stays clamped.
   */
  showMore?: string;
  showLess?: string;
  /** What there is none OF: a proposal whose payload carries nothing to read. */
  noContent: string;
  // What the card's body is waiting for, spoken while it waits. Beside
  // noContent because they answer the same question in the two states a body
  // can be absent in, and the card knows neither — only the screen does.
  loading: string;
}>;

/**
 * The two words a COMPACT row needs, and the switch that turns it on.
 *
 * Presence is the density: a surface that has the words has asked for the dense
 * form, and one that has not keeps the full row. Held as a pair rather than as a
 * boolean beside two optional strings, because a compact row with no name for
 * its fold and no name for its menu is a row whose detail and whose two quieter
 * verdicts are unreachable — an optional label on a control that MUST exist is a
 * refusal waiting to happen, which is why `Callout` carries its dismiss label
 * with its handler rather than beside it.
 */
export type DecisionCompactWords = Readonly<{
  /** What opening the row shows: the subject, the message, the deadline. */
  detail: string;
  /** Names the menu holding the verdicts the line has no room to spell. */
  more: string;
}>;

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

// What the payload says, once, for the whole render. A reading rather than four
// derivations scattered through the JSX: whether the card has anything to show
// is the same question as what it shows, and answering it twice is how a card
// ends up drawing an empty state above its own content.
type PayloadReading = Readonly<{
  draft: DecisionDraft;
  diffs: readonly DecisionDiff[];
  /** The declared sentence that says WHY this is being asked, if the kind has one. */
  lead: string | null;
  rest: readonly PayloadField[];
  /** True when `rest` holds wire keys because the kind declared no display policy. */
  raw: boolean;
  hasContent: boolean;
}>;

function readPayload(
  approval: DecisionApproval,
  layout: "deck" | "row",
  display: readonly DecisionDisplay[],
): PayloadReading {
  const change = approval.proposed_change ?? {};
  const draft = draftOf(change);
  const diffs = diffsOf(change);
  // The lead survives both layouts: the one sentence explaining why a decision
  // is in front of somebody is the thing a row is MOST missing.
  const lead = display.find((entry) => entry.lead)?.value ?? null;
  // Declared fields survive a row too, and only declared ones. The deck-only
  // rule was written against the RAW reading, where a row unrolling nine wire
  // keys made a queue unreadable — a real defect, and the reason it stays for
  // an undeclared kind. A declared list is at most four short labelled rows,
  // and suppressing them cost the reader the thing they were being asked
  // about: the Worklist shows one decision at a time, and its close-date card
  // rendered the reason with no proposed date under it.
  const rest =
    layout === "deck" || display.length > 0
      ? restFields(change, draft, diffs, display)
      : [];
  return {
    draft,
    diffs,
    lead,
    rest,
    raw: display.length === 0,
    hasContent:
      draft.subject !== null ||
      draft.body !== null ||
      lead !== null ||
      diffs.length > 0 ||
      rest.length > 0,
  };
}

// cardName is what this card is ABOUT, in the order a reader wants it: the
// drafted subject where there is one, then the record's own name.
//
// Spelled once because two places ask it — the headline, and the article's
// accessible name — and a card whose heading and whose aria-labelledby
// disagreed would be a card screen readers announce as something else.
function cardName(
  approval: DecisionApproval,
  draft: DecisionDraft,
): string | null {
  return draft.subject ?? approval.target_label ?? null;
}

// The chips line, then the headline and the sentence explaining it.
//
// The headline is the drafted SUBJECT where there is one, because that is the
// line that differs: the server's summary names only the addressee, so a queue
// of drafts to the same handful of contacts reads as one sentence over and over.
//
// Failing that it is the RECORD'S NAME, where the server recorded one. Half the
// stageable kinds carry no typed payload, and the summaries their paths compose
// name the target by uuid — "automation wants to assign_owner on deal
// 01a03781-9083-…". A reader asked to approve that decides on the verb alone.
// The name answers "which record" in the one line they are certain to read.
//
// The summary then explains whichever of those two the headline was — and only
// underneath one, since with neither the summary IS the headline and printing it
// twice reads as two facts.
function DecisionHead({
  approval,
  draft,
  labels,
  headingId,
  meta,
  aside,
  provenance,
  confidence,
  compact,
}: Readonly<{
  approval: DecisionApproval;
  draft: DecisionDraft;
  labels: DecisionCardLabels;
  headingId: string;
  meta?: ReactNode;
  aside?: ReactNode;
  provenance?: Provenance;
  confidence?: ConfidenceLevel;
  /** One line: the question first, its qualifiers after it. */
  compact?: boolean;
}>) {
  const named = cardName(approval, draft);
  const headline = named ?? approval.summary ?? null;
  const chips = (
    <div className="dcard-meta">
      {meta}
      {provenance && <ProvenanceTag provenance={provenance} />}
      {confidence && <ConfidenceMeter level={confidence} />}
      {aside && <div className="dcard-aside">{aside}</div>}
    </div>
  );
  // A COMPACT row is scanned by WHAT IS ASKED, so the question leads and the
  // chips qualify it. The tall card keeps the chips first, where the kind and
  // the countdown are the frame the rest of the card is read inside — and where
  // there is a whole plate to read rather than a line to scan.
  if (compact) {
    return (
      <>
        {headline && (
          <p className="t-body approval-headline" id={headingId}>
            {headline}
          </p>
        )}
        {chips}
      </>
    );
  }
  return (
    <>
      {chips}
      {/* What KIND of line the headline is, where it is a drafted subject. It
          sits above the headline rather than beside a repeat of it further down:
          the subject printed twice on one card reads as two facts, and the
          reader then has to work out that they are the same string. */}
      {draft.subject && (
        <span className="t-eyebrow dcard-draft-label dcard-subject-label">
          {labels.draftSubject}
        </span>
      )}
      {headline && (
        <p className="t-h2 approval-headline" id={headingId}>
          {headline}
        </p>
      )}
      {named && approval.summary && (
        <p className="t-caption approval-why">{approval.summary}</p>
      )}
    </>
  );
}

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
            raw={payload.raw}
            labels={labels}
          />
        </SurfaceState>
      </div>
      <div className="dcard-evidence">
        <DecisionEvidence
          evidence={approval.evidence}
          collapsed={layout === "row"}
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
  const ranOut = lapsed && <p className="dcard-lapsed">{labels.expired}</p>;
  const verbs = !decided && !lapsed && !editor && (
    <DecisionVerbs
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
                <p className="t-caption approval-why">{approval.summary}</p>
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

// The verbs.
//
// ONE of the four is a call to action, and the row says so: Accept keeps its
// word and its fill on the trailing edge, the three that decline sit on the
// leading one as glyphs. Four labelled buttons in a flow made the reader read
// all four every time a card arrived, and a queue is answered card after card —
// the whole cost of that row is paid once per decision. The three are exactly
// the case `IconAction` is for: a trash can, a clock turned back and a pencil
// are verbs a reader already knows, and each still carries its translated name
// to a screen reader and to a pointer through the one `label`.
//
// Accept is also the control that STARTS the write, so it is the one that goes
// busy: it keeps the reader's focus and says a verdict is on its way. The other
// three stay `disabled` while it is out, and the difference is the whole point
// of having two props — they did not start anything, they are simply not
// available yet. Drawing Reject busy would claim a rejection nobody sent.
function DecisionVerbs({
  labels,
  compact,
  pending,
  onAccept,
  onEdit,
  onReject,
  onSkip,
}: Readonly<{
  labels: DecisionCardLabels;
  /** The dense row's two extra words; absent draws the three glyphs. */
  compact?: DecisionCompactWords;
  pending?: boolean;
  onAccept?: () => void;
  onEdit?: () => void;
  onReject?: () => void;
  onSkip?: () => void;
}>) {
  if (!onAccept && !onEdit && !onReject && !onSkip) {
    return null;
  }
  const primary = onAccept ? (
    <Button variant="primary" small pending={pending} onClick={onAccept}>
      {labels.accept}
    </Button>
  ) : undefined;
  // THE DENSE ROW SPELLS TWO VERBS AND FOLDS THE REST. Accept is what the row
  // is for and Later is how a reader passes on it, so both keep their words; a
  // rejection and an edit are rarer AND heavier, and a menu gives each a whole
  // line to say so — which is exactly the trade `IconAction` refuses to make
  // for a verb whose consequence must be read before it is pressed.
  if (compact) {
    return (
      <ActionRow className="dcard-verbs" primary={primary}>
        {onSkip && labels.skip && (
          <Button small disabled={pending} onClick={onSkip}>
            {labels.skip}
          </Button>
        )}
        {/* Never drawn EMPTY: a caret over no verbs is a control that opens
            nothing. */}
        {(onReject || onEdit) && (
          <OverflowMenu label={compact.more}>
            {onReject && (
              <Button small disabled={pending} onClick={onReject}>
                {labels.reject}
              </Button>
            )}
            {onEdit && (
              <Button small disabled={pending} onClick={onEdit}>
                {labels.edit}
              </Button>
            )}
          </OverflowMenu>
        )}
      </ActionRow>
    );
  }
  return (
    <ActionRow className="dcard-verbs" primary={primary}>
      {onReject && (
        <IconAction
          small
          label={labels.reject}
          icon={<Trash2 aria-hidden />}
          disabled={pending}
          onClick={onReject}
        />
      )}
      {onSkip && labels.skip && (
        <IconAction
          small
          label={labels.skip}
          icon={<RotateCcwClock aria-hidden />}
          disabled={pending}
          onClick={onSkip}
        />
      )}
      {onEdit && (
        <IconAction
          small
          label={labels.edit}
          icon={<Pencil aria-hidden />}
          disabled={pending}
          onClick={onEdit}
        />
      )}
    </ActionRow>
  );
}
