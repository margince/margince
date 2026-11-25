// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { UseQueryResult } from "@tanstack/react-query";

/**
 * lastReadFailed is whether the newest answer to a read was a failure. It stays
 * true while a retry runs, when `isError` is false, so a failure and its Retry
 * do not vanish while the read is asked again.
 */
export function lastReadFailed(
  query: Pick<UseQueryResult, "errorUpdatedAt" | "dataUpdatedAt">,
): boolean {
  return query.errorUpdatedAt > query.dataUpdatedAt;
}
