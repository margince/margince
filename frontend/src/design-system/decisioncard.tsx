// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId, useState } from "react";
import type { components } from "../api/schema";
import { Button } from "./atoms";
import { type Fact, FactList } from "./factlist";
import type { SectionState } from "./surfacestate";
import { SurfaceState } from "./surfacestate";
import {
  type ConfidenceLevel,
  ConfidenceMeter,
  EvidenceChip,
  FieldDiff,
  type Provenance,
  ProvenanceTag,
} from "./trust";
import "./decisioncard.css";

// DecisionCard — the ONE way this product asks a person to decide something an
// automation staged.
//
// It was `ApprovalRow` in `screens/inbox.tsx`, imported by seven screens, which
// is the shape the design-system README names as a defect: "a screen file is not
// a place to keep a primitive". Two things changed in the move, and both are the
// reason the move was worth making.
//
// It RENDERS THE PROPOSED CONTENT. The old row said "an automation drafted a
// reply to <them>" and stopped — the summary, the provenance and the countdown,
// which are everything about the proposal EXCEPT the proposal. A person cannot
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
// decision that reads one way in a queue and another way on the home screen is
// two decisions to the person answering it.

export type DecisionApproval = components["schemas"]["Approval"];

/**
 * How near the deadline is — the card's own reading of `expires_at`, and the
 * one spelling of these thresholds in the product. `screens/inbox.tsx` maps it
 * onto its countdown badge's tone rather than carrying a second copy of the
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

/** When a staged proposal lapses, in wall-clock ms — null for one that never does. */
export function decisionExpiryMs(approval: DecisionApproval): number | null {
  return approval.expires_at ? new Date(approval.expires_at).getTime() : null;
}

/**
 * Lapsed either because the wire already stamped it (the server expires lazily
 * at read time) or because the live clock crossed `expires_at` since the list
 * was fetched. Both mean the same thing to a reader: it is no longer
 * approvable, so Accept is not drawn at all.
 */
export function decisionLapsed(
  approval: DecisionApproval,
  now: number,
): boolean {
  const expiresAtMs = decisionExpiryMs(approval);
  return (
    approval.status === "expired" || (expiresAtMs != null && now >= expiresAtMs)
  );
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
}>;

/** The old→new sides of one field the proposal would change. */
type DecisionDiff = Readonly<{
  field: string;
  from: string | null;
  to: string | null;
}>;

/** The drafted message a send-shaped payload carries. */
type DecisionDraft = Readonly<{
  subject: string | null;
  body: string | null;
}>;

// A payload value as one line of text. `proposed_change` is an open map in the
// contract, so anything can be under any key: a string is shown as written, a
// scalar as its own digits, and a nested document as its JSON rather than as
// "[object Object]" — which is what a reader saw on the one card that hit it.
function asText(value: unknown): string | null {
  if (value === null || value === undefined) {
    return null;
  }
  if (typeof value === "string") {
    return value.trim() === "" ? null : value;
  }
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  return JSON.stringify(value);
}

// The drafted message, for the kinds whose whole question is words somebody is
// about to send in the reader's name (`held_draft`, `send_email`). Narrowed
// rather than asserted: a kind that puts something other than a string under
// `subject` reads as no subject at all, which is what the inbox already did.
function draftOf(change: Readonly<Record<string, unknown>>): DecisionDraft {
  const subject = typeof change.subject === "string" ? change.subject : null;
  const body = typeof change.body === "string" ? change.body : null;
  return { subject: asText(subject), body: asText(body) };
}

// The `current_<name>` / `proposed_<name>` pairs, which is how this product's
// stagers spell a field change: `compose/signalproposals.go` puts
// current_lifecycle beside proposed_lifecycle precisely so "the card must show
// both sides", and the company context surface reads current_value /
// proposed_value the same way. A `proposed_` key with no sibling is NOT a diff
// — it is a value the proposal adds, and drawing it against a struck-through
// blank would claim we know the old one was empty.
const PROPOSED = "proposed_";
const CURRENT = "current_";

function diffsOf(
  change: Readonly<Record<string, unknown>>,
): readonly DecisionDiff[] {
  const diffs: DecisionDiff[] = [];
  for (const [key, value] of Object.entries(change)) {
    if (!key.startsWith(PROPOSED)) {
      continue;
    }
    const field = key.slice(PROPOSED.length);
    const currentKey = `${CURRENT}${field}`;
    if (!Object.hasOwn(change, currentKey)) {
      continue;
    }
    diffs.push({ field, from: asText(change[currentKey]), to: asText(value) });
  }
  return diffs;
}

// Everything the two readings above did not consume, as label→value rows. Wire
// field identifiers, shown as written: they are a payload path rather than
// prose, and the inbox's detail modal has always rendered them raw for the same
// reason. Drawn only in the deck layout — a row already offers a way through to
// the whole payload, and a queue of rows each unrolling nine keys is not a queue
// anybody can read.
function restOf(
  change: Readonly<Record<string, unknown>>,
  draft: DecisionDraft,
  diffs: readonly DecisionDiff[],
): readonly Fact[] {
  const consumed = new Set<string>();
  if (draft.subject) {
    consumed.add("subject");
  }
  if (draft.body) {
    consumed.add("body");
  }
  for (const diff of diffs) {
    consumed.add(`${PROPOSED}${diff.field}`);
    consumed.add(`${CURRENT}${diff.field}`);
  }
  return Object.entries(change).flatMap(([key, value]) => {
    const text = consumed.has(key) ? null : asText(value);
    return text === null
      ? []
      : [
          {
            key,
            term: <span className="t-mono">{key}</span>,
            value: <span className="dcard-fact">{text}</span>,
          },
        ];
  });
}

// Past this many characters the drafted body is clamped. A card that grows with
// its content stops being a card: one long email pushes every verb below the
// fold, and in the deck it pushes the card behind it off the plate entirely.
const BODY_CLAMP_CHARS = 320;

// The clamped body and its expander.
//
// A `.link-button` rather than `Disclosure`, and the difference is the whole
// point: a disclosure HIDES its content until asked, and a draft nobody can see
// any of is a question with the answer removed. The reader gets the opening
// lines unasked, and the control only lifts the clamp — which is why it is an
// `aria-expanded` toggle over one paragraph rather than a second copy of the
// text behind a summary.
function DraftBody({
  body,
  labels,
}: Readonly<{ body: string; labels: DecisionCardLabels }>) {
  const [open, setOpen] = useState(false);
  const bodyId = useId();
  // Expandable only where the caller named both halves of the toggle: a control
  // whose label the surface has no words for is a control nobody can act on.
  const expandable =
    body.length > BODY_CLAMP_CHARS &&
    labels.showMore !== undefined &&
    labels.showLess !== undefined;
  const clampable = body.length > BODY_CLAMP_CHARS;
  return (
    <>
      <p
        id={bodyId}
        className="dcard-draft-body"
        data-clamped={clampable && !open ? "" : undefined}
      >
        {body}
      </p>
      {expandable && (
        <button
          type="button"
          className="link-button"
          aria-expanded={open}
          aria-controls={bodyId}
          onClick={() => setOpen((shown) => !shown)}
        >
          {open ? labels.showLess : labels.showMore}
        </button>
      )}
    </>
  );
}

// What the proposal actually says, in the order a reader needs it: the words
// they are being asked to put their name on, then the values that would move,
// then whatever else the payload carries.
function DecisionContent({
  draft,
  diffs,
  rest,
  labels,
}: Readonly<{
  draft: DecisionDraft;
  diffs: readonly DecisionDiff[];
  rest: readonly Fact[];
  labels: DecisionCardLabels;
}>) {
  return (
    <>
      {draft.body && (
        <div className="dcard-draft">
          <span className="t-eyebrow dcard-draft-label">
            {labels.draftBody}
          </span>
          <DraftBody body={draft.body} labels={labels} />
        </div>
      )}
      {diffs.map((diff) => (
        <div className="dcard-diff" key={diff.field}>
          <span className="t-eyebrow dcard-draft-label">
            {diff.field.replaceAll("_", " ")}
          </span>
          <FieldDiff oldValue={diff.from} newValue={diff.to} />
        </div>
      ))}
      {rest.length > 0 && <FactList facts={rest} className="dcard-rest" />}
    </>
  );
}

// The receipts, always on the card and never behind a popover: this is the one
// surface where a person has to be able to check a claim BEFORE agreeing to it.
// Collapsed in the row layout, where a queue of verbatim snippets would bury
// the verbs; open in the deck, where there is one card and room to read it.
function DecisionEvidence({
  evidence,
  collapsed,
}: Readonly<{
  evidence: DecisionApproval["evidence"];
  collapsed: boolean;
}>) {
  // Two rows of one source can open with the same twelve characters — a quoted
  // thread quotes itself — and a duplicate key hands one chip's expansion state
  // to another on the next render. So the key carries an occurrence count of the
  // otherwise-identical string: derived from the data rather than from the
  // position, which is what makes it survive a list that arrives in a different
  // order.
  const seen = new Map<string, number>();
  const keyOf = (item: { source_id?: string | null; snippet: string }) => {
    const base = `${item.source_id ?? ""}-${item.snippet.slice(0, 12)}`;
    const before = seen.get(base) ?? 0;
    seen.set(base, before + 1);
    return before === 0 ? base : `${base}#${before}`;
  };
  return (
    <>
      {evidence?.map((item) =>
        item.evidence_snippet ? (
          <EvidenceChip
            key={keyOf({
              source_id: item.source_id,
              snippet: item.evidence_snippet,
            })}
            collapsed={collapsed}
            evidence={{
              snippet: item.evidence_snippet,
              source: item.source_type ?? "",
              lines: item.source_lines,
            }}
          />
        ) : null,
      )}
    </>
  );
}

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
   * banding has one home in `screens/inbox.tsx` that eight surfaces read.
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
}>;

// What the payload says, once, for the whole render. A reading rather than four
// derivations scattered through the JSX: whether the card has anything to show
// is the same question as what it shows, and answering it twice is how a card
// ends up drawing an empty state above its own content.
type PayloadReading = Readonly<{
  draft: DecisionDraft;
  diffs: readonly DecisionDiff[];
  rest: readonly Fact[];
  hasContent: boolean;
}>;

function readPayload(
  approval: DecisionApproval,
  layout: "deck" | "row",
): PayloadReading {
  const change = approval.proposed_change ?? {};
  const draft = draftOf(change);
  const diffs = diffsOf(change);
  const rest = layout === "deck" ? restOf(change, draft, diffs) : [];
  return {
    draft,
    diffs,
    rest,
    hasContent:
      draft.subject !== null ||
      draft.body !== null ||
      diffs.length > 0 ||
      rest.length > 0,
  };
}

// The chips line, then the headline and the sentence explaining it.
//
// The headline is the drafted SUBJECT where there is one, because that is the
// line that differs: the server's summary names only the addressee, so a queue
// of drafts to the same handful of people reads as one sentence over and over.
// The summary then explains it underneath — and only underneath a subject, since
// with no subject the summary IS the headline and printing it twice reads as two
// facts.
function DecisionHead({
  approval,
  draft,
  labels,
  headingId,
  meta,
  aside,
  provenance,
  confidence,
}: Readonly<{
  approval: DecisionApproval;
  draft: DecisionDraft;
  labels: DecisionCardLabels;
  headingId: string;
  meta?: ReactNode;
  aside?: ReactNode;
  provenance?: Provenance;
  confidence?: ConfidenceLevel;
}>) {
  const headline = draft.subject ?? approval.summary ?? null;
  return (
    <>
      <div className="dcard-meta">
        {meta}
        {provenance && <ProvenanceTag provenance={provenance} />}
        {confidence && <ConfidenceMeter level={confidence} />}
        {aside && <div className="dcard-aside">{aside}</div>}
      </div>
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
      {draft.subject && approval.summary && (
        <p className="t-small approval-why">{approval.summary}</p>
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
}: DecisionCardProps) {
  const headingId = useId();
  const payload = readPayload(approval, layout);
  const lapsed = !decided && decisionLapsed(approval, now);
  const named = payload.draft.subject ?? approval.summary ?? null;

  return (
    <article
      // `.staging-card` as well as its own class, and that is reuse rather than
      // decoration: the AI-tinted, dashed "this is not real yet" ground is
      // already declared once, in trust.css, and it is the same claim this card
      // makes. Re-spelling it here would be a second copy of the one signal that
      // separates a proposal from a persisted fact.
      className={["staging-card dcard", className ?? ""]
        .filter(Boolean)
        .join(" ")}
      data-layout={layout}
      data-urgency={urgencyOf(approval, now, decided)}
      data-lapsed={lapsed ? "" : undefined}
      data-approval={approval.id}
      data-testid={testId}
      aria-labelledby={named ? headingId : undefined}
    >
      <DecisionHead
        approval={approval}
        draft={payload.draft}
        labels={labels}
        headingId={headingId}
        meta={meta}
        aside={aside}
        provenance={provenance}
        confidence={confidence}
      />
      <div className="dcard-body">
        <SurfaceState
          state={state ?? (payload.hasContent ? "ready" : "empty")}
          emptyLabel={labels.noContent}
        >
          <DecisionContent
            draft={payload.draft}
            diffs={payload.diffs}
            rest={payload.rest}
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
      {/* A lapsed proposal says so where its verbs used to be. Offering Accept
          on it would be a control whose only possible answer is a refusal. */}
      {lapsed && <p className="dcard-lapsed">{labels.expired}</p>}
      {detail}
      {/* An open editor closes with the deadline. Left drawn, a proposal that
          lapsed while somebody was editing it still offered "approve edited" —
          a write the server can only refuse, and the one control this card is
          careful never to draw (the verbs go for the same reason, below). */}
      {!lapsed && editor}
      {!decided && !lapsed && !editor && (
        <DecisionVerbs
          labels={labels}
          pending={pending}
          onAccept={onAccept}
          onEdit={onEdit}
          onReject={onReject}
          onSkip={onSkip}
        />
      )}
      {notice}
      {children}
    </article>
  );
}

// The verbs.
//
// Accept is the control that STARTS the write, so it is the one that goes busy:
// it keeps the reader's focus and says a verdict is on its way. The other three
// stay `disabled` while it is out, and the difference is the whole point of
// having two props — they did not start anything, they are simply not available
// yet. Drawing Reject busy would claim a rejection nobody sent.
function DecisionVerbs({
  labels,
  pending,
  onAccept,
  onEdit,
  onReject,
  onSkip,
}: Readonly<{
  labels: DecisionCardLabels;
  pending?: boolean;
  onAccept?: () => void;
  onEdit?: () => void;
  onReject?: () => void;
  onSkip?: () => void;
}>) {
  if (!onAccept && !onEdit && !onReject && !onSkip) {
    return null;
  }
  return (
    <div className="approval-gate dcard-verbs">
      {onAccept && (
        <Button variant="primary" small pending={pending} onClick={onAccept}>
          {labels.accept}
        </Button>
      )}
      {onEdit && (
        <Button small disabled={pending} onClick={onEdit}>
          {labels.edit}
        </Button>
      )}
      {onReject && (
        <Button small disabled={pending} onClick={onReject}>
          {labels.reject}
        </Button>
      )}
      {onSkip && labels.skip && (
        <Button small disabled={pending} onClick={onSkip}>
          {labels.skip}
        </Button>
      )}
    </div>
  );
}
