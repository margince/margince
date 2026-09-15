// The palette's LIVE arm: the records a query finds in the workspace, beside
// the built-in commands the palette already knows.
//
// Its own file because it is its own subject — a query, a debounce, a floor, a
// refusal that must not read as an empty workspace, and a second read that
// turns a project id into the line a reader recognises it by. None of that is
// what a command palette IS, and all of it is what `palette.tsx` was mostly
// made of.

import { useQueries, useQuery } from "@tanstack/react-query";
import { useDeferredValue } from "react";
import { api } from "../api/client";
import { useT } from "../i18n";
import type { Command } from "./palette";
import {
  SEARCH_HIT_KIND_KEY,
  type SearchHitType,
  searchHitDestination,
} from "./searchkinds";

// How long a palette search may take before the wait is worth reporting. Below
// this the answer is quicker than a keystroke and a placeholder would flash on
// every letter typed; above it, an unchanged list reads as a palette that has
// stopped listening.
export const SEARCH_PENDING_DELAY_MS = 300;

// What the live search arm has to say: the rows it found, and whether it is
// still working or gave up. The two flags are returned rather than swallowed —
// the palette used to answer a failed search with an empty array, which is the
// same shape as "no matches" and told the reader the workspace holds nothing
// when the truth was that nobody had asked it.
type SearchArm = Readonly<{
  commands: Command[];
  pending: boolean;
  failed: boolean;
}>;

// Live record hits for the palette (RS-1): debounced via useDeferredValue
// rather than a timer (craft: no real-clock waits in the render path), and
// gated on a 2-char floor so single keystrokes don't fire a query per key.
export function useSearchCommands(query: string): SearchArm {
  const t = useT();
  const deferred = useDeferredValue(query.trim());
  const enabled = deferred.length >= 2;
  const result = useQuery({
    queryKey: ["palette-search", deferred],
    enabled,
    queryFn: async () => {
      const { data, error } = await api.GET("/search", {
        params: { query: { q: deferred, limit: 5 } },
      });
      if (error) {
        // Thrown rather than flattened to an empty list: react-query carries it
        // to `isError`, and the palette says the search failed instead of
        // reporting an empty workspace. The builtin commands keep working
        // beside it, which is the degradation that was wanted — losing the
        // sentence was not.
        throw new Error(t("palette.searchFailed"));
      }
      return data.data;
    },
  });
  // Every hit with somewhere to go. `searchHitRoute` is the one place that
  // knows where each kind lives, so a type the server learns to return is
  // routable here the moment it is routable anywhere — and an activity, which
  // has no page, drops out by answering null rather than by being named in a
  // second list that has to be kept in step.
  //
  // An EMAIL hit goes to the search SCREEN with that message open. The palette
  // owns no page and every Command carries a route, so it cannot open a drawer
  // itself — it sends the reader to the one page that already owns this one.
  const hits = (result.data ?? []).flatMap((hit) => {
    const route = searchHitDestination(
      { ...hit, type: hit.type as SearchHitType },
      deferred,
    );
    return route ? [{ hit, route }] : [];
  });
  const projectLines = useProjectHitLines(
    hits.filter(({ hit }) => hit.type === "project").map(({ hit }) => hit.id),
  );
  return {
    commands: hits.map(({ hit, route }) => ({
      id: `record:${hit.type}:${hit.id}`,
      label: hit.title ?? hit.id,
      // A project's secondary line is its key or its company, not the word
      // "project": a search hit for one carries no snippet, and two projects
      // called "Rollout" are told apart by the key a rep already types into
      // subject lines. Every other kind names the kind — TRANSLATED, because
      // this line used to print the wire word and showed a German reader
      // "company" where the rest of the product says Firma.
      subtitle:
        hit.type === "project"
          ? (projectLines.get(hit.id) ??
            t(SEARCH_HIT_KIND_KEY[hit.type as SearchHitType]))
          : t(SEARCH_HIT_KIND_KEY[hit.type as SearchHitType]),
      type: "record" as const,
      route,
    })),
    // `isFetching` rather than `isPending`: a disabled query reports pending
    // forever, and the palette opens with an empty box every time.
    pending: enabled && result.isFetching,
    failed: enabled && result.isError,
  };
}

/**
 * The secondary line for each project hit: the key when the project has one,
 * else the company's name. At most five hits are on screen, so the reads are
 * per record and share the cache entries the project page and the company
 * reference already fill.
 */
function useProjectHitLines(projectIds: string[]): Map<string, string> {
  const projects = useQueries({
    queries: projectIds.map((id) => ({
      queryKey: ["project", id, "ref"],
      staleTime: 60_000,
      queryFn: async () => {
        const { data, error } = await api.GET("/projects/{id}", {
          params: { path: { id } },
        });
        if (error) {
          // A palette line that cannot be resolved falls back to the kind;
          // the hit itself still routes. The project page reports the
          // failure in full.
          return null;
        }
        return data;
      },
    })),
  });
  const companyIds = projects.flatMap((query) =>
    query.data && !query.data.key && query.data.company_id
      ? [query.data.company_id]
      : [],
  );
  const companies = useQueries({
    queries: companyIds.map((id) => ({
      // The same entry EntityRef fills for a company reference.
      queryKey: ["company", "ref", id],
      staleTime: 60_000,
      queryFn: async () => {
        const { data, error } = await api.GET("/companies/{id}", {
          params: { path: { id } },
        });
        if (error) {
          return null;
        }
        return data.display_name ?? null;
      },
    })),
  });
  const companyName = new Map(
    companyIds.map((id, index) => [id, companies[index]?.data ?? null]),
  );
  const lines = new Map<string, string>();
  projects.forEach((query, index) => {
    const project = query.data;
    if (!project) {
      return;
    }
    const line =
      project.key ??
      (project.company_id ? companyName.get(project.company_id) : null);
    if (line) {
      lines.set(projectIds[index], line);
    }
  });
  return lines;
}
