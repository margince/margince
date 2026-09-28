// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The ladder a record climbs: the stages in order, where it stands now, and
 * the move to any other one.
 *
 * Three surfaces drew this and no two drew it alike. The deal wrote a
 * `<fieldset>` of pills inline in its screen; the project copied that chrome
 * into `PhaseStepper` and said so in a comment; the lead built `lead-ladder`
 * as a `<nav>` with chevrons, a filled trail and a line underneath. Same
 * control, three markups and two stylesheets — so a rep who learned the ladder
 * on a lead met a different-looking one on the deal it became.
 *
 * This takes the RIGHT half of each. The lead's shape, because chevrons and a
 * filled trail say "these are in order and you are here", which a row of equal
 * pills does not — the deal showed no trail at all, so nothing on it said which
 * stages were behind. And the deal's semantics, because its comment was right
 * and the lead's markup was wrong: these buttons MOVE the record, they do not
 * take a reader anywhere, and a `<nav>` landmark that writes when you press it
 * misdescribes itself to everyone reading by landmark. It is a group of
 * controls, holding an ordered list because the order is the point.
 *
 * The current step is a MARKER, never a button: where the record stands is a
 * fact, and a control that only restates where you already are is one a reader
 * has to try before learning it does nothing.
 */

import { type ReactNode, useLayoutEffect, useRef } from "react";
import { Button, useScrollRegion } from "./atoms";
import "./stageladder.css";

export type StageStep = {
  // Stable across renders and unique in the ladder — a stage id, a phase name.
  key: string;
  label: string;
  // Behind the current one: the record has been through it. Drawn as a filled
  // trail, which is the half of "where am I" that a row of equal pills cannot
  // say.
  done?: boolean;
  // Where the record IS. Exactly one step should carry it; a ladder with none
  // draws no marker, which is what a record between stages honestly looks
  // like.
  current?: boolean;
  // A way OUT rather than one more rung — won or lost, qualified or
  // disqualified. Two of these are alternatives to each other, not a sequence,
  // so they stand off from the run of open stages.
  terminal?: boolean;
  // The move to this step. Never called for the current one, which draws no
  // control.
  onPick: () => void;
  disabled?: boolean;
  // Why this move cannot be made. Both go straight to `Button`, which owns the
  // refusal contract: `reason` is a sentence it draws under the control,
  // `reasonId` names one the page already draws so a ladder whose every step
  // is refused for one cause says it once. Either marks the step refused.
  reason?: string;
  reasonId?: string;
  testId?: string;
};

/**
 * `label` names the group: what this ladder is a ladder OF. `hint` is the one
 * line under it — how the record got where it is, what a step will ask for
 * before it happens, or why none of them can be taken. Both surfaces that had
 * one kept it; the deal never had a hint and now can.
 *
 * A refusal belongs there when the page has no sentence of its own to point at:
 * drawn once under the ladder and given an id, it is what every rung's
 * `reasonId` names — which is how a ladder refused for one cause says it once
 * rather than under each rung in turn.
 *
 * A pipeline is the workspace's to lengthen, so the open stages never wrap —
 * a wrapped ladder reads as two sentences and, on a phone, buries the record
 * under rows of pills. They hold one row that scrolls and comes to rest on
 * the current stage. The ways out stand outside the scroll: closing a deal
 * from its own page is what the rungs are for, and a scroll may not hide it.
 */
export function StageLadder({
  label,
  steps,
  hint,
}: Readonly<{
  label: string;
  steps: readonly StageStep[];
  hint?: ReactNode;
}>) {
  const run = steps.filter((step) => !step.terminal);
  const exits = steps.filter((step) => step.terminal);
  const runRef = useRef<HTMLDivElement>(null);
  // A refused ladder holds no rung that takes focus, so a run past its edge
  // needs its own keyboard stop; the hook gives one only while it overflows.
  const region = useScrollRegion(runRef, label);
  // The rung the run centres on: where the record stands, or — for a record
  // that has taken a way out — the last stage it climbed before it did. A
  // record the pipeline cannot place leaves the run at its start.
  const hereInRun = run.findIndex((step) => step.current);
  const settleOn =
    hereInRun >= 0 || !exits.some((step) => step.current)
      ? hereInRun
      : run.length - 1;
  useLayoutEffect(() => {
    const scroller = runRef.current;
    if (!scroller) {
      return;
    }
    const rung = scroller.firstElementChild?.children[settleOn];
    if (!rung) {
      scroller.scrollLeft = 0;
      return;
    }
    // Measured against the scroller's own box, so no ancestor has to be
    // positioned; the browser clamps past either end, so a ladder that fits
    // stays where it is.
    const rungBox = rung.getBoundingClientRect();
    const runBox = scroller.getBoundingClientRect();
    scroller.scrollLeft +=
      (rungBox.left + rungBox.right - runBox.left - runBox.right) / 2;
  }, [settleOn]);
  return (
    // A fieldset rather than a nav, and rather than a bare list: it IS a group
    // of controls acting on one record, and the native element says so without
    // a role. `<ol>` inside it carries the one thing the shape claims — that
    // the stages have an order. The ways out are a `<ul>`: two alternatives,
    // not two more rungs.
    <fieldset className="stage-ladder" aria-label={label}>
      <div className="stage-ladder-track">
        {/* The scroller is a box of its own because the region it may
            become would otherwise replace the list's role. */}
        <div className="stage-ladder-run" ref={runRef} {...region}>
          <ol className="stage-ladder-steps">
            {run.map((step, index) => (
              <LadderStep key={step.key} step={step} first={index === 0} />
            ))}
          </ol>
        </div>
        {exits.length > 0 && (
          <ul className="stage-ladder-exits">
            {exits.map((step, index) => (
              <LadderStep
                key={step.key}
                step={step}
                first={run.length === 0 && index === 0}
              />
            ))}
          </ul>
        )}
      </div>
      {hint && <p className="t-caption stage-ladder-hint">{hint}</p>}
    </fieldset>
  );
}

function LadderStep({
  step,
  first,
}: Readonly<{ step: StageStep; first: boolean }>) {
  return (
    <li
      className={[
        "stage-ladder-step",
        step.done ? "is-done" : "",
        step.terminal ? "is-terminal" : "",
      ]
        .filter(Boolean)
        .join(" ")}
    >
      {/* The chevron belongs to the step that FOLLOWS it and sits inside
          that step's own item, so the ways out, dropped to a line of their
          own in a narrow ladder, can shed the one that would open it. */}
      {!first && (
        <span aria-hidden="true" className="stage-ladder-sep">
          ›
        </span>
      )}
      {step.current ? (
        // "You are here" belongs to the element that IS here — the
        // marker, not the item around it. The item also holds the
        // chevron introducing it, and a decorative mark inside the
        // element making the claim puts it in the claim's own text.
        <span className="stage-ladder-here" aria-current="step">
          {step.label}
        </span>
      ) : (
        <Button
          variant="ghost"
          data-testid={step.testId}
          disabled={step.disabled}
          reason={step.reason}
          reasonId={step.reasonId}
          onClick={step.onPick}
        >
          {step.label}
        </Button>
      )}
    </li>
  );
}
