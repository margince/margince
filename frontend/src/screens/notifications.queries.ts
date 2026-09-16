// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

// The notification query-hook family: the centre's own reads and writes, in one
// file because the settings page and the topbar panel are two views of one
// subject and share the cache keys below.

export type NotificationClass = components["schemas"]["NotificationClass"];
export type NotificationDelivery =
  components["schemas"]["NotificationDelivery"];
export type NotificationPreference =
  components["schemas"]["NotificationPreference"];

/**
 * The cache key for this seat's whole preference set.
 *
 * One key for the SET rather than one per class, because the server answers the
 * whole set to every write: a per-class key would leave five entries that the
 * answer to the sixth had already superseded.
 */
const PREFERENCES_KEY = ["notification-preferences"] as const;

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
