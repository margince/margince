// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api, FIRST_PAGE } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";
import { worklistKey } from "./worklist.queries";

// The notification query-hook family: the centre's own reads and writes, in one
// file because the settings page and the topbar panel are two views of one
// subject and share the cache keys below.

export type NotificationClass = components["schemas"]["NotificationClass"];
export type NotificationDelivery =
  components["schemas"]["NotificationDelivery"];
export type NotificationPreference =
  components["schemas"]["NotificationPreference"];
export type NotificationItem = components["schemas"]["NotificationItem"];

/**
 * The cache key for the centre itself.
 *
 * One key for the whole walk, not one per page: the unread count rides on every
 * page and the badge reads the FIRST one, so a reader who has paged back
 * through their history still sees a count about their whole unread set.
 */
export const NOTIFICATIONS_KEY = ["notifications"] as const;

/**
 * The cache key for this seat's whole preference set.
 *
 * One key for the SET rather than one per class, because the server answers the
 * whole set to every write: a per-class key would leave five entries that the
 * answer to the sixth had already superseded.
 */
const PREFERENCES_KEY = ["notification-preferences"] as const;

/**
 * The reader's own notification centre, newest first, a page at a time.
 *
 * REFETCH ON FOCUS AND NOTHING ELSE. It overrides the app-wide default
 * (main.tsx turns focus refetching off) because notices arrive from flows the
 * reader is not watching — an automation fails, a lead ages out, a colleague
 * sends a nudge — so the count in the chrome goes stale on its own rather than
 * in response to anything this tab did. Coming back to the tab is exactly the
 * moment it has to be true again, and it costs one small request.
 *
 * There is deliberately NO background interval. A poll would ask on behalf of a
 * reader who is looking at another window, all day, for a badge whose only job
 * is to be right when somebody looks at it; and the cost is paid by every open
 * tab in the installation at once. The same trade the approvals feed makes.
 *
 * Keyset, never an offset: notices arrive while somebody is reading, and an
 * offset page would show them a line twice for every one that landed above it.
 */
export function useNotifications() {
  return useInfiniteQuery({
    queryKey: NOTIFICATIONS_KEY,
    refetchOnWindowFocus: true,
    initialPageParam: FIRST_PAGE,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await api.GET("/notices", {
        params: { query: { cursor: pageParam ?? undefined } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    // An ABSENT cursor is the last page. The endpoint declines to mint one
    // there on purpose, so a walk that treated "no cursor" as "ask again" would
    // never stop.
    getNextPageParam: (last) => last.next_cursor ?? null,
  });
}

/**
 * Settle everything this reader could see.
 *
 * The answer is 204 with NO BODY, and that is the contract doing the screen a
 * favour: the server also clears notices the centre never shows — a reader's
 * own stage-move notices among them — so its internal count is larger than
 * anything the reader was looking at. No number reaches here to be quoted at
 * them, and the screen re-asks instead of keeping its own arithmetic.
 *
 * The Worklist's notices lane counts the same rows, so it is invalidated
 * beside the centre: settling here and leaving that lane carrying the old
 * figure is one product disagreeing with itself on two screens.
 */
export function useMarkAllNoticesRead() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      const { error } = await api.POST("/notices/read-all");
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: NOTIFICATIONS_KEY });
      queryClient.invalidateQueries({ queryKey: worklistKey });
    },
  });
}

export function useNotificationPreferences() {
  return useQuery({
    queryKey: PREFERENCES_KEY,
    queryFn: async () => {
      const { data, error } = await api.GET("/me/notification-preferences");
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

export function useSaveNotificationPreference() {
  const queryClient = useQueryClient();
  return useMutation({
    // One scope for every row, so two dropdowns changed in quick succession run
    // one after the other rather than racing. Each answer carries the WHOLE set,
    // so an older reply landing last would put the cache back to before the
    // newer choice — the screen would show a class as muted while the server
    // still sends it, which for this setting is the lie that matters.
    scope: { id: "notification-preferences" },
    // Class and delivery travel together as one variable, never read off a
    // render: the row that carries the dropdown is the row that names the class,
    // and a handler reaching back for either could act on the previous render's.
    //
    // Typed to the contract's own enums rather than to `string`, so a delivery
    // the endpoint does not accept cannot reach the wire from here at all.
    mutationFn: async (choice: {
      class: NotificationClass;
      delivery: NotificationDelivery;
    }) => {
      const { data, error } = await api.PUT("/me/notification-preferences", {
        body: choice,
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    // The server answers with the whole set, so the cache takes its word rather
    // than patching one row locally: `chosen` flips on the row that was written
    // and a hand-patched row would have to re-derive that flag from this side.
    onSuccess: (data) => {
      queryClient.setQueryData(PREFERENCES_KEY, data);
    },
  });
}
