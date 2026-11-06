import { useEffect } from "react";
import { ROUTE_CHUNKS } from "./App.screens";

// Fetches the route chunks in the background, once, after login. It runs at
// idle so it never competes with the open screen, with a deadline so a busy tab
// cannot defer it for good. A click that lands before the warm-up finishes
// suspends as it would without this hook, so the worst case is no slower.
export function useWarmRouteChunks() {
  useEffect(() => {
    if (window.matchMedia?.("(pointer: coarse)").matches) return;
    let cancelled = false;
    // One chunk in flight at a time. Asking for all of them empties the
    // browser's per-host connection pool. The record the reader just opened
    // would then queue behind two dozen files nobody asked for, and that is
    // worse than not warming at all.
    const warm = async () => {
      for (const load of ROUTE_CHUNKS) {
        if (cancelled) {
          return;
        }
        try {
          await load();
        } catch (reason: unknown) {
          // Nobody asked for this screen yet, so the reader sees nothing. The
          // navigation that does ask will suspend and reach the error boundary.
          // It is still logged, because an unfetchable chunk is a broken deploy.
          console.warn("route chunk warm-up failed", reason);
        }
      }
    };
    // Typed as always present, and absent in jsdom and in older Safari, so the
    // check is a runtime one whatever the DOM lib claims.
    if (typeof window.requestIdleCallback === "function") {
      const handle = window.requestIdleCallback(() => void warm(), {
        timeout: 500,
      });
      return () => {
        cancelled = true;
        window.cancelIdleCallback(handle);
      };
    }
    const handle = window.setTimeout(() => void warm(), 0);
    return () => {
      cancelled = true;
      window.clearTimeout(handle);
    };
  }, []);
}
