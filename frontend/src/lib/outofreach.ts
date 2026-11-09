// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { useCallback } from "react";
import { api } from "../api/client";
import { useToast } from "../design-system/toast";
import { recordWriteKeys } from "./recordwritekeys";
import { navigateReplacing } from "./router";

export type ReachableKind = "contact" | "company";

// Where archiving a record already takes the reader.
const LIST = { contact: "contacts", company: "companies" } as const;

/**
 * Whether a write can take a private record out of the reader's reach. It
 * can when the record is private to an owner who is not the reader. A share
 * can still keep it in reach, which only the server can tell.
 */
export function mayLeaveReach(
  visibility: string | null | undefined,
  ownerId: string | null | undefined,
  viewerId: string | undefined,
): boolean {
  return visibility === "owner" && ownerId !== viewerId;
}

/**
 * Runs after a write that may have taken the record out of reach. When the
 * server no longer shows it, it says so and takes the reader to the list.
 * Otherwise the page refetches and reads "could not be opened" after a
 * success. Answers whether it left.
 */
export function useLeaveIfOutOfReach(kind: ReachableKind) {
  const queryClient = useQueryClient();
  const toast = useToast();
  return useCallback(
    async (id: string, message: string): Promise<boolean> => {
      if (!(await gone(kind, id))) {
        return false;
      }
      toast.show(message);
      // Replacing, and forgetting the record's reads: Back must not return
      // the reader to a page drawn from a cache the server now refuses.
      navigateReplacing({ screen: LIST[kind] });
      for (const queryKey of recordWriteKeys(kind, id)) {
        queryClient.removeQueries({ queryKey });
      }
      await queryClient.invalidateQueries({ queryKey: [LIST[kind]] });
      return true;
    },
    [kind, queryClient, toast],
  );
}

async function gone(kind: ReachableKind, id: string): Promise<boolean> {
  const path = kind === "contact" ? "/contacts/{id}" : "/companies/{id}";
  try {
    const { response } = await api.GET(path, { params: { path: { id } } });
    return response.status === 404;
  } catch {
    // A failed check is not proof the record is gone; the page stays.
    return false;
  }
}
