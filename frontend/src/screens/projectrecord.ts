// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
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
export function useProjectRecord(id?: string): {
  project: Project | null;
  settled: boolean;
} {
  const query = useQuery({
    // Under the ["project", id] prefix the edit and archive paths already
    // invalidate, so a renamed or archived project reaches this reader too. A
    // key outside that prefix would keep serving the old record for its whole
    // freshness window.
    queryKey: ["project", id, "record"],
    queryFn: async () => {
      const { data, error } = await api.GET("/projects/{id}", {
        params: { path: { id: id ?? "" } },
      });
      // A project the reader may not open is not an error to shout about here:
      // the caller draws nothing either way, and a throw would take the
      // composer down over a record it only wanted to name.
      return error ? null : data;
    },
    enabled: Boolean(id),
    staleTime: 60_000,
  });
  return {
    project: query.data ?? null,
    // Whether the read has finished, however it finished. A caller that states
    // a filing must not state it while the answer is still on its way.
    settled: Boolean(id) && !query.isPending,
  };
}

/**
 * The bracketed key a subject carries so the customer's reply files itself.
 *
 * The capture side reads `[KEY]` out of an inbound subject (the T1 rung of the
 * attribution ladder) and matches it case-insensitively against live project
 * keys. Writing the same shape on the way out is what closes that loop: a
 * conversation started here comes back filed under the project it started in,
 * even when the mail thread is broken by a forward or a new subject.
 *
 * A project with no key gets no tag rather than an empty `[]`, which the
 * matcher reads as nothing anyway.
 */
export function subjectTag(project: Project | null): string {
  // An ARCHIVED project's key is not ours to write. A key is unique among LIVE
  // projects only, so an archived one may already have been taken by a new
  // project — stamping it would route the customer's reply to whichever project
  // holds that key now, which is not the one this message is about.
  if (!project?.key || project.archived_at) {
    return "";
  }
  return `[${project.key}]`;
}

/**
 * `subject` carrying `tag` exactly once, at the front.
 *
 * Idempotent on purpose, and called on EVERY edit while a project is chosen:
 * the tag is kept at the front of the subject rather than written once, so
 * re-applying must neither stack (`[N2P-1] [N2P-1] Re: …`) nor disturb the
 * text around it. The rest of the subject comes back byte for byte — it is
 * what the rep is typing, and tidying it would rewrite their words under the
 * cursor.
 */
export function withSubjectTag(subject: string, tag: string): string {
  if (!tag) {
    return subject;
  }
  return prefixed(stripEveryKeyTag(subject), tag);
}

/**
 * `subject` with every bracketed project key removed, wherever it sits.
 *
 * Not only the leading one, and not only ours: the inbound matcher reads EVERY
 * `[...]` group in a subject, and two live keys make the attribution ambiguous
 * — it resolves to neither. So a subject that already names another project, or
 * names one mid-line, has to be cleared before this one is written, or the tag
 * added here silently cancels itself out.
 *
 * Only key-SHAPED groups go. `[FYI]` is prose to the matcher and prose here.
 */
export function stripEveryKeyTag(subject: string): string {
  // A removed tag takes ONE of the spaces that surrounded it with it, so
  // "Re: [KEY] Hallo" becomes "Re: Hallo" rather than "Re:  Hallo". The rest of
  // the line is returned byte for byte: this runs over a subject the rep is
  // typing, and a global whitespace collapse would rewrite their spacing —
  // eating the second space of "Re:  Kurzer" that they put there on purpose.
  // Exactly ONE space on each side is eligible to go with the tag — the
  // separator this module writes. A second space is the rep's own and stays.
  return subject.replace(/ ?\[[^\]]*\] ?/g, (group) => {
    const inner = group.trim().slice(1, -1);
    if (!keyShaped(inner)) {
      return group;
    }
    // Keep one separator when the tag sat BETWEEN two things; keep none when it
    // sat at either end.
    return group.startsWith(" ") && group.endsWith(" ") ? " " : "";
  });
}

/**
 * `subject` with a leading `tag` removed, if it carries one — along with the
 * ONE space this module puts after it, and no more.
 *
 * What Margince manages is the tag plus a single separator. Everything after
 * that is the rep's, including a second space they typed themselves: stripping
 * every leading space would take `[KEY]  Re:` down to `Re:` and quietly change
 * what they wrote.
 */
export function stripSubjectTag(subject: string, tag: string): string {
  if (!tag || !subject.startsWith(tag)) {
    return subject;
  }
  const rest = subject.slice(tag.length);
  return rest.startsWith(" ") ? rest.slice(1) : rest;
}

/**
 * `body` behind `tag`, with the one separator this module owns.
 *
 * `body` arrives already stripped of the tag and its separator, so it is used
 * verbatim — leading spaces included, because those are the rep's.
 */
function prefixed(body: string, tag: string): string {
  return body === "" ? tag : `${tag} ${body}`;
}

/**
 * Whether one bracketed token could be a project key — the frontend reading of
 * the rule the capture side applies (`project_key_shape`): letter-led, then
 * letters, digits and hyphens, bounded in length. A bare number is deliberately
 * excluded, so `[2026]` and `[4711]` stay prose.
 */
function keyShaped(token: string): boolean {
  return /^[a-z][a-z0-9-]{1,23}$/i.test(token.trim());
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
