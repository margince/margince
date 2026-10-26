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

import { ChevronLeft, ChevronRight } from "lucide-react";
import { type ReactNode, type RefObject, useLayoutEffect, useRef } from "react";
import { useT } from "../i18n";
import { Button, useScrollRegion } from "./atoms";
import { IconAction } from "./iconaction";
import { usePrefersReducedMotion } from "./motion";
import type { ReadingTone } from "./readingtone";
import { type ClippedEdges, useClippedEdges } from "./scrollregion";
import { LadderBars } from "./stageladder.bars";
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
  // One line under the step in the `bars` shape: how long the record spent
  // there ("12 d", "6 d · now"). Absent draws no line, never a dash.
  sub?: ReactNode;
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
 * the current stage. A clipped end fades and carries a button that pages the
 * row that way. The ways out stand outside the scroll. Closing a deal
 * from its own page is what the rungs are for, and a scroll may not hide it.
 */
export function StageLadder({
  label,
  steps,
  hint,
  wrap,
  variant = "chevrons",
  tone = "success",
}: Readonly<{
  label: string;
  steps: readonly StageStep[];
  hint?: ReactNode;
  /**
   * Draws the open stages on as many lines as they need, for a fixed list the
   * reader has to see whole (a wizard's six steps). Without it a run past its
   * row scrolls, which is right for a pipeline the workspace may lengthen.
   */
  wrap?: boolean;
  /**
   * `bars` is the record page's ladder inside How it stands. It shows "Stage N
   * of M" and the move to the next stage over one bar per stage with its `sub`
   * line. The ways out are chips. `chevrons` is every other ladder.
   */
  variant?: "chevrons" | "bars";
  // The current bar's colour in the `bars` shape: the verdict the card beside
  // it reads, so a drifting deal does not stand on a green rung.
  tone?: ReadingTone;
}>) {
  if (variant === "bars") {
    return <LadderBars label={label} steps={steps} hint={hint} tone={tone} />;
  }
  return <ChevronLadder label={label} steps={steps} hint={hint} wrap={wrap} />;
}

function ChevronLadder({
  label,
  steps,
  hint,
  wrap,
}: Readonly<{
  label: string;
  steps: readonly StageStep[];
  hint?: ReactNode;
  wrap?: boolean;
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
  const edges = useClippedEdges(runRef);
  return (
    // A fieldset rather than a nav, and rather than a bare list: it IS a group
    // of controls acting on one record, and the native element says so without
    // a role. `<ol>` inside it carries the one thing the shape claims — that
    // the stages have an order. The ways out are a `<ul>`: two alternatives,
    // not two more rungs.
    <fieldset
      className={wrap ? "stage-ladder is-wrap" : "stage-ladder"}
      aria-label={label}
    >
      <div className="stage-ladder-track">
        <RunPager runRef={runRef} edges={edges}>
          {/* The scroller is a box of its own because the region it may
              become would otherwise replace the list's role. */}
          <div className={runClass(edges)} ref={runRef} {...region}>
            <ol className="stage-ladder-steps">
              {run.map((step, index) => (
                <LadderStep key={step.key} step={step} first={index === 0} />
              ))}
            </ol>
          </div>
        </RunPager>
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

function runClass(edges: ClippedEdges) {
  return [
    "stage-ladder-run",
    edges.start ? "is-clipped-start" : "",
    edges.end ? "is-clipped-end" : "",
  ]
    .filter(Boolean)
    .join(" ");
}

// Most of a row per press, so the last rung in view stays in view as the
// first one after it. The reader keeps their place.
const PAGE_SHARE = 0.75;

/**
 * The buttons over a clipped end of the run. Each shows only while its end
 * hides a rung. A pressed button that goes takes no focus with it: focus moves
 * to the last rung at that end, the other button, or the run.
 */
function RunPager({
  runRef,
  edges,
  children,
}: Readonly<{
  runRef: RefObject<HTMLDivElement | null>;
  edges: ClippedEdges;
  children: ReactNode;
}>) {
  const t = useT();
  const reduced = usePrefersReducedMotion();
  const pagerRef = useRef<HTMLDivElement>(null);
  const pressed = useRef<"start" | "end" | null>(null);
  useLayoutEffect(() => {
    const side = pressed.current;
    if (side === null || edges[side]) {
      return;
    }
    pressed.current = null;
    // A reader whose focus is already elsewhere keeps it there.
    const focused = document.activeElement;
    if (focused && focused !== document.body && focused.isConnected) {
      return;
    }
    const rungs = [
      ...(runRef.current?.querySelectorAll<HTMLButtonElement>("button") ?? []),
    ].filter((rung) => !rung.disabled);
    const nearest = side === "start" ? rungs[0] : rungs.at(-1);
    const other = pagerRef.current?.querySelector<HTMLElement>(
      ".stage-ladder-page button",
    );
    (nearest ?? other ?? runRef.current)?.focus();
  }, [edges, runRef]);
  const page = (side: "start" | "end") => {
    const scroller = runRef.current;
    if (!scroller) {
      return;
    }
    pressed.current = side;
    // scrollLeft grows rightward, toward the start of a right-to-left run.
    const rightward =
      (side === "end") === (getComputedStyle(scroller).direction !== "rtl");
    const step = scroller.clientWidth * PAGE_SHARE;
    scroller.scrollBy({
      left: rightward ? step : -step,
      behavior: reduced ? "auto" : "smooth",
    });
  };
  return (
    <div className="stage-ladder-pager" ref={pagerRef}>
      {edges.start && (
        <span className="stage-ladder-page is-start">
          <IconAction
            label={t("stageLadder.earlier")}
            icon={<ChevronLeft aria-hidden />}
            variant="ghost"
            onClick={() => page("start")}
          />
        </span>
      )}
      {children}
      {edges.end && (
        <span className="stage-ladder-page is-end">
          <IconAction
            label={t("stageLadder.later")}
            icon={<ChevronRight aria-hidden />}
            variant="ghost"
            onClick={() => page("end")}
          />
        </span>
      )}
    </div>
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
