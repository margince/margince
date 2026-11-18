import { useCallback, useRef } from "react";

// Lets one press through until it settles. The flag is set in the press itself,
// so a second click in the same frame, before any pending state renders, is
// refused. `start` receives the call that releases the flag again.
export function useSingleFlight(): (
  start: (settled: () => void) => void,
) => void {
  const busy = useRef(false);
  return useCallback((start) => {
    if (busy.current) return;
    busy.current = true;
    start(() => {
      busy.current = false;
    });
  }, []);
}
