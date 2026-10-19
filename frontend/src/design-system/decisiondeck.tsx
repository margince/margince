// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useRef, useState } from "react";
import { SegmentedControl } from "./atoms";
import { Callout } from "./callout";
import {
  DeckQueue,
  DeckSurface,
  StagingTray,
  useCommitTakesFocus,
} from "./decisiondeck.frame";
import { DeckItemCard } from "./decisiondeck.item";
import { DeckStack, type Leaving, useDeckDrag } from "./decisiondeck.stack";
import {
  deckKeyHandler,
  itemAcceptable,
  verdictSends,
} from "./decisiondeck.verdicts";
import { usePrefersReducedMotion } from "./motion";
import type { SectionState } from "./surfacestate";
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
import type {
  DecisionDeckItem,
  DecisionDeckProps,
  DeckVerdict,
  StagedDecision,
} from "./decisiondeckprops";

export type {
  DecisionDeckChips,
  DecisionDeckItem,
  DecisionDeckLabels,
  DecisionDeckProps,
  DeckFrame,
  DeckVerdict,
  StagedDecision,
} from "./decisiondeckprops";

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
  const [acceptanceChanged, setAcceptanceChanged] = useState(false);
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
    if (verdict === "accept" && !itemAcceptable(item, now, chips)) {
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
    const unavailable = new Set(
      items
        .filter((item) => !itemAcceptable(item, now, chips))
        .map((item) => item.id),
    );
    const answerable = staged.filter(
      (entry) => entry.verdict !== "accept" || !unavailable.has(entry.id),
    );
    if (answerable.length !== staged.length) {
      setStaged(answerable);
      setAcceptanceChanged(true);
    }
    if (answerable.length === 0) return;
    onCommit(answerable);
    // The tray keeps only what is now in flight: a verdict that sends nothing
    // has done everything it will do at the press, so it leaves the tray and
    // the deck rather than sitting under a control that can be pressed again
    // to no effect.
    const quiet = answerable.filter((entry) => !verdictSends(entry));
    if (quiet.length > 0) {
      setDeferred((prev) => [...prev, ...quiet.map((entry) => entry.id)]);
      setStaged((prev) => prev.filter(verdictSends));
    }
  };

  const live = waiting[0];
  // THE GESTURE, wired to the one thing it can do. Its handlers live beside the
  // stack they move (decisiondeck.frame.tsx): what a finger does to a card is
  // that surface's question, and the deck's is what is waiting and staged.
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
      staged={staged}
      labels={labels}
      commitState={commitState}
      commitRef={commitRef}
      onCommit={commit}
      onUnstage={unstage}
    />
  );
  const tray =
    trayBar || notice || acceptanceChanged ? (
      <>
        {trayBar}
        {notice}
        {acceptanceChanged && (
          <Callout
            tone="warning"
            kind="outcome"
            title={labels.acceptanceChanged}
          />
        )}
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

// The input vocabulary and the bundle readings live beside the deck, in
// decisiondeck.verdicts.ts, and are reached through it. A caller talking to the
// deck should not have to know which of its files answered.
export {
  type DecisionSharedFacts,
  dragVerdict,
  keyVerdict,
  sharedFacts,
} from "./decisiondeck.verdicts";
