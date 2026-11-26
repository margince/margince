// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { skipToken, useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { Project } from "./projects.form";

/**
 * One project, whole — for the callers that need more of it than its name.
 *
 * `useEntityName` answers the question every EntityRef asks ("what do I call
 * this id?") and caches a bare string under a key they all share. The composer
 * needs the project KEY as well, to put in a subject line, and widening that
 * cache entry would change what every other reference holds. So this is a
 * second reader with its own key rather than a wider first one.
 *
 * Undefined id means no read: a caller with nothing to look up asks nothing.
 */
export function useProjectRecord(
  id?: string,
  enabled = true,
): {
  project: Project | null;
  settled: boolean;
} {
  const query = useQuery({
    // Under the ["project", id] prefix the edit and archive paths already
    // invalidate, so a renamed or archived project reaches this reader too. A
    // key outside that prefix would keep serving the old record for its whole
    // freshness window.
    queryKey: ["project", id, "record"],
    queryFn: id
      ? async ({ signal }) => {
          const { data, error } = await api.GET("/projects/{id}", {
            signal,
            params: { path: { id } },
          });
          // A project the reader may not open is not an error to shout about
          // here. The caller draws nothing either way, and a throw would take
          // the composer down over a record it only wanted to name.
          return error ? null : data;
        }
      : skipToken,
    enabled,
    staleTime: 60_000,
  });
  return {
    project: query.data ?? null,
    // Whether the read has finished, however it finished. A caller that states
    // a filing must not state it while the answer is still on its way.
    settled: Boolean(id) && !query.isPending,
  };
}

/** Where a send files, and what the composer should say about it. */
export type Filing =
  /** Nothing to say: no project is in reach. */
  | { kind: "none" }
  /**
   * One project, derived rather than chosen — the thread's own, or the deal's
   * when the thread carries none. The composer STATES it and offers to opt out.
   */
  | { kind: "derived"; projectId: string; from: "thread" | "deal" }
  /** Several in reach and none derivable: the rep is asked. */
  | { kind: "choose" };

/**
 * Which project a send files under, before the rep touches anything.
 *
 * The order is the one the capture ladder already uses on the way in, so a
 * message this composer sends lands where a message it captured would: the
 * thread's own filing wins, because a conversation is about one body of work
 * and a sibling already settled it. A deal's project is the fallback for a
 * thread that predates it — the common case for a deal whose project was
 * attached after the conversation started.
 *
 * `choose` is reserved for genuine ambiguity: several projects in reach and
 * nothing naming one of them. A single reachable project is not ambiguous, and
 * asking about it would be a question with one answer.
 */
export function filingFor(input: {
  threadProjectId?: string | null;
  dealProjectId?: string | null;
  reachable: readonly { id: string }[];
}): Filing {
  if (input.threadProjectId) {
    return {
      kind: "derived",
      projectId: input.threadProjectId,
      from: "thread",
    };
  }
  if (input.dealProjectId) {
    return { kind: "derived", projectId: input.dealProjectId, from: "deal" };
  }
  return input.reachable.length > 1 ? { kind: "choose" } : { kind: "none" };
}
