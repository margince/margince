// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./problem";

export type RecordLink = components["schemas"]["RecordLink"];
export type LinkType = components["schemas"]["LinkType"];
export type LinkedRecordType = components["schemas"]["LinkedRecordType"];

const linkTypesKey = ["link-types"] as const;
const recordLinksKey = (type: LinkedRecordType, id: string) =>
  ["record-links", type, id] as const;

/** Core's link types, then each extension's, with the records each may join. */
export function useLinkTypes() {
  return useQuery({
    queryKey: linkTypesKey,
    staleTime: 60_000,
    queryFn: async (): Promise<LinkType[]> => {
      const { data, error } = await api.GET("/link-types");
      if (error) {
        return throwProblem(error);
      }
      return data.data;
    },
  });
}

/** A type's label, or its raw name when no declaration is known (a retired unit). */
export function linkTypeLabel(
  types: readonly LinkType[] | undefined,
  name: string,
): string {
  return types?.find((type) => type.name === name)?.label ?? name;
}

/** The links a record starts or ends, as the server lets this reader see them. */
export function useRecordLinks(type: LinkedRecordType, id: string) {
  return useQuery({
    queryKey: recordLinksKey(type, id),
    queryFn: async (): Promise<RecordLink[]> => {
      const { data, error } = await api.GET("/record-links", {
        params: { query: { record_type: type, record_id: id } },
      });
      if (error) {
        return throwProblem(error);
      }
      return data.data;
    },
  });
}

/**
 * Both writes refresh every record's link list. A link is read from either end,
 * so the other record's page changes too. The deal list's `link_type` filter
 * reads them as well.
 */
export function useRecordLinkWrites() {
  const queryClient = useQueryClient();
  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ["record-links"] });
    void queryClient.invalidateQueries({ queryKey: ["deals"] });
  };
  const create = useMutation({
    mutationFn: async (
      body: components["schemas"]["NewRecordLink"],
    ): Promise<RecordLink> => {
      const { data, error } = await api.POST("/record-links", { body });
      if (error) {
        return throwProblem(error);
      }
      return data;
    },
    onSuccess: refresh,
  });
  const remove = useMutation({
    mutationFn: async (id: string): Promise<void> => {
      const { error } = await api.DELETE("/record-links/{id}", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: refresh,
  });
  return { create, remove };
}
