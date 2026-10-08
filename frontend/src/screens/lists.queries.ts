// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The list surface's reads and writes over /lists, in one place so every
// screen that shows a list — the library, the list page, a record's "Add to
// Shortlist" — reads the same cache entries and invalidates them the same way.

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { recordListsKey } from "./activitykeys";
import { throwProblem, useMe } from "./common";

export type List = components["schemas"]["List"];
export type ListRecordType = List["entity_type"];
export type ListHistoryEntry = components["schemas"]["ListHistoryEntry"];
export type ListExplanation = components["schemas"]["ListMemberExplanation"];
export type ListClauseVerdict = components["schemas"]["ListClauseVerdict"];
export type ListVisit = components["schemas"]["ListVisit"];
export type ListMember = components["schemas"]["ListMember"];

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
  /** Archived lists too; absent is the live ones alone. */
  includeArchived?: boolean;
}>;

/**
 * The lists a query names. `keepRows` holds the rows already read on screen
 * while a changed query is answered, for a caller that re-asks under the reader.
 */
export function useLists(query: ListQuery, enabled = true, keepRows = false) {
  return useQuery({
    queryKey: [LISTS_KEY, "all", query],
    enabled,
    placeholderData: keepRows ? keepPreviousData : undefined,
    queryFn: async () => {
      const { data, error } = await api.GET("/lists", {
        params: {
          query: {
            entity_type: query.entityType,
            list_type: query.listType,
            q: query.q || undefined,
            include_archived: query.includeArchived || undefined,
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

export function useList(id: string, enabled = true) {
  return useQuery({
    enabled,
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

/**
 * Records that the reader opened a list. The server keeps counting from the
 * visit before this one while it is in progress, so the list and the library
 * are read again and still say what was new.
 */
export function useVisitList() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (id: string): Promise<ListVisit> => {
      const { data, error } = await api.POST("/lists/{id}/visit", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (_visit, id) =>
      Promise.all([
        client.invalidateQueries({ queryKey: [LISTS_KEY, "all"] }),
        client.invalidateQueries({ queryKey: [LISTS_KEY, "one", id] }),
      ]),
  });
}

/**
 * Why one record is or is not on a list. Kept under the record's own lists
 * key, so a write to the record makes an open verdict stale with them.
 */
export function useExplanation(
  listId: string,
  recordType: ListedRecordType,
  recordId: string,
) {
  return useQuery({
    queryKey: [...recordListsKey(recordType, recordId), "why", listId],
    queryFn: async () => {
      const { data, error } = await api.GET(
        "/lists/{id}/members/{recordId}/why",
        { params: { path: { id: listId, recordId } } },
      );
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

/**
 * What the list says about each of these records: a Live List member's filter
 * values, a Shortlist member's chooser, date and note. A record that is not a
 * member, or that the reader cannot see, is absent.
 */
export async function listMembersAmong(
  listId: string,
  recordIds: readonly string[],
): Promise<ReadonlyMap<string, ListMember>> {
  if (recordIds.length === 0) {
    return new Map();
  }
  const { data, error } = await api.GET("/lists/{id}/members", {
    params: { path: { id: listId }, query: { entity_id: [...recordIds] } },
  });
  if (error) {
    throwProblem(error);
  }
  return new Map(data.data.map((member) => [member.entity_id, member]));
}

/** The record types a record page offers lists on. */
export type ListedRecordType = Exclude<ListRecordType, "project">;

/** The lists one record is on that the reader may find. */
export function useRecordLists(entityType: ListedRecordType, recordId: string) {
  return useQuery({
    queryKey: recordListsKey(entityType, recordId),
    queryFn: async () => {
      const { data, error } = await api.GET(
        "/records/{entity_type}/{entity_id}/lists",
        { params: { path: { entity_type: entityType, entity_id: recordId } } },
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
  /** Only with team sharing; null or absent is the owner's teams. */
  teamId?: string | null;
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
          team_id: input.teamId ?? undefined,
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
  /** Absent leaves the team as it is; null moves it to the owner's teams. */
  teamId?: string | null;
  stewardId?: string;
  /** A Live List's new filter tree, in the stored encoding. */
  definition?: Record<string, unknown>;
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
          team_id: edit.teamId,
          steward_id: edit.stewardId,
          definition: edit.definition,
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
    mutationFn: async (input: MemberChange) => {
      const { error } = await api.POST("/lists/{id}/members", {
        params: { path: { id: input.listId } },
        body: {
          entity_type: input.entityType,
          entity_id: input.entityId,
          note: input.note || undefined,
        },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: invalidate,
  });
}

export type MemberRestore = Readonly<{
  listId: string;
  undo: components["schemas"]["RemovalUndo"];
}>;

// On the hook, not on `mutate`: an Undo runs after its row has unmounted, and
// React Query drops a `mutate` call's own callbacks once its observer is gone.
type Outcome<V> = Readonly<{
  onSuccess: (variables: V) => void;
  onError: (error: Error) => void;
}>;

/**
 * Take one record off a Shortlist. `onSuccess` gets the handle that puts it
 * back, or null from a server that answered without one.
 */
export function useRemoveMember(outcome: Outcome<MemberRestore | null>) {
  const invalidate = useInvalidateLists();
  return useMutation({
    mutationFn: async (input: MemberChange): Promise<MemberRestore | null> => {
      const { data, error } = await api.POST("/lists/{id}/members/remove", {
        params: { path: { id: input.listId } },
        body: { entity_type: input.entityType, entity_id: input.entityId },
      });
      if (error) {
        throwProblem(error);
      }
      return data ? { listId: input.listId, undo: data } : null;
    },
    onError: outcome.onError,
    onSuccess: async (restore) => {
      await invalidate();
      outcome.onSuccess(restore);
    },
  });
}

/** Put back a record this reader took off a Shortlist, as its author left it. */
export function useRestoreMember(outcome: Outcome<MemberRestore>) {
  const invalidate = useInvalidateLists();
  return useMutation({
    mutationFn: async (input: MemberRestore) => {
      const { error } = await api.POST("/lists/{id}/members/restore", {
        params: { path: { id: input.listId } },
        body: input.undo,
      });
      if (error) {
        throwProblem(error);
      }
    },
    onError: outcome.onError,
    onSuccess: async (_, input) => {
      await invalidate();
      outcome.onSuccess(input);
    },
  });
}
