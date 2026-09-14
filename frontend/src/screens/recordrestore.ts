// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Putting one recorded change back, spelled ONCE for the two surfaces that
// offer it.
//
// The record's own history panel offers it beside an entry, and the worklist's
// receipt offers it for a correction nobody was asked about. Both press the
// same route with the same If-Match rule, and a second copy of that call is how
// the two would drift — one of them growing a retry, a header or an error
// reading the other does not have.
//
// What they do AFTERWARDS is deliberately not here. The history panel
// invalidates the record's own queries and the receipt invalidates the
// worklist's, and folding both into one hook would make each surface refetch
// the other's data on every press.

import { useMutation } from "@tanstack/react-query";
import { api } from "../api/client";
import { ifMatch } from "../api/version";
import type { EntityKind } from "../app/entity";
import { throwProblem } from "./common";

// The variables one press carries. A mutationFn takes what it needs rather
// than closing over render state: the click belongs to the committed render,
// so what it passes cannot be older than the control that carried it.
export type RestorePress = Readonly<{
  kind: EntityKind;
  id: string;
  auditId: string;
  version: number;
}>;

// useRecordRestore presses the restore route and reports what came back.
//
// `If-Match` is REQUIRED by this route rather than optional, so the version
// travels in the press. A caller holding no version must not offer the control
// at all: last-write-wins is not a choice an undo may make on somebody else's
// later edit.
export function useRecordRestore(handlers: {
  onSuccess: () => void;
  onError: (error: unknown) => void;
}) {
  return useMutation({
    mutationFn: async ({ kind, id, auditId, version }: RestorePress) => {
      const { data, error } = await api.POST(
        "/records/{entity_type}/{id}/history/{audit_id}/restore",
        {
          params: {
            path: { entity_type: kind, id, audit_id: auditId },
            ...ifMatch(version),
          },
        },
      );
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: handlers.onSuccess,
    onError: handlers.onError,
  });
}
