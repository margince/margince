// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { QueryKey } from "@tanstack/react-query";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { useInstallationSettings } from "./uploadlimit";

/**
 * Whether a cached answer could be carrying a colleague's own figures.
 *
 * Generous, because it decides what to drop when the installation turns
 * individual performance off, and an admin does that rarely. Dropping too much
 * costs one refetch of a screen somebody is looking at. Dropping too little
 * leaves a colleague's figures on that screen.
 */
export function mayCarryIndividualFigures(key: QueryKey): boolean {
  const head = key[0];
  return (
    typeof head === "string" &&
    (head.startsWith("report") ||
      head.startsWith("forecast") ||
      head.startsWith("analytics"))
  );
}

/**
 * Drops the figures this session already holds when the installation stops
 * showing individual performance.
 *
 * The server refuses every new request as soon as the switch flips. What is
 * left is what a session already fetched, and a screen nobody touches holds it
 * until something asks again. So the settings read this watches comes back when
 * the reader does.
 *
 * It fires on the change, never on the first answer. A session opening with the
 * switch already off has nothing to drop.
 */
export function useFiguresFollowTheSwitch(enabled = true): void {
  const client = useQueryClient();
  const shown = useInstallationSettings(enabled).data?.individual_performance;
  // Undefined is "not answered yet", and also a server that does not serve the
  // field. Neither is a change, so the first defined answer only arms this.
  const previous = useRef<boolean | undefined>(undefined);
  useEffect(() => {
    const was = previous.current;
    if (shown !== undefined) {
      previous.current = shown;
    }
    if (was === true && shown === false) {
      void client.invalidateQueries({
        predicate: (query) => mayCarryIndividualFigures(query.queryKey),
      });
    }
  }, [client, shown]);
}
