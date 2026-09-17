import { useId } from "react";
import type { components } from "../api/schema";
import { StageLadder, type StageStep } from "../design-system/stageladder";
import { useT } from "../i18n";

type Deal = components["schemas"]["Deal"];
type Stage = components["schemas"]["Stage"];

// The ladder a deal climbs, drawn on its overview under the header. Where the
// deal is now is a fact rather than a choice, so the current stage stays a
// marker; every other stage is the move to it — which is what makes a deal
// closable from its own page rather than only by dragging its card on the
// board.
//
// Its own module, like the lead's ladder and the project's phase stepper
// beside it, because the three answer one question on three records and the
// deal's answer had grown a refusal of its own to resolve.

/**
 * DealStageLadder draws the pipeline and, where no move is on offer, says why.
 *
 * `advanceRefused` is where this deal cannot be moved AT ALL — archived, not
 * this caller's to write, or already closed. `advancing` is narrower and different in kind: one
 * advance at a time, because a second click while the first is in flight would
 * send a second write pinned to the same version and the loser reads as a
 * conflict the reader never caused.
 */
export function DealStageLadder({
  deal,
  stages,
  advancing,
  advanceRefused,
  refusedReasonId,
  onAdvance,
}: Readonly<{
  deal: Deal;
  stages: readonly Stage[];
  advancing: boolean;
  advanceRefused: boolean;
  /** The id of the page's sentence about why this deal takes no changes, or
   * undefined while it does. */
  refusedReasonId?: string;
  onAdvance: (toStage: Stage) => void;
}>) {
  const t = useT();
  // WHY THE LADDER REFUSES, said once and pointed at.
  //
  // Every rung is refused for the same cause, so they name ONE element rather
  // than each drawing the same line under itself — the contract `reasonId`
  // exists for, and what the project phase stepper already does. Where the page
  // draws that line already, the rungs point at the page's own: a deal this
  // caller may not change, or one that is archived, says so in its header band.
  // A CLOSED deal has no line anywhere, so the ladder draws one as its hint and
  // names it there.
  //
  // `advancing` is deliberately not a cause here. It is this reader's own press
  // still in flight, and a sentence about it would tell them what they just
  // did — the disabled moment passes on its own.
  const ownReasonId = useId();
  const ownReason =
    advanceRefused && refusedReasonId === undefined
      ? t("deal.closedTakesNoStage")
      : undefined;
  return (
    <StageLadder
      label={t("deals.stage")}
      hint={ownReason ? <span id={ownReasonId}>{ownReason}</span> : undefined}
      steps={dealStageSteps({
        deal,
        stages,
        refused: advancing || advanceRefused,
        reasonId: advanceRefused ? (refusedReasonId ?? ownReasonId) : undefined,
        onAdvance,
      })}
    />
  );
}

// The pipeline's stages as ladder rungs: what is behind the deal, where it
// stands, and the ways out.
//
// `position` orders the pipeline and is what the trail is read from — a stage
// earlier in the pipeline than the deal's own has been passed. A deal whose
// stage the pipeline cannot name (one archived out from under it) leaves every
// rung unpassed rather than guessing a position: a trail drawn from a guess
// says the deal went through stages it may never have seen.
function dealStageSteps({
  deal,
  stages,
  refused,
  reasonId,
  onAdvance,
}: Readonly<{
  deal: Deal;
  stages: readonly Stage[];
  refused: boolean;
  /** The one element saying why no move is offered, or undefined while one is.
   * Every rung points at it rather than drawing its own copy — a ladder refused
   * for one cause says it once. */
  reasonId?: string;
  onAdvance: (toStage: Stage) => void;
}>): StageStep[] {
  const here = stages.find((stage) => stage.id === deal.stage_id);
  return stages.map((stage) => ({
    key: stage.id,
    label: stage.name,
    done: here !== undefined && stage.position < here.position,
    current: stage.id === deal.stage_id,
    // Won and lost are the two ways out rather than two more rungs.
    terminal: stage.semantic !== "open",
    disabled: refused,
    reasonId,
    onPick: () => onAdvance(stage),
  }));
}
