// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";

// A refused dial in the address is named, and the list shows no rows. Dropping
// the dial would show wider rows under an address that still narrows.

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
