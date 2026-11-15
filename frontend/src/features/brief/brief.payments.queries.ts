// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  focusManager,
  useInfiniteQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect } from "react";
import { api } from "../../api/client";
import { grants } from "../../lib/accessgrants";
import { healthScope } from "../../lib/healthscopekey";
import { problemFieldErrorsOf, unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";

export function useCustomerPayments(enabled: boolean) {
  const client = useQueryClient();
  const me = useMe();
  const scope = healthScope(me.data);
  const allowed =
    me.isSuccess &&
    !me.isRefetchError &&
    grants(me.data, "finance", "read") &&
    grants(me.data, "company", "read");
  const query = useInfiniteQuery({
    queryKey: ["customer-payments", scope],
    enabled: enabled && allowed && scope !== undefined,
    staleTime: 0,
    refetchOnWindowFocus: false,
    gcTime: 0,
    initialPageParam: "",
    queryFn: async ({ pageParam, signal }) =>
      unwrap(
        await api.GET("/finance/customer-payments", {
          params: { query: { cursor: pageParam || undefined, limit: 50 } },
          signal,
        }),
      ),
    getNextPageParam: (last) =>
      last.page.has_more ? (last.page.next_cursor ?? undefined) : undefined,
  });
  useEffect(
    () =>
      focusManager.subscribe((focused) => {
        if (focused && enabled && allowed && scope !== undefined) {
          void client.resetQueries({
            queryKey: ["customer-payments", scope],
            exact: true,
          });
        }
      }),
    [client, enabled, allowed, scope],
  );
  const pages = query.data?.pages;
  const latest = pages?.at(-1);
  // A cursor from a replaced accounting connection never works again, so the
  // list starts over instead of retrying it.
  const staleCursor =
    query.isFetchNextPageError &&
    problemFieldErrorsOf(query.error).some(
      (fault) =>
        fault.field === "cursor" && fault.code === "cursor_param_mismatch",
    );
  useEffect(() => {
    if (
      staleCursor ||
      (latest &&
        pages?.some((page) => page.access_scope !== latest.access_scope))
    ) {
      void client.resetQueries({
        queryKey: ["customer-payments", scope],
        exact: true,
      });
    }
  }, [staleCursor, latest, pages, client, scope]);
  return {
    query,
    allowed,
    latest,
    pages:
      pages?.filter((page) => page.access_scope === latest?.access_scope) ?? [],
  };
}
