import { useState } from "react";

export function useReportingPages() {
  const [cursors, setCursors] = useState<readonly (string | undefined)[]>([
    undefined,
  ]);
  return {
    cursor: cursors.at(-1),
    canBack: cursors.length > 1,
    back: () =>
      setCursors((current) =>
        current.length > 1 ? current.slice(0, -1) : current,
      ),
    next: (cursor: string | undefined) =>
      setCursors((current) => (cursor ? [...current, cursor] : [undefined])),
  };
}
