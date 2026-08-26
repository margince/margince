// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { AlarmClock, ArrowRight, Check, X } from "lucide-react";
import type { CSSProperties } from "react";
import type { components } from "../api/schema";
import { ordinalNumber } from "../format/format";
import { Badge, Button, Card, PendingBody } from "./atoms";
import { Meter } from "./readings";
import "./briefitem.css";

// BriefItemCard — one ranked entry of a Morning-Brief run, as the card the
// queue is built from: rank, the deal it is about, the §10.1 composite, the
// five-factor decomposition the composite is made of, the evidence behind it,
// the rep's own state, and the three verbs that move it.
//
// PRESENTATIONAL AND CONTROLLED. It fetches nothing and writes nothing: the
// verbs are callbacks and the two things that need a locale — a percentage and
// an instant — arrive as formatters. That is what lets one card serve the home
// queue, a story, and a test without any of them standing up a query client.
//
// Copy is the caller's, all of it, through `labels`. Nothing here knows a word
// of any language, which is the design-system rule and also the reason the
// evidence count arrives already pluralised: a card cannot choose between "1
// source" and "2 sources" for a language whose plural rules it has never seen.

type BriefItem = components["schemas"]["MorningBriefItem"];
type FeatureVector = components["schemas"]["MorningBriefFeatureVector"];

/** One dimension of the §10.1 decomposition. */
export type BriefFactorKey = keyof FeatureVector;

/** A verb that moves an item out of the queue, or defers it. */
export type BriefItemAction = "act" | "dismiss" | "snooze";

// The reading order of the vector. The composite is not the mean of these —
// it is a weighted score — so the order is the one the scoring reads in rather
// than anything derived from the values.
//
// A factor the contract adds and this list does not carry would render as five
// bars and a silence, which is the failure a reader cannot see. What catches it
// is `briefitem.test.tsx`'s bar census: its fixture is typed as the wire vector,
// so a sixth factor forces the fixture to grow and the count assertion to fail
// until this list grows with it.
const FACTOR_KEYS: readonly BriefFactorKey[] = [
  "winnability",
  "revenue",
  "timing",
  "momentum",
  "warmth",
];

/**
 * Every word this card says, in the reader's language.
 *
 * One object rather than fifteen props because they are one thing — the card's
 * vocabulary — and a caller that has translated four of them has a card that
 * says nothing about the other eleven.
 */
export type BriefItemLabels = Readonly<{
  /** Names the position for a reader who cannot see it is a rank ("Rank"). */
  rank: string;
  /** The composite's own row label ("Score"). */
  composite: string;
  /** One per factor. A `Record` over the whole union: a factor the contract
   *  grows cannot be left unnamed at the call site. */
  factors: Readonly<Record<BriefFactorKey, string>>;
  /** The evidence count, already pluralised ("3 sources"). */
  evidence: string;
  /** What an item with NO evidence says. Evidence-or-omit means a brief item
   *  always has some, so an empty list is a fault worth stating rather than an
   *  empty badge worth hiding. */
  evidenceNone: string;
  /** The deal link's text before the caller knows the deal's name. */
  openDeal: string;
  act: string;
  dismiss: string;
  snooze: string;
  /** The three settled states, as the word a badge carries. */
  acted: string;
  dismissed: string;
  snoozed: string;
  /** Prefixes the instant a snoozed item comes back at ("Back"). */
  resurfaces: string;
}>;

// The badge tone per state. `snoozed` keeps the accent because a deferral is
// still the rep's work — it is the one settled state that comes back — while
// acted and dismissed are verdicts and read as one.
const STATE_TONE: Record<
  BriefItem["state"],
  "success" | "warn" | "accent" | undefined
> = {
  new: undefined,
  acted: "success",
  dismissed: "warn",
  snoozed: "accent",
};

export function BriefItemCard({
  item,
  labels,
  dealName,
  amount,
  revenueBasisNote,
  formatPercent,
  formatInstant,
  pending,
  error,
  onOpenDeal,
  onAct,
  onDismiss,
  onSnooze,
  testId,
}: Readonly<{
  item: BriefItem;
  labels: BriefItemLabels;
  /**
   * The deal's name, once the caller has read the deal. Absent leaves
   * `labels.openDeal` on the link: a brief item always knows which deal it is
   * about, so the link is never withheld — only its name is not in yet.
   */
  dealName?: string;
  /**
   * The deal's money, already formatted (a caller with no figure passes the
   * absent marker). Null or absent draws nothing: an amount is the deal's
   * fact, not the item's, and a euro sign over an unknown currency reads as a
   * real EUR figure.
   */
  amount?: string | null;
  /**
   * What the revenue factor normalised AGAINST — the run's
   * `revenue_norm_minor`, as a sentence the caller composed. Without it the
   * revenue bar is the one factor a reader cannot check: 0.4 of an unnamed
   * base is not a reading, it is a number.
   */
  revenueBasisNote?: string;
  /** A 0..1 proportion as the reader's own percentage. Every figure on the
   *  card goes through this ONE function, so the composite and the bars cannot
   *  disagree about what 0.62 is called. */
  formatPercent: (fraction: number) => string;
  /** A UTC instant in the reader's locale and zone. */
  formatInstant: (utcIso: string) => string;
  /** The verb whose write is in flight. It marks that button busy and refuses
   *  the other two — a queue entry takes one decision at a time. */
  pending?: BriefItemAction;
  /** A refusal the last write came back with, in the reader's language. */
  error?: string;
  onOpenDeal: (dealId: string) => void;
  onAct: (itemId: string) => void;
  onDismiss: (itemId: string) => void;
  /** Fires with the item id only. WHEN a snooze comes back is the screen's
   *  policy and needs a clock, so the instant is composed there and this card
   *  reads no clock at all. */
  onSnooze: (itemId: string) => void;
  testId?: string;
}>) {
  // Acted and dismissed are verdicts: the item is done and offers no verbs.
  // Snoozed is not — it comes back, so it recedes with them but keeps its
  // buttons, which is how a rep pulls a deferred deal forward again.
  const closed = item.state === "acted" || item.state === "dismissed";
  const settled = item.state !== "new";
  return (
    <Card
      as="article"
      // `inset` IS the recede. The recessed ground, no shadow and no boundary
      // are already a card variant, so a settled item reads as the same card
      // sitting one layer back rather than as a second treatment invented here.
      inset={settled}
      className={settled ? "brief-item brief-item-quiet" : "brief-item"}
      testId={testId ?? `brief-item-${item.id}`}
    >
      <div className="brief-item-head">
        <span className="brief-item-rank t-mono">
          <span className="sr-only">{labels.rank}</span>#
          {ordinalNumber(item.rank)}
        </span>
        {/* Not a `Button`: this is the card's TITLE. Button owns control
            geometry — a shared 40px height, a width floor, its own icon sizing
            — and a headline wearing it stops reading as a headline. Same
            exception the conformance gate already makes for a link that looks
            like a button. */}
        <button
          type="button"
          className="brief-item-name"
          onClick={() => onOpenDeal(item.deal_id)}
        >
          <span className="brief-item-name-text">
            {dealName ?? labels.openDeal}
          </span>
          <ArrowRight aria-hidden="true" />
        </button>
        {amount !== undefined && amount !== null && (
          <span className="brief-item-amount t-mono">{amount}</span>
        )}
      </div>
      <BriefFactorVector
        item={item}
        labels={labels}
        formatPercent={formatPercent}
      />
      {revenueBasisNote !== undefined && (
        <p className="brief-item-basis t-caption">{revenueBasisNote}</p>
      )}
      <div className="brief-item-foot">
        <BriefItemEvidence item={item} labels={labels} />
        <BriefItemState
          item={item}
          labels={labels}
          formatInstant={formatInstant}
        />
        {!closed && (
          <span className="brief-item-actions">
            <Button
              small
              variant="primary"
              pending={pending === "act"}
              disabled={pending !== undefined && pending !== "act"}
              onClick={() => onAct(item.id)}
            >
              <Check aria-hidden="true" /> {labels.act}
            </Button>
            <Button
              small
              pending={pending === "snooze"}
              disabled={pending !== undefined && pending !== "snooze"}
              onClick={() => onSnooze(item.id)}
            >
              <AlarmClock aria-hidden="true" /> {labels.snooze}
            </Button>
            <Button
              small
              pending={pending === "dismiss"}
              disabled={pending !== undefined && pending !== "dismiss"}
              onClick={() => onDismiss(item.id)}
            >
              <X aria-hidden="true" /> {labels.dismiss}
            </Button>
          </span>
        )}
      </div>
      {error !== undefined && (
        // A refusal arrives AFTER a press, on a card the reader is already on,
        // so it is announced rather than left to be noticed.
        <p className="brief-item-error t-caption" role="alert">
          {error}
        </p>
      )}
    </Card>
  );
}

/**
 * The pending shape of one card.
 *
 * The queue's placeholder is a CARD rather than a bare row of bones, because
 * what arrives is cards: three loose bars where a card will be reflows the
 * whole column the moment the run lands. `label` is `PendingBody`'s required
 * announcement — a placeholder carries no text, so it is the only thing a
 * reader who cannot see the bars is given.
 */
export function BriefItemCardPending({
  label,
  testId,
}: Readonly<{ label: string; testId?: string }>) {
  return (
    <Card as="article" className="brief-item" testId={testId}>
      <PendingBody label={label} lines={4} />
    </Card>
  );
}

// The bars, and the whole reason this card is worth having in the design
// system: five proportions that have to read as ONE comparison.
//
// They share a grid, so every track starts and ends at the same x and the eye
// compares lengths instead of guessing at five separately-scaled bars. The
// composite is the grid's FIRST ROW rather than a figure parked in the header:
// same scale, same fill, same track, heavier ink. That is what stops the
// headline number and the decomposition from disagreeing visually — they are
// literally drawn on one axis.
function BriefFactorVector({
  item,
  labels,
  formatPercent,
}: Readonly<{
  item: BriefItem;
  labels: BriefItemLabels;
  formatPercent: (fraction: number) => string;
}>) {
  return (
    <div className="brief-item-bars">
      <BriefBar
        index={0}
        headline
        label={labels.composite}
        fraction={item.composite}
        formatPercent={formatPercent}
      />
      {FACTOR_KEYS.map((key, position) => (
        <BriefBar
          key={key}
          index={position + 1}
          label={labels.factors[key]}
          fraction={item.feature_vector[key]}
          formatPercent={formatPercent}
        />
      ))}
    </div>
  );
}

// The row's own stagger step, as a custom property the mount animation in the
// stylesheet multiplies by `--stagger-enter`. Typed alongside the standard
// properties so the object literal keeps its keys instead of casting them away.
type BarVars = CSSProperties & Record<`--${string}`, string | number>;

// One labelled proportion. `display: contents` on this wrapper is what puts the
// three cells into the grid ABOVE it while the row still owns its stagger index
// — custom properties inherit through it.
function BriefBar({
  index,
  label,
  fraction,
  formatPercent,
  headline,
}: Readonly<{
  index: number;
  label: string;
  fraction: number;
  formatPercent: (fraction: number) => string;
  headline?: boolean;
}>) {
  // A value outside 0..1 is a data fault, not a reason to draw a bar wider
  // than its track or a negative one. The clamp lands BEFORE the formatter so
  // the digits and the bar are the same number.
  const clamped = Math.max(0, Math.min(1, fraction));
  const vars: BarVars = { "--brief-bar-index": index };
  return (
    <div
      className={
        headline ? "brief-item-bar brief-item-bar-headline" : "brief-item-bar"
      }
      style={vars}
    >
      <span className="brief-item-bar-label">{label}</span>
      {/* Percent rather than the raw 0..1 pair: the fact IS a normalised
          proportion, and a screen reader saying "0.62 of 1" reads worse than
          "62 of 100" for the identical reading. */}
      <Meter
        value={Math.round(clamped * 100)}
        max={100}
        label={label}
        dense
        flat
      />
      <span className="brief-item-bar-value t-mono">
        {formatPercent(clamped)}
      </span>
    </div>
  );
}

// What is behind the score. The count is the reading — a raw evidence id is a
// uuid, which is not a thing a rep can act on — but ZERO is a different fact
// from a small number: the brief promises evidence-or-omit, so an item that
// arrived with none is a fault the card states instead of drawing an empty
// badge nobody can tell from a loading one.
function BriefItemEvidence({
  item,
  labels,
}: Readonly<{ item: BriefItem; labels: BriefItemLabels }>) {
  if (item.evidence_ids.length === 0) {
    return <Badge tone="warn">{labels.evidenceNone}</Badge>;
  }
  return <Badge quiet>{labels.evidence}</Badge>;
}

// The rep's own state, and WHEN it was set. `state_at` is half the fact: a
// dismissal from this morning and one from three weeks ago say different things
// about whether the queue is being worked, and the card used to show neither.
function BriefItemState({
  item,
  labels,
  formatInstant,
}: Readonly<{
  item: BriefItem;
  labels: BriefItemLabels;
  formatInstant: (utcIso: string) => string;
}>) {
  const word = stateWord(item.state, labels);
  if (word === undefined) {
    return null;
  }
  return (
    <span className="brief-item-state">
      <Badge tone={STATE_TONE[item.state]}>{word}</Badge>
      {item.state_at !== undefined && item.state_at !== null && (
        <time className="brief-item-when" dateTime={item.state_at}>
          {formatInstant(item.state_at)}
        </time>
      )}
      {/* A snooze's own state, said out loud. The instant is the whole point of
          the affordance — an item deferred to nowhere in particular is
          indistinguishable from one quietly dropped — and it is drawn even
          though the item is receded, because it is the one thing on a snoozed
          card the reader came back for. */}
      {item.snoozed_until !== undefined && item.snoozed_until !== null && (
        <span className="brief-item-resurface">
          <AlarmClock aria-hidden="true" />
          {labels.resurfaces}{" "}
          <time dateTime={item.snoozed_until}>
            {formatInstant(item.snoozed_until)}
          </time>
        </span>
      )}
    </span>
  );
}

// `new` carries no state word: it is the absence of a decision, and a badge
// saying so would put a status on every unworked row in the queue.
function stateWord(
  state: BriefItem["state"],
  labels: BriefItemLabels,
): string | undefined {
  switch (state) {
    case "acted":
      return labels.acted;
    case "dismissed":
      return labels.dismissed;
    case "snoozed":
      return labels.snoozed;
    case "new":
      return undefined;
  }
}
