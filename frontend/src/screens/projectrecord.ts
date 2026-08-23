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
 * Idempotent on purpose: the composer re-derives the subject whenever the
 * filing changes, and a rep may also have typed the tag themselves. Re-applying
 * must not produce `[N2P-1] [N2P-1] Re: …`. A tag the rep DELETED stays
 * deleted — that is the opt-out, and this function is only called when the
 * filing itself changes, never on every keystroke.
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
  return (
    subject
      .replace(/\[[^\]]*\]/g, (group) =>
        keyShaped(group.slice(1, -1)) ? "" : group,
      )
      // Removing a tag from mid-line leaves the spaces that surrounded it, and a
      // subject reading "Re:  Hallo" is a tell that something was cut out of it.
      .replace(/\s{2,}/g, " ")
  );
}

/** `subject` with a leading `tag` removed, if it carries one. */
export function stripSubjectTag(subject: string, tag: string): string {
  if (!tag) {
    return subject;
  }
  const trimmed = subject.trimStart();
  return trimmed.startsWith(tag)
    ? trimmed.slice(tag.length).trimStart()
    : subject;
}

/** `body` behind `tag`, with one space and no leading blank. */
function prefixed(body: string, tag: string): string {
  const rest = body.trim();
  return rest ? `${tag} ${rest}` : tag;
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
