// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useId, useRef, useState } from "react";
import { ACTION_TOAST_MS } from "../../design-system/toast";

// A page event rather than a module-level slot: every listener leaves with the
// card that added it, so nothing outlives the page or a test that rendered it.
const VERDICT_EVENT = "margince:approval-verdict";

export type HeldRejection = Readonly<{
  held: boolean;
  hold: () => void;
  undo: () => void;
  announce: () => void;
}>;

// One held rejection per page: it is sent when the window ends, the card
// unmounts, the page hides, or another approval card announces a verdict.
export function useHeldRejection(send: () => void): HeldRejection {
  const self = useId();
  const [held, setHeld] = useState(false);
  const latestSend = useRef(send);
  const cancel = useRef<() => void>(() => undefined);
  useEffect(() => {
    latestSend.current = send;
  });

  useEffect(() => {
    if (!held) {
      return;
    }
    let settled = false;
    const flush = () => {
      if (!settled) {
        settled = true;
        latestSend.current();
      }
    };
    const release = () => {
      flush();
      setHeld(false);
    };
    const onVerdict = (event: Event) => {
      if (!(event instanceof CustomEvent) || event.detail !== self) {
        release();
      }
    };
    cancel.current = () => {
      settled = true;
    };
    const timer = globalThis.setTimeout(release, ACTION_TOAST_MS);
    globalThis.addEventListener("pagehide", release);
    globalThis.addEventListener(VERDICT_EVENT, onVerdict);
    return () => {
      globalThis.clearTimeout(timer);
      globalThis.removeEventListener("pagehide", release);
      globalThis.removeEventListener(VERDICT_EVENT, onVerdict);
      flush();
    };
  }, [held, self]);

  const announce = () => {
    globalThis.dispatchEvent(new CustomEvent(VERDICT_EVENT, { detail: self }));
  };
  return {
    held,
    hold: () => {
      announce();
      setHeld(true);
    },
    undo: () => {
      cancel.current();
      setHeld(false);
    },
    announce,
  };
}
