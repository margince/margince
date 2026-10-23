// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { CSSProperties, ReactNode } from "react";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { Badge, Button } from "./atoms";
import { type ReadingTone, readingColours } from "./readingtone";
import type { StageStep } from "./stageladder";

type BarVars = CSSProperties & Record<`--${string}`, string | number>;

/**
 * The `bars` shape of `StageLadder`: where the record stands in words, the
 * move to the next stage, the ways out, then one bar per open stage. The
 * current stage stays a marker, never a button, as in the chevron shape.
 */
export function LadderBars({
  label,
  steps,
  hint,
  tone,
}: Readonly<{
  label: string;
  steps: readonly StageStep[];
  hint?: ReactNode;
  tone: ReadingTone;
}>) {
  const run = steps.filter((step) => !step.terminal);
  const exits = steps.filter((step) => step.terminal);
  const vars: BarVars = {
    "--stage-ladder-count": Math.max(run.length, 1),
    "--stage-ladder-here": readingColours(tone).mark,
  };
  return (
    <fieldset className="stage-ladder is-bars" aria-label={label} style={vars}>
      <div className="stage-ladder-summary">
        <Position run={run} />
        <ul className="stage-ladder-exits">
          {exits.map((step) => (
            <li key={step.key} className="stage-ladder-step is-terminal">
              <ExitChip step={step} tone={tone} />
            </li>
          ))}
        </ul>
      </div>
      <ol className="stage-ladder-bars">
        {run.map((step) => (
          <li
            key={step.key}
            className={
              step.done ? "stage-ladder-bar is-done" : "stage-ladder-bar"
            }
          >
            <BarStep step={step} />
          </li>
        ))}
      </ol>
      {hint && <p className="t-caption stage-ladder-hint">{hint}</p>}
    </fieldset>
  );
}

// "Stage 3 of 7 · Solution Discovery", and the move to the stage after it
// while one is open. A record past its last open stage, or one the pipeline
// cannot place, says nothing here rather than a position it does not hold.
function Position({ run }: Readonly<{ run: readonly StageStep[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const here = run.findIndex((step) => step.current);
  const current = run[here];
  if (!current) {
    return null;
  }
  const next = run[here + 1];
  return (
    <>
      <span className="stage-ladder-position t-caption">
        {t("standing.ladder.position", {
          n: formatNumber(here + 1, locale),
          total: formatNumber(run.length, locale),
        })}
      </span>
      <span className="stage-ladder-name">{current.label}</span>
      {next && (
        <Button
          variant="ghost"
          disabled={next.disabled}
          reason={next.reason}
          reasonId={next.reasonId}
          onClick={next.onPick}
        >
          {t("standing.ladder.moveTo", { stage: next.label })}
          <span aria-hidden="true">{"→"}</span>
        </Button>
      )}
    </>
  );
}

function BarStep({ step }: Readonly<{ step: StageStep }>) {
  const body = (
    <>
      <span className="stage-ladder-track-bar" aria-hidden="true" />
      <span className="stage-ladder-label">{step.label}</span>
      {step.sub ? <span className="stage-ladder-sub">{step.sub}</span> : null}
    </>
  );
  if (step.current) {
    return (
      <span className="stage-ladder-bar-step is-here" aria-current="step">
        {body}
      </span>
    );
  }
  return (
    <Button
      variant="ghost"
      className="stage-ladder-bar-step"
      data-testid={step.testId}
      disabled={step.disabled}
      reason={step.reason}
      reasonId={step.reasonId}
      onClick={step.onPick}
    >
      {body}
    </Button>
  );
}

// A way out the record has taken is a chip naming it; one it has not is a
// dashed pill that starts the move.
function ExitChip({
  step,
  tone,
}: Readonly<{ step: StageStep; tone: ReadingTone }>) {
  if (step.current) {
    return (
      <span aria-current="step">
        <Badge tone={tone === "success" ? "success" : "default"}>
          {step.label}
        </Badge>
      </span>
    );
  }
  return (
    <Button
      variant="ghost"
      className="stage-ladder-exit"
      data-testid={step.testId}
      disabled={step.disabled}
      reason={step.reason}
      reasonId={step.reasonId}
      onClick={step.onPick}
    >
      <span className="stage-ladder-exit-label">{step.label}</span>
    </Button>
  );
}
