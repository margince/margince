// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The list surface's reads and writes over /lists, in one place so every
// screen that shows a list — the library, the list page, a record's "Add to
// Shortlist" — reads the same cache entries and invalidates them the same way.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem, useMe } from "./common";

export type List = components["schemas"]["List"];
export type ListRecordType = List["entity_type"];
export type ListHistoryEntry = components["schemas"]["ListHistoryEntry"];
export type ListExplanation = components["schemas"]["ListMemberExplanation"];
export type ListClauseVerdict = components["schemas"]["ListClauseVerdict"];

/** Every cache entry a list write can make stale starts with this key. */
export const LISTS_KEY = "lists";

/**
 * Whether this installation has lists switched on. The server answers 404 on
 * every list route while it has not, so a surface that offered one would only
 * lead to a refusal; an unreadable /me fails closed the same way.
 */
export function useListsAvailable(): boolean {
  return useMe().data?.settings_availability?.lists === true;
}

export type ListQuery = Readonly<{
  entityType?: ListRecordType;
  listType?: List["list_type"];
  q?: string;
}>;

export function useLists(query: ListQuery, enabled = true) {
  return useQuery({
    queryKey: [LISTS_KEY, "all", query],
    enabled,
    queryFn: async () => {
      const { data, error } = await api.GET("/lists", {
        params: {
          query: {
            entity_type: query.entityType,
            list_type: query.listType,
            q: query.q || undefined,
          },
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

export function useList(id: string) {
  return useQuery({
    queryKey: [LISTS_KEY, "one", id],
    queryFn: async () => {
      const { data, error } = await api.GET("/lists/{id}", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

export function useListHistory(id: string) {
  return useQuery({
    queryKey: [LISTS_KEY, "history", id],
    queryFn: async () => {
      const { data, error } = await api.GET("/lists/{id}/history", {
        params: { path: { id }, query: { limit: 50 } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

export function useExplanation(listId: string, recordId: string | null) {
  return useQuery({
    queryKey: [LISTS_KEY, "why", listId, recordId],
    enabled: recordId !== null,
    queryFn: async () => {
      const { data, error } = await api.GET(
        "/lists/{id}/members/{recordId}/why",
        { params: { path: { id: listId, recordId: recordId ?? "" } } },
      );
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

/** Every list read goes stale together: a change to one list can move a
 * count, a member page, a why and the library row that shows it. */
function useInvalidateLists() {
  const client = useQueryClient();
  return () =>
    Promise.all([
      client.invalidateQueries({ queryKey: [LISTS_KEY] }),
      client.invalidateQueries({ queryKey: ["company360"] }),
    ]);
}

export type NewList = Readonly<{
  name: string;
  entityType: ListRecordType;
  listType: List["list_type"];
  definition?: Record<string, unknown>;
  purpose?: string;
  sharing?: List["sharing"];
}>;

export function useCreateList() {
  const invalidate = useInvalidateLists();
  return useMutation({
    mutationFn: async (input: NewList) => {
      const { data, error } = await api.POST("/lists", {
        body: {
          name: input.name,
          entity_type: input.entityType,
          list_type: input.listType,
          definition: input.definition,
          purpose: input.purpose || undefined,
          sharing: input.sharing ?? "team",
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: invalidate,
  });
}

export type ListEdit = Readonly<{
  id: string;
  version: number;
  name?: string;
  purpose?: string | null;
  sharing?: List["sharing"];
  stewardId?: string;
}>;

export function useUpdateList() {
  const invalidate = useInvalidateLists();
  return useMutation({
    mutationFn: async (edit: ListEdit) => {
      const { data, error } = await api.PATCH("/lists/{id}", {
        params: { path: { id: edit.id } },
        body: {
          version: edit.version,
          name: edit.name,
          purpose: edit.purpose,
          sharing: edit.sharing,
          steward_id: edit.stewardId,
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: invalidate,
  });
}

export function useArchiveList() {
  const invalidate = useInvalidateLists();
  return useMutation({
    mutationFn: async (input: Readonly<{ id: string; archive: boolean }>) => {
      const { data, error } = input.archive
        ? await api.DELETE("/lists/{id}", {
            params: { path: { id: input.id } },
          })
        : await api.POST("/lists/{id}/restore", {
            params: { path: { id: input.id } },
          });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: invalidate,
  });
}

export type MemberChange = Readonly<{
  listId: string;
  entityType: ListRecordType;
  entityId: string;
  note?: string;
}>;

export function useChangeMember() {
  const invalidate = useInvalidateLists();
  return useMutation({
    mutationFn: async (
      input: MemberChange & Readonly<{ remove?: boolean }>,
    ) => {
      const body = {
        entity_type: input.entityType,
        entity_id: input.entityId,
        note: input.note || undefined,
      };
      const params = { path: { id: input.listId } };
      const { error } = input.remove
        ? await api.POST("/lists/{id}/members/remove", { params, body })
        : await api.POST("/lists/{id}/members", { params, body });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: invalidate,
  });
}
