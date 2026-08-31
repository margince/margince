// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useState } from "react";
import { Button } from "../design-system/atoms";
import type { MarginceCoreState } from "../design-system/margince-core";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import "./onboarding-ignition.css";

/**
 * The moment the installation acquires the ability to think, held on screen.
 *
 * WHY IT IS HELD AT ALL. Every other write in this product should get out of the
 * way, and this one should not. Before it, nothing here can read a website,
 * draft a line or run overnight; after it, all of that is true. The server
 * answers in a few hundred milliseconds and the old screen simply swapped to the
 * next question, so the one irreversible thing a first-time admin does went by
 * without being marked — and the room's light, which is the product's own signal
 * that an agent is running, snapped on while they were looking at a form.
 *
 * WHAT IT SAYS IS WHAT HAPPENED. The key is sealed, the model was reached, and
 * from here the product can think and still cannot act on its own. Three claims,
 * all true at the moment they are drawn, and the last one is the one worth
 * hearing at the moment the first two land.
 *
 * THE CHOREOGRAPHY IS CSS, the Core's states are not. Delays, the wash and the
 * beats are `animation-delay` in one stylesheet, so `prefers-reduced-motion`
 * turns the whole sequence into its end state in one place and no timer has to
 * agree with a keyframe. What CSS cannot do is tell the orb what it is doing, so
 * three timers do that and nothing else.
 */

/** How the Core moves through the sequence, in the order it happens. */
const CORE_BEATS: ReadonlyArray<{ at: number; state: MarginceCoreState }> = [
  // Reaching the vendor for the first time: material going out and coming back.
  { at: 0, state: "ingest" },
  // The answer arrived and is being made sense of.
  { at: 2100, state: "working" },
  // Bound, and waiting for the person who bound it.
  { at: 3900, state: "idle" },
];

/**
 * Drives the Core through the sequence and reports where it is.
 *
 * The ring climbs to a little under half, not to full: the model is bound and
 * first run is not finished, so a full ring here would claim the rest of the
 * setup was done too.
 */
export function useIgnitionCore(running: boolean): {
  state: MarginceCoreState;
  progress: number | undefined;
} {
  const [beat, setBeat] = useState(0);
  useEffect(() => {
    if (!running) {
      setBeat(0);
      return;
    }
    const timers = CORE_BEATS.slice(1).map((b, i) =>
      setTimeout(() => setBeat(i + 1), b.at),
    );
    return () => {
      for (const timer of timers) {
        clearTimeout(timer);
      }
    };
  }, [running]);
  if (!running) {
    return { state: "idle", progress: undefined };
  }
  return {
    state: CORE_BEATS[beat].state,
    progress: 0.12 + (beat / (CORE_BEATS.length - 1)) * 0.3,
  };
}

/**
 * What the installation can and cannot do, now that it can think.
 *
 * The refusal is last and is drawn as a refusal, because it is the one a reader
 * did not ask for and the one that makes the other two safe to have.
 */
const CAPABILITIES: ReadonlyArray<{ what: MessageKey; can: boolean }> = [
  { what: "firstRun.ignite.read", can: true },
  { what: "firstRun.ignite.draft", can: true },
  { what: "firstRun.ignite.act", can: false },
];

/**
 * The sequence itself.
 *
 * `vendor` is the label of the vendor whose key was just sealed — named on the
 * chip, because "sealed in the vault" without saying whose key is a sentence
 * about a mechanism rather than about what the reader just did.
 */
export function Ignition({
  vendor,
  onDone,
}: Readonly<{ vendor: string; onDone: () => void }>) {
  const t = useT();
  return (
    <div className="ob-ig">
      {/* The wash is the STAGE's — it comes from the orb, which is in the other
          column, and the stage is what knows where the orb is. */}
      <p className="ob-ig-sealed">{t("firstRun.ignite.sealed", { vendor })}</p>
      <p className="ob-ig-beat" data-beat="1">
        {t("firstRun.ignite.reaching")}
      </p>
      {/* One live region for the sequence, and it is the LIST: what changed is
          what the installation can and cannot do, and a screen reader hearing
          three timed lines in four seconds hears an interruption rather than a
          ceremony. The headline above is the stage's and announces itself. */}
      <ul className="ob-ig-can" role="status">
        {CAPABILITIES.map((c) => (
          <li key={c.what} data-can={c.can}>
            <b>
              {t(c.can ? "firstRun.ignite.canNow" : "firstRun.ignite.cannot")}
            </b>
            <span>{t(c.what)}</span>
          </li>
        ))}
      </ul>
      <div className="ob-ig-go">
        <Button variant="primary" onClick={onDone}>
          {t("firstRun.ignite.carryOn")}
        </Button>
      </div>
    </div>
  );
}
