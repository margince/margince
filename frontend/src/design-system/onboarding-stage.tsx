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
export function OnboardingStage({
  lit,
  coreState = "idle",
  coreProgress,
  coreFeed,
  anchor = "center",
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
   * `center` for a question of a fixed size; `start` for a surface that GROWS
   * while somebody reads it. A centred column re-centres on every arriving
   * block, so the line being read travels upward while it is being read.
   */
  anchor?: "center" | "start";
  /** The step's place in the flow. Absent where the flow does not number itself. */
  eyebrow?: string;
  title: string;
  sub: string;
  children: ReactNode;
}>) {
  return (
    <div className="ob-stage" data-lit={lit} data-anchor={anchor}>
      <div className="ob-stage-light" />
      <div className="ob-stage-core">
        <MarginceCoreScene
          state={coreState}
          progress={coreProgress}
          feed={coreFeed}
        />
      </div>
      <div className="ob-stage-column arrive-stack">
        {eyebrow === undefined ? null : <Eyebrow as="h2">{eyebrow}</Eyebrow>}
        <h1 className="ob-stage-title">{title}</h1>
        <p className="ob-stage-sub">{sub}</p>
        {children}
      </div>
    </div>
  );
}
