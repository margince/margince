// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";

// A list whose address holds a dial its reader is refused names that dial and
// shows no rows: dropping it would show wider rows under the narrowed address.

export function heldRefusedDials(
  filters: Readonly<Record<string, string>>,
  refused: ReadonlySet<string>,
): string[] {
  return Object.keys(filters).filter(
    (dial) => Boolean(filters[dial]) && refused.has(dial),
  );
}

// A refusal /me did not foresee reads /me again, so the dials the list names
// catch up with the role.
export function useReReadMeWhen(refusedUnforeseen: boolean): void {
  const queryClient = useQueryClient();
  useEffect(() => {
    if (refusedUnforeseen) {
      queryClient.invalidateQueries({ queryKey: ["me"] });
    }
  }, [refusedUnforeseen, queryClient]);
}
