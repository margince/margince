// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { unwrap } from "./common";

// The extension units this installation composed, as the Extensions page and
// the role editor read them. Gated on `extension_access:read` server-side.

export type ExtensionUnit = components["schemas"]["ComposedExtension"];

export const EXTENSIONS_KEY = ["extension-access", "extensions"] as const;

export function useExtensions(enabled: boolean) {
  return useQuery({
    queryKey: EXTENSIONS_KEY,
    enabled,
    queryFn: async (): Promise<readonly ExtensionUnit[]> => {
      return unwrap(await api.GET("/extensions")).extensions;
    },
  });
}
