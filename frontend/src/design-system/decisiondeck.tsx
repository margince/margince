// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useEffect, useRef, useState } from "react";
import { SegmentedControl } from "./atoms";
import type {
  DecisionApproval,
  DecisionCardLabels,
  DecisionCompactWords,
  DecisionDisplay,
} from "./decisioncard";
import {
  DeckQueue,
  DeckSurface,
  StagingTray,
  useCommitTakesFocus,
} from "./decisiondeck.frame";
import { DeckItemCard } from "./decisiondeck.item";
import { DeckStack, type Leaving, useDeckDrag } from "./decisiondeck.stack";
import {
  type DecisionSharedFacts,
  deckKeyHandler,
  itemLapsed,
  verdictSends,
} from "./decisiondeck.verdicts";
import { usePrefersReducedMotion } from "./motion";
import type { SectionDetail, SectionState } from "./surfacestate";
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

/** What a contact can say about one staged proposal. */
export type DeckVerdict = "accept" | "edit" | "reject" | "skip";

// The input vocabulary and the bundle readings live beside the deck rather than
// in it (decisiondeck.verdicts.ts), and are reached THROUGH it: a caller talking
// to the deck should not have to know which of its files answered.
export {
  type DecisionSharedFacts,
  dragVerdict,
  keyVerdict,
  sharedFacts,
} from "./decisiondeck.verdicts";

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
  /**
   * The two words that turn the LIST form's rows dense, and the switch that
   * turns it on: a surface that has them gets one line per decision with the
   * proposal behind a popover, and one that has not keeps the full row.
   *
   * Words rather than a flag for the reason `DecisionCompactWords` states: the
   * dense line hides the proposal behind one control and folds two verdicts
   * into another, and neither is reachable unnamed.
   */
  compactRow?: DecisionCompactWords;
}>;

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
   * Told when a CONTACT puts a verdict in the tray or takes it back out, for a
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
   * a contact's answers, and clearing them on failure would ask for all of them
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
  // Required, like the SurfaceState it forwards to: this deck cannot know what
  // its caller is waiting for, and a generic line is what the required prop
  // one level down exists to prevent.
  loadingLabel: string;
  /** Under the tray: what a refused commit said, in the caller's words. */
  notice?: ReactNode;
  chips?: (
    approval: DecisionApproval,
    shared: DecisionSharedFacts,
  ) => DecisionDeckChips;
  /**
   * Lets the SURFACE draw the deck's chrome: it is handed the view toggle and
   * the deck's whole content, and decides where each goes.
   *
   * For a deck that lives inside a `Panel`, where the toggle belongs in the
   * header band beside the panel's title — one band, one interval, the same
   * shape as every other zone on the page. Without this the deck draws its own
   * heading row, and a panel around it would then carry two headings: the
   * panel's title and a second one inside its body.
   *
   * It does NOT lift the deck's state. The toggle is the deck's own control,
   * already wired; the frame only says where it stands.
   */
  frame?: (parts: DeckFrame) => ReactNode;
  /**
   * How many rows the LIST form draws before it defers to the fuller surface.
   *
   * Absent draws them all, which is right for a deck that IS its page. A capped
   * list is for a page that OPENS with the decisions and goes on to something
   * else: three questions a reader can answer on the way past, and the rest
   * where every one of them is. The cap never reaches the deck form — a stack
   * hides what is behind the live card anyway, and capping it would leave a
   * count that says "4 more behind" over a pile that holds three.
   */
  listCap?: number;
  /** The way to the rest, drawn under a list the cap cut. */
  listRest?: (hidden: number) => ReactNode;
}>;

/**
 * The deck's parts, for a surface that frames them.
 *
 * Two rather than a slot per piece: the toggle is what a header band wants and
 * everything else — the body, the tray, the notice — is one block that belongs
 * under it in that order. A frame that could reorder those would be a second
 * opinion about what a reader reads first.
 */
export type DeckFrame = Readonly<{
  /** The Deck/List control, or null while there is nothing to switch between. */
  toggle: ReactNode;
  /**
   * The queue itself — and NULL while every card is staged, which is a state
   * with nothing to draw rather than an empty one: the tray below says what is
   * true, and a body reading "nothing is waiting on you" over a reader's own
   * staged verdicts would contradict it.
   */
  content: ReactNode;
  /**
   * The staging tray, and anything a refused commit said under it. Null while
   * the tray is empty and the last commit was clean.
   *
   * Apart from `content` because of WHERE it goes: it belongs to the whole zone
   * rather than to the queue, so a surface puts it in its own band — `Panel`'s
   * foot, edge to edge with a hairline over it — and the tray then stops
   * drawing the standalone box it needs when it floats at the foot of a page.
   */
  tray: ReactNode;
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
  frame,
  listCap,
  listRest,
}: DecisionDeckProps) {
  const reduced = usePrefersReducedMotion();
  // THE LIST IS THE DEFAULT, for every reader.
  //
  // A deck answers one decision at a time and hides the rest behind it, which
  // is the right shape for working through a pile and the wrong one for
  // arriving at a page: a reader opening their morning wants to see what is
  // waiting before they start answering it, and a deck tells them a count and
  // shows them one. The deck is a step away, on the switch beside the heading.
  //
  // It was already the default for a reader who asked for less motion, on the
  // reasoning that a deck IS its motion and one without it is a list drawn the
  // expensive way. That argument was never really about motion.
  const [view, setView] = useState<DeckView>("list");
  const [staged, setStaged] = useState<readonly StagedDecision[]>([]);
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
    const quiet = staged.filter((entry) => !verdictSends(entry));
    if (quiet.length > 0) {
      setDeferred((prev) => [...prev, ...quiet.map((entry) => entry.id)]);
      setStaged((prev) => prev.filter(verdictSends));
    }
  };

  const live = waiting[0];

  // THE GESTURE, wired to the one thing it can do. Its four handlers and the
  // drag they share live beside the stack they move (decisiondeck.frame.tsx):
  // what a finger is doing to a card is that surface's question, and the deck's
  // is what is waiting and what has been staged.
  const gesture = useDeckDrag({ live, onStage: stage });

  // Shortcuts fire only while the deck's own surface holds focus, never while a
  // control inside it does: Enter on the Accept button must accept that card,
  // not commit the whole tray behind it.
  const onKeyDown = deckKeyHandler({
    live,
    onStage: stage,
    onUnstage: unstage,
    onCommit: commit,
  });

  useCommitTakesFocus(waiting.length === 0 && staged.length > 0, commitRef);

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

  // What the LIST shows, and what it leaves to the surface behind it. The cap
  // is the list's alone: the stack already hides what is behind the live card,
  // and a capped stack would count what it does not hold.
  const listed = listCap === undefined ? waiting : waiting.slice(0, listCap);
  const hidden = waiting.length - listed.length;

  // EVERY CARD STAGED, and nothing sent yet: the queue is not empty and not
  // cleared, it is held in the tray. `SurfaceState`'s empty arm would say
  // "nothing is waiting on you" over the reader's own unsent verdicts, so the
  // queue draws nothing at all and the tray carries the sentence — which it
  // already does, by counting them.
  const allStaged = waiting.length === 0 && !cleared && staged.length > 0;

  // Built whether or not it is drawn, so the arms inside it stay at the top
  // level of this function rather than nesting one deeper inside the choice
  // below — three states of a queue read as three states, not as a branch of a
  // branch.
  const stack = (
    <DeckStack
      live={live}
      waiting={waiting}
      drag={gesture.drag}
      leaving={leaving}
      now={now}
      labels={labels}
      chips={chips}
      onStage={stage}
      onKeyDown={onKeyDown}
      {...gesture.handlers}
      onLeaveEnd={() => setLeaving(null)}
    />
  );
  const list = (
    <>
      <ul className="ddeck-list">
        {listed.map((item) => (
          <li key={item.id}>
            <DeckItemCard
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
      {/* A list showing three of nine that did not say where the other six are
          has hidden them. */}
      {hidden > 0 && listRest?.(hidden)}
    </>
  );
  const surface = (
    <DeckQueue
      state={resolved}
      stateDetail={stateDetail}
      loadingLabel={loadingLabel}
      labels={labels}
      tally={cleared ? tally : null}
      shown={view === "deck" ? stack : list}
    />
  );
  const queue = allStaged ? null : surface;

  // The band a surface puts at the foot of its own pane: what is held, and what
  // a refused commit said about the last press. Null when there is neither —
  // an empty band is a hairline under nothing.
  const trayBar = staged.length > 0 && (
    <StagingTray
      count={staged.length}
      labels={labels}
      commitState={commitState}
      commitRef={commitRef}
      onCommit={commit}
      onUnstage={unstage}
    />
  );
  const tray =
    trayBar || notice ? (
      <>
        {trayBar}
        {notice}
      </>
    ) : null;

  // FRAMED BY THE SURFACE, where one is offered: the toggle goes wherever that
  // surface keeps its controls — a panel's header band — and the deck claims no
  // region of its own, because the frame's own heading already names one and two
  // names for one zone put it in a screen reader's list twice.
  return (
    <DeckSurface
      deckLabel={labels.deckLabel}
      title={title}
      toggle={toggle}
      queue={queue}
      tray={tray}
      frame={frame}
    />
  );
}
