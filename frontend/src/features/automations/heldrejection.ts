// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useCallback, useEffect, useRef, useState } from "react";
import type { DecisionHeldRejection } from "../../design-system/decisioncard";
import { ACTION_TOAST_MS } from "../../design-system/toast";
import type { Translator } from "../../i18n";

const VERDICT_EVENT = "margince:approval-verdict";

/** `sent` lasts until the card turns decided; a refusal returns to `idle`. */
export type HeldPhase = "idle" | "held" | "sent";

export type HeldRejection = Readonly<{
  phase: HeldPhase;
  hold: () => void;
  undo: () => void;
  pause: (paused: boolean) => void;
  announce: () => void;
}>;

// One held rejection per page: it is sent when the window ends, the card
// unmounts, the page hides, or another approval card announces a verdict.
export function useHeldRejection({
  send,
  decided,
  onRefusedAfterLeaving,
}: Readonly<{
  send: () => Promise<unknown>;
  decided: boolean;
  onRefusedAfterLeaving: (error: unknown) => void;
}>): HeldRejection {
  const [phase, setPhase] = useState<HeldPhase>("idle");
  const [paused, setPaused] = useState(false);
  const owed = useRef(false);
  const left = useRef(false);
  const remaining = useRef(ACTION_TOAST_MS);
  const latest = useRef({ send, onRefusedAfterLeaving });
  useEffect(() => {
    latest.current = { send, onRefusedAfterLeaving };
  });

  const release = useCallback(() => {
    if (!owed.current) {
      return;
    }
    owed.current = false;
    setPhase("sent");
    latest.current.send().catch((error: unknown) => {
      if (left.current) {
        latest.current.onRefusedAfterLeaving(error);
      } else {
        setPhase("idle");
      }
    });
  }, []);

  useEffect(() => {
    if (decided) {
      owed.current = false;
    }
  }, [decided]);

  const holding = phase === "held" && !decided;
  useEffect(() => {
    if (!holding || paused) {
      return;
    }
    const started = Date.now();
    const timer = globalThis.setTimeout(release, remaining.current);
    return () => {
      globalThis.clearTimeout(timer);
      remaining.current -= Date.now() - started;
    };
  }, [holding, paused, release]);

  useEffect(() => {
    if (!holding) {
      return;
    }
    const onHidden = () => {
      if (document.visibilityState === "hidden") {
        release();
      }
    };
    globalThis.addEventListener("pagehide", release);
    globalThis.addEventListener(VERDICT_EVENT, release);
    document.addEventListener("visibilitychange", onHidden);
    return () => {
      globalThis.removeEventListener("pagehide", release);
      globalThis.removeEventListener(VERDICT_EVENT, release);
      document.removeEventListener("visibilitychange", onHidden);
    };
  }, [holding, release]);

  useEffect(() => {
    left.current = false;
    return () => {
      left.current = true;
      release();
    };
  }, [release]);

  const announce = () => {
    globalThis.dispatchEvent(new Event(VERDICT_EVENT));
  };
  return {
    phase,
    hold: () => {
      announce();
      owed.current = true;
      remaining.current = ACTION_TOAST_MS;
      setPaused(false);
      setPhase("held");
    },
    undo: () => {
      owed.current = false;
      setPhase("idle");
    },
    pause: setPaused,
    announce,
  };
}

export function heldRejectionBar(
  t: Translator,
  held: HeldRejection,
): DecisionHeldRejection | undefined {
  if (held.phase === "idle") {
    return undefined;
  }
  return {
    label: t("decision.status.rejected"),
    undo: t("common.undo"),
    onUndo: held.phase === "held" ? held.undo : undefined,
    onPause: held.pause,
  };
}
