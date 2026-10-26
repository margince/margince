// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useRef } from "react";
import type { SaveAsk } from "./composesaveddraft";

// How long the words rest before the draft keeps them.
const AUTOSAVE_MS = 1000;

/**
 * Keeps the draft once the reader pauses: while `when` holds, each change to
 * `key` restarts a short wait, and the save then writes what is on screen.
 *
 * The ask is read when the wait ends, not when it starts, so the write
 * carries the newest words and the newest version.
 */
export function useAutosave(
  when: boolean,
  key: string,
  save: (ask: SaveAsk) => void,
  askOf: () => SaveAsk | null,
) {
  const latest = useRef(askOf);
  useEffect(() => {
    latest.current = askOf;
  });
  // biome-ignore lint/correctness/useExhaustiveDependencies: each change to the words restarts the pause the save waits for
  useEffect(() => {
    if (!when) return;
    const timer = setTimeout(() => {
      const ask = latest.current();
      if (ask) save(ask);
    }, AUTOSAVE_MS);
    return () => clearTimeout(timer);
  }, [when, key, save]);
}
