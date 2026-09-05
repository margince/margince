// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId, useState } from "react";
import type { components } from "../api/schema";
import { Badge, Button } from "./atoms";
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
// It was `ApprovalRow` in a screen file, imported by seven screens, which
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
 * Every decision surface draws this one — the inbox's Decisions row and Home's
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

/**
 * What one payload field is called and how to read it, already resolved into
 * the reader's own language by the caller.
 *
 * Resolved rather than looked up here, for the reason DecisionToolChip takes a
 * verb rather than a kind: which fields a kind shows is the product's
 * vocabulary, and a primitive holding a copy of it would be a second author of
 * it. This tier knows how to DRAW a labelled fact; it does not know that a
 * close-date correction has a `basis`.
 */
export type DecisionDisplay = Readonly<{
  /** The payload key this describes. */
  field: string;
  /** What to call it, in the reader's language. */
  label: string;
  /** The value, already formatted — a date on the reader's calendar, an enum
   *  in words. Null where the payload does not carry the field. */
  value: string | null;
  /** Leads the body as a sentence rather than sitting in the fact list. */
  lead?: boolean;
}>;

// Everything the readings above did not consume, as label→value rows.
//
// A kind that declares a display policy shows exactly what it declared: the
// caller resolved those fields, so the payload's remaining keys are identifiers
// and bookkeeping that answer nothing a person was asked. Printing them was how
// a business question came to read as a database row — `deal_id`,
// `target_version`, `flags: ["unrealistic_stale"]` under a headline about a
// deal going quiet.
//
// A kind that declares nothing keeps the old reading: wire keys as written.
// That is not a nicer fallback, it is an honest one — the raw-args kinds carry
// an agent's tool arguments or an automation's action, with no typed payload to
// describe, and inventing captions for a bag of unknown keys would be guessing
// at what the software meant. Drawn only in the deck layout, where the row
// offers a way through to the whole payload instead.
function restOf(
  change: Readonly<Record<string, unknown>>,
  draft: DecisionDraft,
  diffs: readonly DecisionDiff[],
  display: readonly DecisionDisplay[],
): readonly Fact[] {
  if (display.length > 0) {
    return display.flatMap((entry) =>
      entry.lead || entry.value === null
        ? []
        : [
            {
              key: entry.field,
              term: <span>{entry.label}</span>,
              value: <span className="dcard-fact">{entry.value}</span>,
            },
          ],
    );
  }
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

// What the proposal actually says, in the order a reader needs it: why they
// are being asked, then the words they are being asked to put their name on,
// then the values that would move, then the rest.
//
// The reason comes FIRST and unlabelled. It is a sentence the server wrote for
// a person — the close-date sweep calls its own field "the plain-language
// derivation" — so captioning it would frame an explanation as a data point,
// and burying it under the values it explains asks the reader to work out the
// question from the answer.
function DecisionContent({
  draft,
  diffs,
  lead,
  rest,
  raw,
  labels,
}: Readonly<{
  draft: DecisionDraft;
  diffs: readonly DecisionDiff[];
  lead: string | null;
  rest: readonly Fact[];
  /** The facts are wire keys, not declared fields — see restOf. */
  raw: boolean;
  labels: DecisionCardLabels;
}>) {
  return (
    <>
      {lead && <p className="dcard-lead">{lead}</p>}
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
      {rest.length > 0 && (
        <FactList
          facts={rest}
          className={raw ? "dcard-rest dcard-rest-raw" : "dcard-rest"}
        />
      )}
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
    // The FULL snippet, not a prefix: two quotes from one source that open the
    // same way are different evidence, and a key that could not tell them apart
    // handed one chip the other's expansion state whenever the list reordered.
    const base = JSON.stringify([item.source_id ?? "", item.snippet]);
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
  rest: readonly Fact[];
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
      ? restOf(change, draft, diffs, display)
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
// of drafts to the same handful of people reads as one sentence over and over.
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
  const named = cardName(approval, draft);
  const headline = named ?? approval.summary ?? null;
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
  display = [],
}: DecisionCardProps) {
  const headingId = useId();
  const payload = readPayload(approval, layout, display);
  const lapsed = !decided && decisionLapsed(approval, now);
  // The SAME question the headline asks, so the article's accessible name and
  // the line a sighted reader sees cannot come apart: a card named only by its
  // target_label rendered a headline and pointed aria-labelledby at nothing.
  const named = cardName(approval, payload.draft) ?? approval.summary ?? null;

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
