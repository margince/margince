// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { unwrap } from "../../lib/problem";

// The legal entities the installation trades as. The server reads the
// default entity's name and logo from the anchor company. So this list is where
// a screen learns what each entity is called.

export type LegalEntity = components["schemas"]["LegalEntity"];
export type CreateLegalEntity =
  components["schemas"]["CreateLegalEntityRequest"];

export const LEGAL_ENTITIES_KEY = ["legal-entities"] as const;

export function useLegalEntities() {
  return useQuery({
    queryKey: LEGAL_ENTITIES_KEY,
    queryFn: async () => unwrap(await api.GET("/legal-entities")).data,
  });
}

export function useCreateLegalEntity() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: CreateLegalEntity) =>
      unwrap(await api.POST("/legal-entities", { body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: LEGAL_ENTITIES_KEY }),
  });
}
