// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createContext, type ReactNode, useContext, useState } from "react";
import { createPortal } from "react-dom";
import { ThemeToggle } from "../app/theme-toggle";
import { AmbientWaves } from "./ambient-waves";
import { Eyebrow } from "./eyebrow";
import { Logomark } from "./logomark";
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
 * The stage headline's id, for a board that is an answer to it.
 *
 * Exported rather than spelled twice: a screen whose title IS the question
 * labels its control group by pointing here, and a hand-typed copy on either
 * side is an `aria-labelledby` that resolves to nothing the day one of them is
 * renamed.
 */
export const STAGE_TITLE_ID = "ob-stage-title";

/**
 * The Core's element id, for a surface that has to send something TO it.
 *
 * The crawl's evidence flies into the orb, and the orb is not inside the canvas
 * that draws the evidence: it belongs to the room, several elements away. So the
 * picture measures the real element rather than being told a coordinate, which
 * is also what keeps the two agreeing while the Core changes size.
 */
export const STAGE_CORE_ID = "ob-stage-core";

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
  flow,
  progress,
  step,
  where,
  coreStateLabel,
  aside,
  lit,
}: Readonly<{
  flow: string;
  progress?: StageProgress;
  step?: string;
  where?: string;
  coreStateLabel?: string;
  aside?: ReactNode;
  lit: boolean;
}>) {
  return (
    <div className="ob-stage-band">
      <p className="ob-stage-who">
        {/* Setup carries no other chrome, so the mark is the one place the
            reader is told whose software they are inside. The real logomark,
            not a letter in a box: the shell's workspace chip and this are the
            same product saying its own name, and the stand-in that used to sit
            here was a second mark that could drift from the brand's. */}
        <span className="ob-stage-mark" aria-hidden="true">
          <Logomark size={15} />
        </span>
        {/* What the reader is doing here at all, beside whose software it is.
            The stop and the sub-step trail it muted: a masthead names the place
            first and the position second, and the position is also what the
            dashes are for. */}
        <span className="ob-stage-flow">{flow}</span>
        {progress === undefined && step === undefined ? null : (
          <span className="ob-stage-step">
            · {step ?? progress?.steps[progress.at]}
          </span>
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
      <div className="ob-stage-end">
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
        {aside}
        {/* Setup is railless: no top bar, so without this the reader meets nine
            screens in a row with no way to change a theme they can already see.
            The STAGE owns it rather than each screen passing one, because it is
            true of every onboarding screen and a per-caller prop is a rule that
            holds until the screen that forgets it. */}
        <span className="ob-stage-rule" aria-hidden="true" />
        <ThemeToggle />
      </div>
    </div>
  );
}

export function OnboardingStage({
  flow,
  lit,
  coreState = "idle",
  coreProgress,
  coreFeed,
  coreStateLabel,
  aside,
  coreFlash,
  coreScale = "hero",
  anchor = "center",
  progress,
  step,
  where,
  hint,
  actions,
  eyebrow,
  title,
  sub,
  children,
}: Readonly<{
  /**
   * What the reader is doing here: the flow's own name, beside the mark.
   *
   * REQUIRED, and a prop rather than a constant in here, for the two reasons
   * that pull opposite ways. No copy lives in a primitive, so the stage cannot
   * spell "Setup" itself and cannot translate it. But a screen that forgot it
   * would leave the masthead saying only where in a flow it does not name, so
   * the type asks for it and no caller can quietly drop it.
   */
  flow: string;
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
   * The band's right slot: what the installation is running on, and what this
   * setup has spent.
   *
   * A NODE rather than words, because it is the one thing in the band a reader
   * can open, and the runtime disclosure that does the opening already exists.
   * The stage keeps the place and stays out of the claim.
   */
  aside?: ReactNode;
  /**
   * `center` for a question of a fixed size; `start` for a surface that GROWS
   * while somebody reads it. A centred column re-centres on every arriving
   * block, so the line being read travels upward while it is being read.
   */
  anchor?: "center" | "start";
  /** How far through the flow this screen is. See `StageProgress`. */
  progress?: StageProgress;
  /**
   * This screen's own name, for a surface that knows what it is without knowing
   * the flow around it.
   *
   * The gate is the case: it is prop-driven with no router, so it can say "Read
   * the site" honestly and cannot say how many stops the passage has or which
   * one this is. Naming a flow it cannot see would be inventing one. A caller
   * that knows the whole shape passes `progress` instead and the step name comes
   * from there.
   */
  step?: string;
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
  /**
   * The way onward, on the card's bottom rail beside the hint.
   *
   * On the RAIL rather than under the board, because a board can be long or run
   * in two columns, and a Next placed at the end of one column is a Next the
   * reader has to hunt for. Absent for a screen whose only action is part of the
   * question itself, which is most of first run.
   */
  actions?: ReactNode;
  /** The step's place in the flow. Absent where the flow does not number itself. */
  eyebrow?: string;
  title: string;
  /**
   * The sentence under the title, where the title alone would leave a reader
   * guessing what the answer decides.
   *
   * Optional because some screens are their own explanation: a board that shows
   * eight cards under "Eight it will not guess at." has already said it, and a
   * line of prose between the two only pushes the work further down.
   */
  sub?: string;
  children: ReactNode;
}>) {
  // The rail's action cell, handed to whichever step is on the board so it
  // can put its own way onward there — see StageActions.
  const [actsSlot, setActsSlot] = useState<HTMLElement | null>(null);
  return (
    <div className="ob-page">
      {/* The ground the card stands on, the same one the sign-in surface has.
          Setup is the other half of arriving: the reader has just come off that
          screen and is still being introduced to the product. It sits BEHIND
          the card, which is opaque, so it is only ever seen in the margin
          around the room. */}
      <AmbientWaves />
      <div
        className="ob-stage"
        data-lit={lit}
        data-anchor={anchor}
        data-core={coreScale}
      >
        <div className="ob-stage-light" />
        <StageBand
          flow={flow}
          progress={progress}
          step={step}
          where={where}
          coreStateLabel={coreStateLabel}
          aside={aside}
          lit={lit}
        />
        <div className="ob-stage-scene">
          {/* The Core's place is the STAGE's to decide, and so is its first
              light: anchored to the orb itself rather than to a remembered
              offset into the window, which is nowhere near the ball at most
              widths and nowhere near it at all once the scene stacks. */}
          <div className="ob-stage-core" id={STAGE_CORE_ID} data-unlit={!lit}>
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
            {/* A STABLE id, because on a screen whose title is the question
                itself the board below is answering this heading, and a control
                group there labels itself by pointing at it. One stage is
                mounted at a time (see the mounting note), so the id is unique
                by construction. */}
            <h1 className="ob-stage-title" id={STAGE_TITLE_ID}>
              {title}
            </h1>
            {sub === undefined ? null : <p className="ob-stage-sub">{sub}</p>}
            <StageActionsSlot.Provider value={actsSlot}>
              {children}
            </StageActionsSlot.Provider>
          </div>
        </div>
        {/* Always in the tree, because a step fills the action cell from
            inside the board (StageActions) and a cell that is only rendered
            once something asks for it is a cell nothing can ask for. The
            stylesheet hides a foot with nothing on it. */}
        <div className="ob-stage-foot">
          {hint === undefined ? null : <p className="ob-stage-hint">{hint}</p>}
          <div className="ob-stage-acts" ref={setActsSlot}>
            {actions}
          </div>
        </div>
      </div>
    </div>
  );
}

const StageActionsSlot = createContext<HTMLElement | null>(null);

/**
 * A step's way onward, placed on the stage's bottom rail from inside the board.
 *
 * The rail is the one part of the room that stays in view while a long board
 * scrolls, so the button belongs there — but the step owns the button's state
 * and the stage renders the rail, and lifting every step's readiness into the
 * screen that mounts the stage would make one component hold every step's
 * form. A portal lets the step keep its state and still put the control where
 * the reader looks for it. A submit button rendered here still submits its
 * form through the `form` attribute; nothing else about it changes.
 *
 * Without a rail — a scene rendered on its own, in a test or on a surface that
 * is not the stage — the actions render where they are written, so a step's
 * way onward can never quietly vanish with the frame it expected. Inside the
 * stage the rail exists one commit after the stage mounts, and the actions
 * appear there on that commit.
 */
export function StageActions({ children }: Readonly<{ children: ReactNode }>) {
  const slot = useContext(StageActionsSlot);
  if (slot === null) {
    return children;
  }
  return createPortal(children, slot);
}
