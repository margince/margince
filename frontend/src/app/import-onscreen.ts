// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useSyncExternalStore } from "react";

/**
 * Whether a surface on screen right now already draws the mail import in full.
 *
 * The capture chip (app/capture-chip.tsx) is the import's gauge for a reader who
 * is doing something else, so it stands down wherever the run is already on the
 * page: the connections card in Settings, and the backread step of onboarding.
 * The chip used to know that as a route — `#/settings/connections` spelled
 * inside it — which is the chip holding a second copy of where those panels
 * live, and it could only ever name the one place it knew about. The panels say
 * it themselves instead, so a third one is covered by mounting.
 *
 * A module-level count published on every change, the shape
 * `api/model-inflight.ts` already uses: the chip is the page's SIBLING in the
 * shell's tree rather than its ancestor, so a context provided by a panel could
 * not reach it, and there is exactly one chip in one app.
 */
let surfaces = 0;
const listeners = new Set<() => void>();

function notify(): void {
  for (const listener of listeners) {
    listener();
  }
}

function subscribe(onChange: () => void): () => void {
  listeners.add(onChange);
  return () => {
    listeners.delete(onChange);
  };
}

function importRunOnScreen(): boolean {
  return surfaces > 0;
}

/**
 * Declare that this surface draws the import run in full while `drawing` holds.
 *
 * A boolean rather than a bare mount, because both panels stay mounted for a
 * mailbox with no run at all — a window picker is not a gauge, and a chip that
 * stood down behind one would stand down behind every connected mailbox on the
 * card.
 */
export function useDrawsImportRun(drawing: boolean): void {
  useEffect(() => {
    if (!drawing) {
      return;
    }
    surfaces += 1;
    notify();
    return () => {
      surfaces -= 1;
      notify();
    };
  }, [drawing]);
}

/** Whether a surface on screen already draws the run, as a component sees it. */
export function useImportRunOnScreen(): boolean {
  return useSyncExternalStore(subscribe, importRunOnScreen, importRunOnScreen);
}
