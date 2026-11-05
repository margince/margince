// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { unwrap } from "./problem";

// The extension units this installation composed, as the Extensions page and
// the role editor read them. Gated on `extension_access:read` server-side.

export type ExtensionUnit = components["schemas"]["ComposedExtension"];

export const EXTENSIONS_KEY = ["extension-access", "extensions"] as const;

export type ExtensionSettings = components["schemas"]["ExtensionSettings"];

const settingsKey = (unit: string) =>
  ["extension-access", "settings", unit] as const;

export function useExtensionSettings(unit: string, enabled: boolean) {
  return useQuery({
    queryKey: settingsKey(unit),
    enabled,
    queryFn: async () =>
      unwrap(
        await api.GET("/extensions/{name}/settings", {
          params: { path: { name: unit } },
        }),
      ),
  });
}

export function useUpdateExtensionSettings(unit: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: ExtensionSettings) =>
      unwrap(
        await api.PUT("/extensions/{name}/settings", {
          params: { path: { name: unit } },
          body,
        }),
      ),
    onSuccess: (saved) => queryClient.setQueryData(settingsKey(unit), saved),
  });
}

export function useExtensions(enabled: boolean) {
  return useQuery({
    queryKey: EXTENSIONS_KEY,
    enabled,
    queryFn: async (): Promise<readonly ExtensionUnit[]> => {
      return unwrap(await api.GET("/extensions")).extensions;
    },
  });
}
