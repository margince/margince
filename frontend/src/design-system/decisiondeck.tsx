// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type CSSProperties,
  type KeyboardEvent,
  type PointerEvent,
  type ReactNode,
  type Ref,
  useEffect,
  useRef,
  useState,
} from "react";
import {
  Button,
  Disclosure,
  EmptyState,
  SectionHeader,
  SegmentedControl,
} from "./atoms";
import {
  type DecisionApproval,
  DecisionCard,
  type DecisionCardLabels,
  type DecisionDisplay,
  decisionLapsed,
} from "./decisioncard";
import { usePrefersReducedMotion } from "./motion";
import {
  type SectionDetail,
  type SectionState,
  SurfaceState,
} from "./surfacestate";
import type { ConfidenceLevel, Provenance } from "./trust";
import "./decisiondeck.css";

// DecisionDeck — the morning queue of staged proposals, answered one at a time.
//
// STAGE, THEN COMMIT, and that is the whole design rather than a flourish.
// `modules/approvals/service.go` is explicit that a recorded decision is
// un-undoable: approving mints a single-use token and executes the effect,
// rejecting closes the row, and neither has a reverse. A surface that sent a
// verdict on the swipe would therefore be a surface where a flick of the wrist
// sends an email. So a swipe or a key STAGES a verdict here, locally, and
// nothing leaves the browser until somebody presses commit — which makes the
// staging tray the undo the backend does not have. Un-staging is free right up
// to that press, and costs nothing after it because there is nothing to undo.
//
// It fetches nothing and mutates nothing. The items and the verbs arrive as
// props, the copy arrives as `labels`, and the screen that owns the mutations is
// the one that receives the committed verdicts.
//
// A BUNDLE IS ONE DECISION. The server stamps every proposal one act staged with
// a shared `bundle_id`, and the API decides the whole set in one call
// (`POST /approval-bundles/{bundle_id}/approve|reject`) — so a ten-recipient
// send is one question with ten recipients behind an expander, never ten
// questions. Rendered flat it would be ten answers to something the reader
// decided once.

/** What a person can say about one staged proposal. */
export type DeckVerdict = "accept" | "edit" | "reject" | "skip";

/**
 * One thing to decide: a proposal staged on its own, or one act's bundle, which
 * the API decides as a unit and which therefore reads as a single decision.
 *
 * A bundle of ONE is deliberately not a bundle — the caller emits it as a
 * `single`. A group holding a single child hides the very question it exists to
 * present, and the reader gains nothing for the click.
 */
export type DecisionDeckItem =
  | Readonly<{ kind: "single"; id: string; approval: DecisionApproval }>
  | Readonly<{
      kind: "bundle";
      id: string;
      bundleId: string;
      members: readonly DecisionApproval[];
    }>;

/** A verdict waiting in the tray, and what it answers. */
export type StagedDecision = Readonly<{
  id: string;
  verdict: DeckVerdict;
}>;

/**
 * The chips the CALLER owns on one card's meta line, plus its trust readings.
 * A function rather than a field on the item, because each of these needs
 * something this tier does not hold: the agent tier map, the kind catalog, the
 * signed-in reader's own id, the locale the countdown is spelled in.
 */
export type DecisionDeckChips = Readonly<{
  meta?: ReactNode;
  aside?: ReactNode;
  provenance?: Provenance;
  confidence?: ConfidenceLevel;
  /**
   * What THIS kind of proposal shows, in the reader's language. It rides here
   * for the same reason the chips do: resolving it needs the kind catalogue and
   * the locale, and this tier holds neither.
   */
  display?: readonly DecisionDisplay[];
}>;

/**
 * The words. All of them the caller's, for the reason `DecisionCardLabels` gives:
 * a primitive that carried its own copy would be the second author of the
 * product's vocabulary.
 *
 * The three counted ones are functions rather than templates because plural
 * agreement is a property of the language, not of this component — a caller
 * hands them the count and gets its own catalog's answer back.
 */
export type DecisionDeckLabels = Readonly<{
  /** Forwarded to every card. */
  card: DecisionCardLabels;
  /** Names the deck as a region, and names its keyboard surface. */
  deckLabel: string;
  /** The `[Deck | List]` toggle: the group's name and its two options. */
  viewLabel: string;
  viewDeck: string;
  viewList: string;
  /** The keyboard legend. Drawn, not hidden: a shortcut nobody is told about is
   *  a shortcut for whoever wrote it.
   *
   *  It must say that the arrows STAGE. This deck is stage-then-commit — an
   *  arrow moves a verdict into the tray and nothing leaves until `commit`
   *  runs — so a legend reading "→ accept" beside "Enter send" invites the
   *  reading where the arrow already sent it and Enter is a separate act. A
   *  reader who believes that presses four arrows, walks away, and has sent
   *  nothing. */
  keys: string;
  /** How many cards are still behind the live one. */
  behind: (count: number) => string;
  /** The tray: how many verdicts are waiting, and the two controls over them. */
  staged: (count: number) => string;
  commit: string;
  unstage: string;
  /** The earned moment: the queue is clear, this many were decided, at this time. */
  clearedTitle: string;
  cleared: (count: number) => string;
  clearedTime: (atMs: number) => string;
  /** There was never anything here — the ONE sentence allowed to say that. */
  empty: string;
  /** A bundle, as one decision with N members behind it. */
  bundleSummary: (members: number) => string;
  bundleMembers: (members: number) => string;
}>;

/** How far a drag must travel before it is a verdict rather than a nudge. */
const DRAG_THRESHOLD_PX = 72;

/** How much the live card leans as it is dragged: one degree per this many px. */
const DRAG_ROTATION_DIVISOR = 22;

/** How many card edges peek out behind the live one. */
const PEEK_DEPTH = 2;

/**
 * The verdict a finished drag means, or null for one that did not travel far
 * enough and springs back. The DOMINANT axis decides, so a diagonal drag is
 * whichever direction it mostly went rather than two verdicts at once.
 */
export function dragVerdict(dx: number, dy: number): DeckVerdict | null {
  const horizontal = Math.abs(dx) >= Math.abs(dy);
  const travel = horizontal ? Math.abs(dx) : Math.abs(dy);
  if (travel < DRAG_THRESHOLD_PX) {
    return null;
  }
  if (horizontal) {
    return dx > 0 ? "accept" : "reject";
  }
  return dy < 0 ? "edit" : "skip";
}

/**
 * The verdict a key means, or null for a key this deck does not claim. Exported
 * so the keyboard and the pointer are provably the same vocabulary rather than
 * two lists that happen to agree today.
 */
export function keyVerdict(key: string): DeckVerdict | null {
  if (key === "ArrowRight") {
    return "accept";
  }
  if (key === "ArrowLeft") {
    return "reject";
  }
  if (key === "ArrowUp") {
    return "edit";
  }
  return key === "ArrowDown" ? "skip" : null;
}

/**
 * The approval a card draws for one item.
 *
 * A bundle shows the first member that has NOT lapsed, and that choice is what
 * keeps the card's reading and the deck's Accept guard from contradicting each
 * other: the API decides every still-pending member in one call, so a bundle
 * whose oldest member ran out of time is still answerable, and a card drawing
 * that member would say "expired" over a decision the reader may still make.
 * With every member lapsed there is nothing to choose, and the first one is as
 * honest as any other.
 */
function representative(item: DecisionDeckItem, now: number): DecisionApproval {
  if (item.kind === "single") {
    return item.approval;
  }
  return (
    item.members.find((member) => !decisionLapsed(member, now)) ??
    item.members[0]
  );
}

/** Whether there is anything left on this item to accept. */
function itemLapsed(item: DecisionDeckItem, now: number): boolean {
  return decisionLapsed(representative(item, now), now);
}

/**
 * The facts a card may state about the WHOLE item — never the representative's
 * alone.
 *
 * Drawing a bundle from one member is right for what the card DECIDES and wrong
 * for what it CLAIMS. A ten-recipient send staged by two agents at two
 * confidences read as one agent at one confidence, and the reader answered all
 * ten on that reading. So a fact its members do not share is absent here rather
 * than sampled from one of them: an omitted chip is honest, a wrong one is not.
 *
 * Absent is also what a fact nobody recorded looks like, and that collapse is
 * deliberate — both mean "this card cannot say", which is the whole of what a
 * chip could truthfully report either way.
 */
export type DecisionSharedFacts = Readonly<{
  kind?: string;
  proposedBy?: string;
  confidence?: number;
}>;

/**
 * What every member of an item agrees on. A single agrees with itself, so it
 * carries its own facts whole.
 *
 * The chips are built from THIS rather than from the drawn approval, which is
 * what keeps the rule from being one a caller has to remember: a fact the
 * members disagree on is not merely discouraged as a chip, it is not there to
 * draw one from.
 */
export function sharedFacts(item: DecisionDeckItem): DecisionSharedFacts {
  const members = item.kind === "single" ? [item.approval] : item.members;
  return {
    kind: agreed(members, (member) => member.kind),
    proposedBy: agreed(members, (member) => member.proposed_by),
    confidence: agreed(members, (member) => member.confidence),
  };
}

/**
 * One fact across the members, or undefined where any two disagree.
 *
 * A member that never carried the fact needs no arm of its own: it reads as
 * absent, a set holding one absence and one value does not agree, and a set of
 * absences agrees on nothing a chip could print.
 */
function agreed<T>(
  members: readonly DecisionApproval[],
  read: (member: DecisionApproval) => T | null | undefined,
): T | undefined {
  const values = members.map(read);
  const first = values[0];
  return first != null && values.every((value) => value === first)
    ? first
    : undefined;
}

// What the deck's body is in, given what the caller knows and what the deck can
// see. A caller's non-ready state wins — a failed read is a fact the deck has no
// way to discover — and `ready` defers, because only the deck knows whether
// anything is left to draw.
function bodyState(
  state: SectionState | undefined,
  hasItems: boolean,
  cleared: boolean,
): SectionState {
  if (state && state !== "ready") {
    return state;
  }
  return hasItems || cleared ? "ready" : "empty";
}

type DeckView = "deck" | "list";

// A drag in progress. The ORIGIN is kept and the offset derived from it, rather
// than accumulating `movementX`: a pointer that leaves the element, or a host
// that reports no movement deltas at all, still yields the honest distance from
// where the finger went down.
type Drag = Readonly<{
  pointerId: number;
  startX: number;
  startY: number;
  dx: number;
  dy: number;
}>;

/**
 * The card on its way out: which verdict sent it, and WHERE THE HAND LET GO.
 *
 * The offset is the whole point of this being an object rather than a verdict.
 * A card that leaves from the middle of the plate after a swipe that ended 90px
 * to the right reads as a second, different card — the eye tracks the one it was
 * dragging, and losing it at the release point is what made the exit look like a
 * snap-back. Staged from the keyboard there is no gesture to continue, and the
 * offsets are zero.
 */
type Leaving = Readonly<{
  verdict: DeckVerdict;
  dx: number;
  dy: number;
}>;

/** The release point, handed to the exit animation as custom properties. */
type GhostVars = CSSProperties & Record<`--${string}`, string>;

/**
 * The release point as the style the exit opens on. Declared as `GhostVars`
 * rather than asserted at the call site: the type is what makes the custom
 * properties checkable, and an assertion would say the same thing while
 * switching the checking off.
 */
function ghostVars(leaving: Leaving): GhostVars {
  return {
    "--ddeck-from-x": `${leaving.dx}px`,
    "--ddeck-from-y": `${leaving.dy}px`,
    "--ddeck-from-rot": `${leaving.dx / DRAG_ROTATION_DIVISOR}deg`,
  };
}

export type DecisionDeckProps = Readonly<{
  /** Everything still waiting. The deck reorders nothing. */
  items: readonly DecisionDeckItem[];
  /**
   * The instant deadlines are judged against, in wall-clock ms. A prop for the
   * reason `DecisionCard` takes one: a story pins it, and a test does not depend
   * on what time the suite happens to run at.
   */
  now: number;
  labels: DecisionDeckLabels;
  /**
   * The deck's own heading, drawn on the same row as the Deck/List toggle. A
   * prop rather than a caller-side `SectionHeader` because a title above the
   * deck and a toggle inside it are two rows saying one thing, and the row the
   * toggle belongs to is the one that names what it switches. Omitted, the deck
   * carries no heading and the toggle keeps the row to itself.
   */
  title?: string;
  /** Fired ONCE, on the explicit commit, with exactly what is in the tray. */
  onCommit: (staged: readonly StagedDecision[]) => void;
  /**
   * Told when a PERSON puts a verdict in the tray or takes it back out, for a
   * caller keeping a count of its own. Neither sends anything.
   *
   * User-triggered only, and deliberately: the tray also empties on its own —
   * when a committed item leaves the queue, and when a verdict that sends
   * nothing is committed — and reporting those as un-staging would tell a caller
   * that somebody changed their mind about a decision that has already gone.
   */
  onStage?: (staged: StagedDecision) => void;
  onUnstage?: (staged: StagedDecision) => void;
  /**
   * Whether the commit the caller was handed is still in flight, or came back
   * refused. A refused commit KEEPS the tray: the verdicts are the only copy of
   * a person's answers, and clearing them on failure would ask for all of them
   * again.
   */
  commitState?: "idle" | "sending" | "failed";
  /** What the caller says about the read behind these items. `ready` defers to
   *  what the deck can see; anything else wins. */
  state?: SectionState;
  /** What the four honest-but-not-ready states need in order to be actionable —
   *  above all a retry, without which `failed` is `unavailable` with extra
   *  words. */
  stateDetail?: SectionDetail;
  loadingLabel?: string;
  /** Under the tray: what a refused commit said, in the caller's words. */
  notice?: ReactNode;
  chips?: (
    approval: DecisionApproval,
    shared: DecisionSharedFacts,
  ) => DecisionDeckChips;
}>;

export function DecisionDeck({
  items,
  now,
  labels,
  title,
  onCommit,
  onStage,
  onUnstage,
  commitState = "idle",
  state,
  stateDetail,
  loadingLabel,
  notice,
  chips,
}: DecisionDeckProps) {
  const reduced = usePrefersReducedMotion();
  // The list is the default for a reader who asked for less motion: a deck IS
  // its motion, and one without it is a list drawn the expensive way.
  const [view, setView] = useState<DeckView>(reduced ? "list" : "deck");
  const [staged, setStaged] = useState<readonly StagedDecision[]>([]);
  const [drag, setDrag] = useState<Drag | null>(null);
  // The card flying off, kept only until its animation ends. It carries no text
  // and no control — it is the silhouette leaving, so nothing a reader or a test
  // can reach is on screen twice.
  const [leaving, setLeaving] = useState<Leaving | null>(null);
  const [tally, setTally] = useState<{ count: number; at: number } | null>(
    null,
  );
  // Answered with a verdict that SENDS NOTHING — later, or edit-elsewhere — and
  // committed. Those items are still pending on the server, so they never leave
  // `items` and the tray would hold them for the rest of the session: the plate
  // read "clear" with a commit control over a tray whose only contents could be
  // pressed forever without anything happening. They are held here instead, out
  // of the deck and out of the tray, which is what "later" means.
  const [deferred, setDeferred] = useState<readonly string[]>([]);
  const commitRef = useRef<HTMLButtonElement>(null);

  const held = new Set([...staged.map((entry) => entry.id), ...deferred]);
  const waiting = items.filter((item) => !held.has(item.id));

  // A staged verdict whose item has left `items` was DECIDED — the caller sent
  // it and the queue answered without it. That is the one signal this component
  // has that a commit landed, and it is a better one than a success callback:
  // it cannot claim a decision the list still shows as waiting.
  useEffect(() => {
    const present = new Set(items.map((item) => item.id));
    const gone = staged.filter((entry) => !present.has(entry.id));
    if (gone.length === 0) {
      return;
    }
    setStaged((prev) => prev.filter((entry) => present.has(entry.id)));
    setTally((prev) => ({ count: (prev?.count ?? 0) + gone.length, at: now }));
  }, [items, staged, now]);

  const stage = (
    item: DecisionDeckItem,
    verdict: DeckVerdict,
    from?: { dx: number; dy: number },
  ) => {
    // A lapsed proposal cannot be accepted, and the guard lives here rather than
    // only on the card: the card withholds the button, and the keyboard and the
    // swipe reach the same verdict without one.
    if (verdict === "accept" && itemLapsed(item, now)) {
      return;
    }
    const entry = { id: item.id, verdict };
    // Re-answering an item replaces its verdict rather than queueing a second
    // one: a tray holding "accept" and "reject" for one proposal is a tray with
    // no answer in it.
    setStaged((prev) => [...prev.filter((held) => held.id !== item.id), entry]);
    setLeaving(
      reduced ? null : { verdict, dx: from?.dx ?? 0, dy: from?.dy ?? 0 },
    );
    onStage?.(entry);
  };

  const unstage = () => {
    const last = staged.at(-1);
    if (!last) {
      return;
    }
    setStaged((prev) => prev.slice(0, -1));
    onUnstage?.(last);
  };

  /** Whether this verdict is one the caller will actually send. */
  const sends = (entry: StagedDecision) =>
    entry.verdict === "accept" || entry.verdict === "reject";

  const commit = () => {
    if (staged.length === 0 || commitState === "sending") {
      return;
    }
    onCommit(staged);
    // The tray keeps only what is now in flight. A verdict that sends nothing
    // has done everything it is going to do at the moment of the press — later
    // means later, and an edit is answered on the queue's own form — so it moves
    // out of the tray and out of the deck rather than sitting under a commit
    // control that can be pressed again to no effect.
    const quiet = staged.filter((entry) => !sends(entry));
    if (quiet.length > 0) {
      setDeferred((prev) => [...prev, ...quiet.map((entry) => entry.id)]);
      setStaged((prev) => prev.filter(sends));
    }
  };

  const live = waiting[0];

  const onPointerDown = (event: PointerEvent<HTMLFieldSetElement>) => {
    if (!live || event.button !== 0) {
      return;
    }
    // A press that STARTS on a control belongs to that control, and this guard
    // is what makes the four buttons on a deck card work at all. Pointer capture
    // retargets every later event for that pointer — including the compatibility
    // `click` — at the capturing element, so capturing here sent the click to
    // this fieldset and the button under the finger never heard about it. Accept
    // did nothing in the deck and worked in the list, which is exactly the shape
    // of the bug that was reported. `closest` rather than a tag test: the press
    // lands on the label or the icon inside the button.
    if (
      event.target instanceof Element &&
      event.target.closest("button, a, input, textarea, select, summary")
    ) {
      return;
    }
    event.currentTarget.setPointerCapture?.(event.pointerId);
    setDrag({
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      dx: 0,
      dy: 0,
    });
  };

  const onPointerMove = (event: PointerEvent<HTMLFieldSetElement>) => {
    const { clientX, clientY, pointerId } = event;
    setDrag((held) =>
      held && held.pointerId === pointerId
        ? { ...held, dx: clientX - held.startX, dy: clientY - held.startY }
        : held,
    );
  };

  const onPointerUp = (event: PointerEvent<HTMLFieldSetElement>) => {
    if (!drag || drag.pointerId !== event.pointerId) {
      return;
    }
    setDrag(null);
    // Measured from THIS event, not from the last move. A pointer that travels
    // and lifts without another `pointermove` in between — a fast flick, a
    // synthetic release, a pen leaving the surface — would otherwise be judged
    // on where it last was seen rather than on where it let go, which decides
    // both the verdict and where the exit starts.
    const dx = event.clientX - drag.startX;
    const dy = event.clientY - drag.startY;
    const verdict = dragVerdict(dx, dy);
    if (verdict && live) {
      // Where the hand let go, so the card continues out from there rather than
      // restarting its flight from the middle of the plate.
      stage(live, verdict, { dx, dy });
    }
  };

  /**
   * The gesture taken away rather than finished — the browser cancelling the
   * pointer (a system gesture, a scroll takeover, the pen leaving range).
   *
   * Nothing is staged. A cancelled drag is one the person never completed, and
   * treating it as a release meant the system could decide a proposal on the
   * reader's behalf: a swipe interrupted past the threshold sent the card. The
   * card springs back instead, which is what a cancellation looks like.
   */
  const onPointerCancel = (event: PointerEvent<HTMLFieldSetElement>) => {
    if (drag && drag.pointerId === event.pointerId) {
      setDrag(null);
    }
  };

  // Shortcuts fire only while the deck's own surface holds focus, never while a
  // control inside it does: Enter on the Accept button must accept that card,
  // not commit the whole tray behind it.
  const onKeyDown = (event: KeyboardEvent<HTMLFieldSetElement>) => {
    if (event.target !== event.currentTarget) {
      return;
    }
    const verdict = keyVerdict(event.key);
    if (verdict && live) {
      event.preventDefault();
      stage(live, verdict);
      return;
    }
    if (event.key === "u" || event.key === "U") {
      event.preventDefault();
      unstage();
      return;
    }
    if (event.key === "Enter") {
      event.preventDefault();
      commit();
    }
  };

  // Staging the LAST card takes the keyboard surface off the screen with it —
  // there is no live card left to hold a tab stop — so the commit control takes
  // the focus the deck just lost. Without this the reader who worked the whole
  // queue from the keyboard is left with focus on the document body and the one
  // press that matters unreachable except by tabbing back in from the top.
  const emptied = waiting.length === 0 && staged.length > 0;
  useEffect(() => {
    if (emptied) {
      commitRef.current?.focus();
    }
  }, [emptied]);

  const cleared = waiting.length === 0 && (tally?.count ?? 0) > 0;
  const resolved = bodyState(state, waiting.length > 0, cleared);

  // The toggle asks HOW to show what is waiting, so it exists only while
  // something is. Over a cleared plate or an empty queue it is a control with
  // nothing to switch between — noise on the one screen whose whole point is
  // that there is nothing left to do.
  const toggle =
    waiting.length > 0 ? (
      <SegmentedControl
        options={["deck", "list"] as const}
        value={view}
        onChange={setView}
        label={labels.viewLabel}
        labels={{ deck: labels.viewDeck, list: labels.viewList }}
      />
    ) : null;

  return (
    <section className="ddeck" aria-label={labels.deckLabel}>
      {/* Titled, the heading and the toggle are ONE row: `SectionHeader` already
          lays a title against its own controls, so the deck reuses it rather
          than growing a second header that would have to agree with it. */}
      {title ? (
        <SectionHeader level={2} title={title} actions={toggle} />
      ) : (
        toggle && <div className="ddeck-head">{toggle}</div>
      )}
      <SurfaceState
        state={resolved}
        emptyLabel={labels.empty}
        loadingLabel={loadingLabel}
        detail={stateDetail}
      >
        {cleared && tally ? (
          <EmptyState title={labels.clearedTitle}>
            <p className="t-body">{labels.cleared(tally.count)}</p>
            <p className="t-small ddeck-cleared-time">
              {labels.clearedTime(tally.at)}
            </p>
          </EmptyState>
        ) : view === "deck" ? (
          <DeckStack
            live={live}
            waiting={waiting}
            drag={drag}
            leaving={leaving}
            now={now}
            labels={labels}
            chips={chips}
            onStage={stage}
            onKeyDown={onKeyDown}
            onPointerDown={onPointerDown}
            onPointerMove={onPointerMove}
            onPointerUp={onPointerUp}
            onPointerCancel={onPointerCancel}
            onLeaveEnd={() => setLeaving(null)}
          />
        ) : (
          <ul className="ddeck-list">
            {waiting.map((item) => (
              <li key={item.id}>
                <ItemCard
                  item={item}
                  layout="row"
                  now={now}
                  labels={labels}
                  chips={chips}
                  onStage={stage}
                />
              </li>
            ))}
          </ul>
        )}
      </SurfaceState>
      {staged.length > 0 && (
        <StagingTray
          count={staged.length}
          labels={labels}
          commitState={commitState}
          commitRef={commitRef}
          onCommit={commit}
          onUnstage={unstage}
        />
      )}
      {notice}
    </section>
  );
}

// The tray. A live region, because the count changing is the whole feedback a
// staged verdict gets — a reader driving this from the keyboard has to hear that
// their swipe landed somewhere.
function StagingTray({
  count,
  labels,
  commitState,
  commitRef,
  onCommit,
  onUnstage,
}: Readonly<{
  count: number;
  labels: DecisionDeckLabels;
  commitState: "idle" | "sending" | "failed";
  // Handed down so the deck can move focus here when the last card leaves the
  // stack — the tray becomes the only thing left to press.
  commitRef: Ref<HTMLButtonElement>;
  onCommit: () => void;
  onUnstage: () => void;
}>) {
  return (
    <div className="ddeck-tray" role="status">
      <span className="ddeck-tray-count">{labels.staged(count)}</span>
      <Button small onClick={onUnstage} disabled={commitState === "sending"}>
        {labels.unstage}
      </Button>
      <Button
        ref={commitRef}
        variant="primary"
        small
        pending={commitState === "sending"}
        onClick={onCommit}
      >
        {labels.commit}
      </Button>
    </div>
  );
}

// The stack: the live card, the edges of the two behind it, and how many more
// there are. The peeked plates are blank and `aria-hidden` — they are depth, not
// content, and a screen reader announcing two empty cards would be describing
// the drawing rather than the queue.
function DeckStack({
  live,
  waiting,
  drag,
  leaving,
  now,
  labels,
  chips,
  onStage,
  onKeyDown,
  onPointerDown,
  onPointerMove,
  onPointerUp,
  onPointerCancel,
  onLeaveEnd,
}: Readonly<{
  live: DecisionDeckItem | undefined;
  waiting: readonly DecisionDeckItem[];
  drag: Drag | null;
  leaving: Leaving | null;
  now: number;
  labels: DecisionDeckLabels;
  chips?: (
    approval: DecisionApproval,
    shared: DecisionSharedFacts,
  ) => DecisionDeckChips;
  onStage: (
    item: DecisionDeckItem,
    verdict: DeckVerdict,
    from?: { dx: number; dy: number },
  ) => void;
  onKeyDown: (event: KeyboardEvent<HTMLFieldSetElement>) => void;
  onPointerDown: (event: PointerEvent<HTMLFieldSetElement>) => void;
  onPointerMove: (event: PointerEvent<HTMLFieldSetElement>) => void;
  onPointerUp: (event: PointerEvent<HTMLFieldSetElement>) => void;
  onPointerCancel: (event: PointerEvent<HTMLFieldSetElement>) => void;
  onLeaveEnd: () => void;
}>) {
  if (!live) {
    return null;
  }
  const behind = waiting.length - 1;
  const peeks = Math.min(behind, PEEK_DEPTH);
  return (
    <div className="ddeck-stack">
      {/*
        The plate: the live card and the edges behind it, and NOTHING else. The
        peeked layers are absolutely positioned against this box rather than
        against the whole stack, which is what keeps them off the two lines
        underneath — sized to the stack they covered the count and the keyboard
        legend, and the legend is the only thing that tells a reader the arrow
        keys exist at all.
      */}
      <div className="ddeck-plate">
        {Array.from({ length: peeks }, (_, index) => (
          <div
            className="ddeck-peek"
            key={waiting[index + 1].id}
            data-depth={index + 1}
            aria-hidden="true"
          />
        ))}
        {leaving && (
          <div
            className="ddeck-ghost"
            data-verdict={leaving.verdict}
            aria-hidden="true"
            // Where the flight STARTS: the point the hand released, as the
            // transform the keyframes open on. Custom properties rather than a
            // second keyframe set per verdict — the offset is data, and eight
            // hand-written keyframes to say four directions from an arbitrary
            // point is not.
            style={ghostVars(leaving)}
            onAnimationEnd={onLeaveEnd}
          />
        )}
        {/*
        The keyboard surface, and a real `fieldset` rather than a div wearing
        `role="group"`: what is inside it IS a set of controls that belong
        together — the four verbs — which is the same reasoning `ChoiceList` and
        the list surface's `Menu` are built on, and the browser then exposes the
        grouping without an ARIA role on top of it.

        It takes a tab stop of its own, on top of the four buttons inside it, and
        that is the point rather than an oversight: the arrow keys ARE the deck,
        so a reader who cannot use a pointer needs somewhere to stand in order to
        press them. `aria-keyshortcuts` is how they are told, and the legend
        under the card is how everyone else is.
      */}
        <fieldset
          className="ddeck-live"
          // biome-ignore lint/a11y/noNoninteractiveTabindex: the tab stop is what makes the four arrow-key verdicts reachable at all — a swipe surface with no keyboard equivalent is a surface only a pointer can answer
          tabIndex={0}
          aria-label={labels.deckLabel}
          aria-keyshortcuts="ArrowRight ArrowLeft ArrowUp ArrowDown U Enter"
          onKeyDown={onKeyDown}
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          onPointerCancel={onPointerCancel}
        >
          {/*
            The moving part, and KEYED ON THE CARD.

            Everything that moves lives in here rather than on the fieldset, and
            the split is what fixes two things at once. Keyed, React mounts a
            fresh box for the next card — so the transform the swipe left behind
            cannot transition back to zero on the card that has just arrived,
            which is exactly the "the same card snapped back into the pile" this
            deck was reported for, and the arrival animation plays because a
            remount is what plays it. And because the KEYBOARD surface is the
            fieldset outside it, none of that touches focus: a reader working the
            queue with the arrow keys keeps their tab stop through every verdict.
          */}
          <div
            key={live.id}
            className="ddeck-card"
            data-dragging={drag ? "" : undefined}
            // The verdict this drag WOULD stage, once it has travelled far
            // enough to be one. Drawn as a ring in the verdict's own colour, so
            // a reader learns what right and left mean while their finger is
            // still down rather than after the card has gone. It says nothing an
            // assistive reader is missing: the four buttons inside carry the same
            // four verdicts in words at every moment.
            data-verdict={
              (drag ? dragVerdict(drag.dx, drag.dy) : null) ?? undefined
            }
            style={
              drag
                ? {
                    transform: `translate(${drag.dx}px, ${drag.dy}px) rotate(${
                      drag.dx / DRAG_ROTATION_DIVISOR
                    }deg)`,
                  }
                : undefined
            }
          >
            <ItemCard
              item={live}
              layout="deck"
              now={now}
              labels={labels}
              chips={chips}
              onStage={onStage}
            />
          </div>
        </fieldset>
      </div>
      <p className="t-small ddeck-behind">
        {labels.behind(Math.max(behind, 0))}
      </p>
      <p className="t-small ddeck-keys">{labels.keys}</p>
    </div>
  );
}

// One item as a card. A bundle carries the count of what saying yes would decide
// on its meta line and its members behind an expander, because the API decides
// the set in one call and the reader answers it once.
function ItemCard({
  item,
  layout,
  now,
  labels,
  chips,
  onStage,
}: Readonly<{
  item: DecisionDeckItem;
  layout: "deck" | "row";
  now: number;
  labels: DecisionDeckLabels;
  chips?: (
    approval: DecisionApproval,
    shared: DecisionSharedFacts,
  ) => DecisionDeckChips;
  onStage: (item: DecisionDeckItem, verdict: DeckVerdict) => void;
}>) {
  const approval = representative(item, now);
  const trim = chips?.(approval, sharedFacts(item)) ?? {};
  const members = item.kind === "bundle" ? item.members : [];
  return (
    <DecisionCard
      approval={approval}
      layout={layout}
      now={now}
      labels={labels.card}
      provenance={trim.provenance}
      confidence={trim.confidence}
      display={trim.display}
      aside={trim.aside}
      meta={
        <>
          {trim.meta}
          {members.length > 0 && (
            <span className="ddeck-bundle-count">
              {labels.bundleSummary(members.length)}
            </span>
          )}
        </>
      }
      detail={
        members.length > 0 ? (
          <Disclosure
            className="ddeck-bundle-open"
            summary={labels.bundleMembers(members.length)}
          >
            {/* A list, not an indent: the size and the boundaries of the set
                have to reach a reader who is hearing this page. */}
            <ul className="ddeck-bundle-members">
              {members.map((member) => (
                <li key={member.id} className="t-small">
                  {member.summary ?? member.kind}
                </li>
              ))}
            </ul>
          </Disclosure>
        ) : undefined
      }
      onAccept={() => onStage(item, "accept")}
      onEdit={() => onStage(item, "edit")}
      onReject={() => onStage(item, "reject")}
      onSkip={() => onStage(item, "skip")}
    />
  );
}
