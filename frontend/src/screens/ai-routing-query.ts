// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

// The stored routing document and the ETag it was read at. One read, shared by
// every card that shows a binding and every editor that writes one.

export type RoutingRead = {
  routing: components["schemas"]["AiRouting"];
  version: string;
};

export const ROUTING_KEY = ["ai-routing"] as const;

export async function fetchRouting(): Promise<RoutingRead> {
  const { data, error, response } = await api.GET("/ai/routing");
  if (error || !response.ok) {
    throwProblem(error);
  }
  if (!data) throw new Error("AI routing unavailable");
  return { routing: data, version: response.headers.get("ETag") ?? "" };
}

export function useRouting(enabled: boolean) {
  return useQuery({ enabled, queryKey: ROUTING_KEY, queryFn: fetchRouting });
}
