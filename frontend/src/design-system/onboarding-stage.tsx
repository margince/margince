// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Eyebrow } from "./eyebrow";
import { MarginceCoreScene, type MarginceCoreState } from "./margince-core";
import "./onboarding-stage.css";

/**
 * The room every onboarding question is asked in, before the workbench.
 *
 * ONE STAGE, NOT FOUR COLUMNS. First run, the gate and the payoff each had
 * their own full-viewport frame, and the three disagreed about everything a
 * reader would notice: where the Core sat, how large the headline was, whether
 * the ground carried any light. They are one moment in the product, so they get
 * one room and differ only in what stands in it.
 *
 * IT OWNS THE CORE. The Core's place on this surface is the stage's decision,
 * not each screen's, which is also what lets a screen swap what it is asking
 * without the orb moving — see the mounting note below.
 *
 * MOUNT IT ONCE PER FLOW. A stage per step means React unmounts one and mounts
 * the next when the step changes: the light snaps on instead of coming up (a
 * CSS transition does not run on a mount), the Core tears down and rebuilds its
 * WebGL context, and its float, breathe and sheen loops restart from phase 0.
 * Callers render ONE of these and change its children, which is why `title`,
 * `sub` and `coreState` are props rather than something a child supplies.
 */
/**
 * Where the reader is in a flow, and how far along it runs.
 *
 * `steps` are the flow's own stops in order, `at` the index of the one on
 * screen. An index rather than a "done" flag per stop, because a flow is walked
 * forwards: two stops both claiming to be current is a state the type should not
 * be able to express.
 *
 * Absent for a flow that does not number itself. The gate is one question and
 * inventing a counter for it would be progress copy nobody wrote.
 */
export type StageProgress = Readonly<{ steps: readonly string[]; at: number }>;

/**
 * The band across the top of the stage: where you are, and what the Core is
 * doing said in words.
 *
 * It runs the full width and stays put while the scene under it changes, which
 * is what makes it read as the frame rather than as part of the question. The
 * dashes are `aria-hidden` — the step's own name beside them is the accessible
 * statement, and a row of decorative marks announced one by one is noise.
 */
function StageBand({
  progress,
  where,
  coreStateLabel,
  lit,
}: Readonly<{
  progress?: StageProgress;
  where?: string;
  coreStateLabel?: string;
  lit: boolean;
}>) {
  return (
    <div className="ob-stage-band">
      <p className="ob-stage-who">
        {/* Setup carries no other chrome, so the mark is the one place the
            reader is told whose software they are inside. */}
        <span className="ob-stage-mark" aria-hidden="true">
          M
        </span>
        {progress === undefined ? null : (
          <span className="ob-stage-step">{progress.steps[progress.at]}</span>
        )}
        {where === undefined ? null : (
          <span className="ob-stage-where">· {where}</span>
        )}
      </p>
      {progress === undefined ? null : (
        <span className="ob-stage-dashes" aria-hidden="true">
          {progress.steps.map((step, i) => (
            <span
              key={step}
              data-at={
                i < progress.at ? "done" : i === progress.at ? "now" : "todo"
              }
            />
          ))}
        </span>
      )}
      {coreStateLabel === undefined ? null : (
        // Readable, and deliberately NOT a live region. The orb is aria-hidden
        // and WDS-CORE-4 requires its state to be stated in words: stated, not
        // announced. A second `role="status"` on the stage competes with the
        // one the screen under it already owns: it mounts first, so a query for
        // the status resolves on this frame label instead of on the sentence a
        // reader has to act on, and on the gate it is the read's own phase line
        // said a second time.
        <p className="ob-stage-corestate" data-unlit={!lit}>
          {coreStateLabel}
        </p>
      )}
    </div>
  );
}

export function OnboardingStage({
  lit,
  coreState = "idle",
  coreProgress,
  coreFeed,
  coreStateLabel,
  coreFlash,
  coreScale = "hero",
  anchor = "center",
  progress,
  where,
  hint,
  eyebrow,
  title,
  sub,
  children,
}: Readonly<{
  /**
   * Whether this installation has a model bound, which is the ONLY thing the
   * room's indigo means. It is read from the server rather than set per screen:
   * inventing a second trigger is how one colour acquires two meanings.
   */
  lit: boolean;
  coreState?: MarginceCoreState;
  /** 0..1, drawn as the Core's ring. Omit it and no ring renders. */
  coreProgress?: number;
  /** Context arriving at the Core. Off unless material really is arriving. */
  coreFeed?: boolean;
  /**
   * The Core's first light, washing the room once.
   *
   * For the one moment that earns it: an installation acquiring a model. It runs
   * from the orb's own cell outward and does not repeat, so a caller sets it
   * when that happens rather than holding it on.
   */
  coreFlash?: boolean;
  /**
   * How much room the Core takes: `hero` while it is the subject of the screen,
   * `work` once the reader has something of their own to attend to.
   *
   * It is a TRANSITION between two sizes rather than two layouts, which is the
   * whole point — the orb stays the same element in the same place and gives
   * ground as the work arrives, instead of a screen replacing another screen.
   * That only holds while the stage stays mounted across the change; see the
   * mounting note above.
   */
  coreScale?: "hero" | "work";
  /**
   * What the Core is doing, in words, for the band to carry.
   *
   * The Core is `aria-hidden` (WDS-CORE-4) and every state it shows has to be
   * stated in text by the surface around it — that is what makes it safe for
   * the orb to be this decorative. Stated, not announced: the band is the
   * room's frame, and the screen standing in it owns the live region. The stage
   * cannot compose the sentence itself: no copy lives in a primitive. A caller
   * that passes no label draws no band reading, which is honest for a screen
   * whose Core is not carrying a state anybody needs to act on.
   */
  coreStateLabel?: string;
  /**
   * `center` for a question of a fixed size; `start` for a surface that GROWS
   * while somebody reads it. A centred column re-centres on every arriving
   * block, so the line being read travels upward while it is being read.
   */
  anchor?: "center" | "start";
  /** How far through the flow this screen is. See `StageProgress`. */
  progress?: StageProgress;
  /**
   * Which part of the current step this is, beside the step's own name.
   *
   * A stop can take several screens ("Set up" holds the sign-in, the model and
   * the platform question), and the dashes cannot say that: they count stops.
   * Absent where a stop is one screen.
   */
  where?: string;
  /**
   * One line on the card's bottom edge about what the reader is looking at.
   *
   * For the thing the screen cannot say without becoming an essay: why a count
   * has no total, why two addresses are not interchangeable. Chrome, so it does
   * not arrive with the board.
   */
  hint?: string;
  /** The step's place in the flow. Absent where the flow does not number itself. */
  eyebrow?: string;
  title: string;
  sub: string;
  children: ReactNode;
}>) {
  return (
    <div className="ob-page">
      <div
        className="ob-stage"
        data-lit={lit}
        data-anchor={anchor}
        data-core={coreScale}
      >
        <div className="ob-stage-light" />
        <StageBand
          progress={progress}
          where={where}
          coreStateLabel={coreStateLabel}
          lit={lit}
        />
        <div className="ob-stage-scene">
          {/* The Core's place is the STAGE's to decide, and so is its first
              light: anchored to the orb itself rather than to a remembered
              offset into the window, which is nowhere near the ball at most
              widths and nowhere near it at all once the scene stacks. */}
          <div className="ob-stage-core" data-unlit={!lit}>
            {coreFlash ? (
              <span className="ob-stage-flash" aria-hidden="true" />
            ) : null}
            <MarginceCoreScene
              state={coreState}
              progress={coreProgress}
              feed={coreFeed}
            />
          </div>
          <div className="ob-stage-board arrive-stack">
            {eyebrow === undefined ? null : (
              <Eyebrow as="h2">{eyebrow}</Eyebrow>
            )}
            <h1 className="ob-stage-title">{title}</h1>
            <p className="ob-stage-sub">{sub}</p>
            {children}
          </div>
        </div>
        {hint === undefined ? null : (
          <div className="ob-stage-foot">
            <p className="ob-stage-hint">{hint}</p>
          </div>
        )}
      </div>
    </div>
  );
}
