// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type QueryKey, queryOptions } from "@tanstack/react-query";
import { api } from "../api/client";
import { timelineBaseKey } from "./activitykeys";
import type { EntityKind } from "./entity";
import { unwrap } from "./problem";

/** The read every surface that shows the record's newest unanswered item shares. */
export function latestInboundKey(
  entityType: EntityKind,
  entityId: string,
): QueryKey {
  return [...timelineBaseKey(entityType, entityId), "latest-inbound"];
}

/** The record's latest-inbound read; one request serves the item and the last exchange. */
export function latestInboundQuery(entityType: EntityKind, entityId: string) {
  return queryOptions({
    queryKey: latestInboundKey(entityType, entityId),
    queryFn: async () =>
      unwrap(
        await api.GET("/activities/latest-inbound", {
          params: { query: { entity_type: entityType, entity_id: entityId } },
        }),
      ),
  });
}
