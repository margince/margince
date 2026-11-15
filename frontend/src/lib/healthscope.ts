// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClientContext } from "@tanstack/react-query";
import { useContext, useSyncExternalStore } from "react";
import type { components } from "../api/schema";
import { healthScope } from "./healthscopekey";

import { useMe } from "./session";

export { healthScope } from "./healthscopekey";

function subscribeVisibility(changed: () => void) {
  document.addEventListener("visibilitychange", changed);
  return () => document.removeEventListener("visibilitychange", changed);
}

export function useHealthVisible() {
  return useSyncExternalStore(
    subscribeVisibility,
    () => document.visibilityState !== "hidden",
    () => false,
  );
}

export function useHealthScope() {
  return healthScope(useMe().data);
}

/**
 * A `placeholderData` that keeps the last answer on screen while the next one
 * loads, but never carries rows across a change of reader scope.
 */
export function previousOfScope(readerScope: string | undefined) {
  return <Data>(
    previous: Data | undefined,
    previousQuery: { queryKey: readonly unknown[] } | undefined,
  ): Data | undefined =>
    readerScope !== undefined && previousQuery?.queryKey.includes(readerScope)
      ? previous
      : undefined;
}

/**
 * The reader scope of the cached session, without asking /me and without
 * needing a QueryClient. For code that must drop what it holds when the scope
 * changes, but must not gate on a session it may never have.
 */
export function useCachedHealthScope() {
  const client = useContext(QueryClientContext);
  return useSyncExternalStore(
    (notify) =>
      client
        ? client.getQueryCache().subscribe((event) => {
            if (event.query.queryKey[0] === "me") notify();
          })
        : () => {},
    () =>
      healthScope(
        client?.getQueryData<components["schemas"]["MeResponse"]>(["me"]),
      ),
    () => undefined,
  );
}
