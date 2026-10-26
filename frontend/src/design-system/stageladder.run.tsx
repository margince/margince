// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronLeft, ChevronRight } from "lucide-react";
import { type ReactNode, type RefObject, useLayoutEffect, useRef } from "react";
import { useT } from "../i18n";
import { useScrollRegion } from "./atoms";
import { IconAction } from "./iconaction";
import { usePrefersReducedMotion } from "./motion";
import { type ClippedEdges, useClippedEdges } from "./scrollregion";
import type { StageStep } from "./stageladder";

/**
 * The scrolling row of open stages that both shapes of `StageLadder` draw: it
 * comes to rest on the current stage, and a clipped end fades and carries a
 * button that pages the row that way. `children` is the list of stages.
 */
export function LadderRun({
  label,
  steps,
  children,
}: Readonly<{
  label: string;
  steps: readonly StageStep[];
  children: ReactNode;
}>) {
  const runRef = useRef<HTMLDivElement>(null);
  // A refused ladder holds no rung that takes focus, so a run past its edge
  // needs its own keyboard stop; the hook gives one only while it overflows.
  const region = useScrollRegion(runRef, label);
  const settleOn = restingStage(steps);
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
    <RunPager runRef={runRef} edges={edges}>
      {/* The scroller is a box of its own because the region it may
          become would otherwise replace the list's role. */}
      <div className={runClass(edges)} ref={runRef} {...region}>
        {children}
      </div>
    </RunPager>
  );
}

// The rung the run centres on: where the record stands, or, for a record that
// has taken a way out, the last stage it climbed. A record the pipeline cannot
// place leaves the run at its start.
function restingStage(steps: readonly StageStep[]): number {
  const run = steps.filter((step) => !step.terminal);
  const here = run.findIndex((step) => step.current);
  const tookWayOut = steps.some((step) => step.terminal && step.current);
  return here < 0 && tookWayOut ? run.length - 1 : here;
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
