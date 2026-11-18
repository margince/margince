// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  partialMatchKey,
  type QueryClient,
  type QueryKey,
} from "@tanstack/react-query";
import { recordListsKey } from "../../lib/activitykeys";
import { leadScoreKey, leadWriteKeys } from "../../lib/leadkeys";

// Apart from leadkeys.ts, which Home loads: only the lead screens close a lead.

/**
 * Marks stale what closing one lead changed, less the two reads the server
 * refuses for an archived lead: its score breakdown and its lists. Their
 * readers still hold the live lead in the beat after the write, so a refresh
 * then would only fail. They stop asking once the closed lead arrives.
 */
export function invalidateClosedLead(client: QueryClient, id: string): void {
  const refused = [leadScoreKey(id), recordListsKey("lead", id)];
  const askable = (queryKey: QueryKey) =>
    !refused.some((key) => partialMatchKey(queryKey, key));
  for (const key of leadWriteKeys(id)) {
    client.invalidateQueries({
      queryKey: key,
      predicate: (query) => askable(query.queryKey),
    });
  }
}
