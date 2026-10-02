// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import {
  fetchRouting,
  ROUTING_KEY,
  type RoutingRead,
} from "./ai-routing-query";
import {
  type SliceValue,
  sameSlice,
  sliceOf,
  withSlice,
} from "./ai-routing-slice";
import { problemCode, problemCodeOf, throwProblem } from "./common";

// Writing one slice of the routing document onto the latest one, and telling a
// colleague's edit to the same slice from one elsewhere.

// The slice moved under the editor between open and Save.
class SliceMoved extends Error {}

export function isConflict(error: unknown): boolean {
  return error instanceof SliceMoved || problemCodeOf(error) === "version_skew";
}

export function readLatest(
  queryClient: ReturnType<typeof useQueryClient>,
): Promise<RoutingRead> {
  return queryClient.fetchQuery({
    queryKey: ROUTING_KEY,
    queryFn: fetchRouting,
    staleTime: 0,
  });
}

// Puts one slice onto the latest document. A 409 means some write landed
// between the re-read and the PUT; it is retried once, because the re-read
// then tells a colleague's edit to ANOTHER lane — which is no conflict — from
// one to this lane, which throws SliceMoved as it would have at first.
export async function writeSlice(
  latestRead: () => Promise<RoutingRead>,
  base: SliceValue,
  edit: SliceValue,
): Promise<RoutingRead> {
  for (let attempt = 0; ; attempt++) {
    const latest = await latestRead();
    if (!sameSlice(sliceOf(latest.routing, base), base)) {
      throw new SliceMoved();
    }
    const { data, error, response } = await api.PUT("/ai/routing", {
      body: withSlice(latest.routing, edit),
      // Always sent: an absent If-Match is an unconditional overwrite.
      headers: { "If-Match": latest.version },
    });
    if (error) {
      if (attempt === 0 && problemCode(error) === "version_skew") continue;
      throwProblem(error);
    }
    if (!data) throw new Error("AI routing unavailable");
    return { routing: data, version: response.headers.get("ETag") ?? "" };
  }
}
