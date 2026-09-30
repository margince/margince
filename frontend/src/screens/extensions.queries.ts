// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

// The extension units this installation composed, as the Extensions page and
// the role editor read them. Gated on `extension_access:read` server-side.

export type ExtensionUnit = components["schemas"]["ComposedExtension"];

export const EXTENSIONS_KEY = ["extension-access", "extensions"] as const;

// `extensions` is REQUIRED in its envelope, so the `?? []` narrows `data`,
// which openapi-fetch types as possibly-undefined on the error branch that
// `throwProblem` has already left.
export function useExtensions(enabled: boolean) {
  return useQuery({
    queryKey: EXTENSIONS_KEY,
    enabled,
    queryFn: async (): Promise<readonly ExtensionUnit[]> => {
      const { data, error } = await api.GET("/extensions");
      if (error) {
        throwProblem(error);
      }
      return data?.extensions ?? [];
    },
  });
}
