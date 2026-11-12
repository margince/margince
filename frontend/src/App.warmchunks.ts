import { useEffect } from "react";
import { ROUTE_CHUNKS } from "./App.screens";

export function useWarmRouteChunks() {
  useEffect(() => {
    if (window.matchMedia?.("(pointer: coarse)").matches) return;
    let cancelled = false;
    let next = 0;
    let cancelPending: (() => void) | undefined;

    const schedule = () => {
      if (cancelled || next >= ROUTE_CHUNKS.length) return;
      // Each import can consume the idle period. Yield before the next one,
      // without a deadline that could force background work into a busy frame.
      if (typeof window.requestIdleCallback === "function") {
        const handle = window.requestIdleCallback(() => void warm());
        cancelPending = () => window.cancelIdleCallback(handle);
      } else {
        const handle = window.setTimeout(() => void warm(), 0);
        cancelPending = () => window.clearTimeout(handle);
      }
    };

    const warm = async () => {
      if (cancelled) return;
      const load = ROUTE_CHUNKS[next++];
      if (!load) return;
      try {
        await load();
      } catch (reason: unknown) {
        console.warn("route chunk warm-up failed", reason);
      }
      schedule();
    };

    schedule();
    return () => {
      cancelled = true;
      cancelPending?.();
    };
  }, []);
}
