// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";
import { SIGN_OFF_QUERY } from "./composesignoff";

type EmailSignature = components["schemas"]["EmailSignature"];

/**
 * Writes (or, empty, clears) the caller's own signature, and sends back to ask
 * everything that read it: the settings row, and every composer's preview of
 * the sign-off a send appends.
 */
export function useSaveSignature(
  onSaved: (saved: EmailSignature | undefined) => void,
) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (next: string) => {
      const { data, error } = await api.PUT("/me/email-signature", {
        body: { body: next },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (saved) => {
      queryClient.invalidateQueries({ queryKey: ["me-email-signature"] });
      queryClient.invalidateQueries({ queryKey: [SIGN_OFF_QUERY] });
      onSaved(saved);
    },
  });
}
