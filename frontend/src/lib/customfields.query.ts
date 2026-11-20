// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { queryOptions, skipToken } from "@tanstack/react-query";
import { api } from "../api/client";
import type { CfObject } from "./customfields.logic";
import { unwrap } from "./problem";

/** The first segment of every custom-field catalog key, for a write that refreshes them all. */
export const CUSTOM_FIELDS_KEY = "custom-fields";

/**
 * One record type's custom-field catalog, active and retired fields both. The
 * admin screen refreshes this key on every change, so each reader follows a
 * rename. With no object, or `enabled` false, it asks nothing.
 */
export function customFieldsQuery(
  object: CfObject | undefined,
  enabled = true,
) {
  return queryOptions({
    queryKey: [CUSTOM_FIELDS_KEY, object] as const,
    queryFn:
      enabled && object !== undefined
        ? async () =>
            unwrap(
              await api.GET("/custom-fields", {
                params: { query: { object } },
              }),
            )
        : skipToken,
  });
}
