// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, type Ref, type RefObject, useEffect } from "react";
import { Button, EmptyState, SectionHeader } from "./atoms";
import type { DecisionDeckLabels, DeckFrame } from "./decisiondeck";
import {
  type SectionDetail,
  type SectionState,
  SurfaceState,
} from "./surfacestate";

// THE TWO PARTS A SURFACE PLACES: what the deck is showing, and what it is
// holding.
//
// They are the deck's `DeckFrame` made concrete — the stack of cards a reader
// answers one at a time, and the tray of verdicts nothing has sent yet — and
// they live beside the deck rather than in it because the deck's own file is
// the QUEUE's logic: what is waiting, what is staged, what a gesture meant.
// How a card leans under a finger and how a tray reports what it holds are two
// pictures of that state, and a reader after either was walking the whole
// component to find it.
/**
 * WHERE THE DECK'S THREE PARTS GO.
 *
 * Either the surface around it places them — `frame` is handed the toggle, the
 * queue and the tray and decides which band each belongs in — or the deck draws
 * its own region and stacks them itself. A framed deck claims NO region and no
 * name: the frame's own heading already names one, and two names for one zone
 * put it in a screen reader's landmark list twice.
 */
export function DeckSurface({
  deckLabel,
  title,
  toggle,
  queue,
  tray,
  frame,
}: Readonly<{
  deckLabel: string;
  title?: string;
  toggle: ReactNode;
  queue: ReactNode;
  tray: ReactNode;
  frame?: (parts: DeckFrame) => ReactNode;
}>) {
  if (frame) {
    return frame({ toggle, content: queue, tray });
  }
  return (
    <section className="ddeck" aria-label={deckLabel}>
      {/* Titled, the heading and the toggle are ONE row: `SectionHeader`
          already lays a title against its own controls, so the deck reuses it
          rather than growing a second header that would have to agree with
          it. */}
      {title ? (
        <SectionHeader level={2} title={title} actions={toggle} />
      ) : (
        toggle && <div className="ddeck-head">{toggle}</div>
      )}
      {queue}
      {tray}
    </section>
  );
}

/**
 * The commit control takes the focus the deck lost.
 *
 * Staging the LAST card takes the keyboard surface off the screen with it —
 * there is no live card left to hold a tab stop — so without this the reader
 * who worked the whole queue from the keyboard is left with focus on the
 * document body and the one press that matters unreachable except by tabbing
 * back in from the top.
 *
 * Beside the tray rather than in the deck, because it is a fact about that
 * control: the tray is what appears when the stack empties, and this is where
 * it takes the reader.
 */
export function useCommitTakesFocus(
  emptied: boolean,
  commitRef: RefObject<HTMLButtonElement | null>,
) {
  useEffect(() => {
    if (emptied) {
      commitRef.current?.focus();
    }
  }, [emptied, commitRef]);
}

/**
 * WHAT THE DECK IS SHOWING: the read's own state, the earned plate, or the
 * queue in whichever form the reader chose.
 *
 * `shown` arrives already built, because which of the two forms it is belongs
 * to the deck's own state and the plate does not: a cleared queue is the one
 * reading this tier can make on its own, from a tally of what it watched leave.
 */
export function DeckQueue({
  state,
  stateDetail,
  loadingLabel,
  labels,
  tally,
  shown,
}: Readonly<{
  state: SectionState;
  stateDetail?: SectionDetail;
  loadingLabel: string;
  labels: DecisionDeckLabels;
  /** What was decided and when, or null while there is still a queue. */
  tally: { count: number; at: number } | null;
  /** The stack or the list, whichever the reader is working in. */
  shown: ReactNode;
}>) {
  return (
    <SurfaceState
      state={state}
      emptyLabel={labels.empty}
      loadingLabel={loadingLabel}
      detail={stateDetail}
    >
      {tally ? (
        <EmptyState title={labels.clearedTitle}>
          {/* `.empty-body` is already the measured 14px paragraph, so the
              sentence carries no size of its own. The timestamp below it does,
              because it is a quieter SECOND line rather than the sentence
              itself. */}
          <p>{labels.cleared(tally.count)}</p>
          <p className="t-caption ddeck-cleared-time">
            {labels.clearedTime(tally.at)}
          </p>
        </EmptyState>
      ) : (
        shown
      )}
    </SurfaceState>
  );
}

// The tray. A live region, because the count changing is the whole feedback a
// staged verdict gets — a reader driving this from the keyboard has to hear that
// their swipe landed somewhere.
export function StagingTray({
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
