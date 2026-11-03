// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch } from "../api/version";
import { postMultipart } from "./multipart";
import { unwrap } from "./problem";

// The legal entities the installation trades as. The server reads the
// default entity's name and logo from the anchor company. So this list is where
// a screen learns what each entity is called.

export type LegalEntity = components["schemas"]["LegalEntity"];
export type CreateLegalEntity =
  components["schemas"]["CreateLegalEntityRequest"];
export type UpdateLegalEntity =
  components["schemas"]["UpdateLegalEntityRequest"];

export const LEGAL_ENTITIES_KEY = ["legal-entities"] as const;

export function useLegalEntities(enabled = true) {
  return useQuery({
    queryKey: LEGAL_ENTITIES_KEY,
    queryFn: async () => unwrap(await api.GET("/legal-entities")).data,
    enabled,
  });
}

function useInvalidateEntities() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: LEGAL_ENTITIES_KEY });
}

export function useCreateLegalEntity() {
  const invalidate = useInvalidateEntities();
  return useMutation({
    mutationFn: async (body: CreateLegalEntity) =>
      unwrap(await api.POST("/legal-entities", { body })),
    onSuccess: invalidate,
  });
}

export function useUpdateLegalEntity() {
  const invalidate = useInvalidateEntities();
  return useMutation({
    mutationFn: async (
      input: Readonly<{ id: string; version: number; body: UpdateLegalEntity }>,
    ) =>
      unwrap(
        await api.PATCH("/legal-entities/{id}", {
          params: { path: { id: input.id }, ...ifMatch(input.version) },
          body: input.body,
        }),
      ),
    onSuccess: invalidate,
  });
}

export function useArchiveLegalEntity() {
  const invalidate = useInvalidateEntities();
  return useMutation({
    mutationFn: async (entity: Readonly<{ id: string; version: number }>) =>
      unwrap(
        await api.POST("/legal-entities/{id}/archive", {
          params: { path: { id: entity.id }, ...ifMatch(entity.version) },
        }),
      ),
    onSuccess: invalidate,
  });
}

// A logo write answers the entity. Only its version and logo are read back:
// the editor's next save must name the new version, and its preview the new
// picture.
export type LogoWritten = Readonly<{
  version: number;
  logoUrl: string | null;
}>;

export function useLegalEntityLogo() {
  const invalidate = useInvalidateEntities();
  return useMutation({
    mutationFn: async (
      input: Readonly<{ id: string; version: number; file: File | null }>,
    ): Promise<LogoWritten | null> => {
      if (input.file === null) {
        const removed = unwrap(
          await api.DELETE("/legal-entities/{id}/logo", {
            params: { path: { id: input.id }, ...ifMatch(input.version) },
          }),
        );
        return { version: removed.version, logoUrl: null };
      }
      const payload = await postMultipart(
        `/v1/legal-entities/${encodeURIComponent(input.id)}/logo`,
        { file: input.file },
        ifMatch(input.version).header,
      );
      return logoWrittenOf(payload);
    },
    onSuccess: invalidate,
  });
}

export type LogoWrite = ReturnType<typeof useLegalEntityLogo>;

// The multipart answer arrives untyped; a body without a version is treated as
// unknown, and the list refetch brings the entity back.
function logoWrittenOf(payload: unknown): LogoWritten | null {
  if (typeof payload !== "object" || payload === null) return null;
  const version: unknown = Reflect.get(payload, "version");
  const logoUrl: unknown = Reflect.get(payload, "logo_url");
  if (typeof version !== "number") return null;
  return { version, logoUrl: typeof logoUrl === "string" ? logoUrl : null };
}

const membersKey = (entityId: string) =>
  [...LEGAL_ENTITIES_KEY, entityId, "members"] as const;

/**
 * Which entity each user works for, by user id. A user no entity lists works
 * for the default, so only the further entities are asked.
 */
export function useEntityOfUser(entities: readonly LegalEntity[]) {
  const further = entities.filter((entity) => !entity.is_default);
  const lists = useQueries({
    queries: further.map((entity) => ({
      queryKey: membersKey(entity.id),
      queryFn: async () =>
        unwrap(
          await api.GET("/legal-entities/{id}/members", {
            params: { path: { id: entity.id } },
          }),
        ).data,
    })),
  });
  const byUser = new Map<string, string>();
  for (const list of lists) {
    for (const member of list.data ?? []) {
      byUser.set(member.user_id, member.legal_entity_id);
    }
  }
  return {
    byUser,
    isPending: lists.some((list) => list.isPending),
    isError: lists.some((list) => list.isError),
  };
}

export function useAssignLegalEntity() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: Readonly<{ entityId: string; userId: string }>) =>
      unwrap(
        await api.PUT("/legal-entities/{id}/members", {
          params: { path: { id: input.entityId } },
          body: { user_id: input.userId },
        }),
      ),
    // A move leaves one list and joins another, and member_count changes on
    // both, so every entity read is stale.
    onSuccess: () => qc.invalidateQueries({ queryKey: LEGAL_ENTITIES_KEY }),
  });
}
