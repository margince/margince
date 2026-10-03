// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api, FIRST_PAGE } from "../api/client";
import type { components } from "../api/schema";
import { useT } from "../i18n";
import { throwProblem } from "./common";

// The workspace roster as the frontend reads it: one walk over `/users` or
// `/teams`, cached once, and the hooks that read it. Split out of entityref.tsx,
// which names colleagues with it.

// user/team are EntityRef-only: they have no 360 to route to, so they resolve
// off the shared roster list and render as plain text.
export type RosterKind = "user" | "team";

type User = components["schemas"]["User"];
type Team = components["schemas"]["Team"];

// Roster lookups share one cache entry across every EntityRef + the Share
// picker: `/users` and `/teams` are workspace-wide lists, so reading one list
// once and finding-by-id is cheaper (and more cacheable) than a per-id GET for
// every rendered reference.

type RosterEntry = User | Team;

// HOW THE ROSTER IS READ, AND WHERE IT STOPS.
//
// This walk is the PICKER's list: who a reader may choose as an owner, a
// subject, a filter value. Naming somebody a record already points at is a
// separate read (`useMemberName`, membernames.ts) — a by-id lookup that never
// walks and never runs out of budget, which is what makes it safe to leave
// invited and deactivated seats off this list entirely: they may be pointed at
// but never chosen.
//
// Both list endpoints are keyset-paged and the contract caps `limit` at 200, so
// ONE page is not the roster: past 200 members an owner on the second page
// resolved to a raw uuid and every picker built on this hook silently dropped
// everyone above the cap. The walk follows `next_cursor` to the end instead.
//
// It is BOUNDED because the cursor is the server's to mint, and a walk that
// trusts one unconditionally is a page that never paints. ROSTER_WALK_PAGES
// pages of the contract's maximum page size reach 2 000 members — well past the
// largest workspace one installation serves, and already ten SEQUENTIAL round
// trips, which is where opening a picker becomes a wait a reader notices.
// Beyond that the answer is a server-side resolve rather than more client
// pages, so the walk stops and says so: `partial` travels with the entries,
// because a truncation nobody states reads as the whole workspace.
const ROSTER_PAGE_SIZE = 200;
const ROSTER_WALK_PAGES = 10;

/** The roster as one list, plus whether it is the WHOLE list. */
type Roster = { entries: RosterEntry[]; partial: boolean };

async function readRosterPage(
  kind: RosterKind,
  cursor: string | null,
): Promise<{ entries: RosterEntry[]; next: string | null }> {
  // The two endpoints answer differently-typed rows, so each arm reads its own
  // — a shared call would have to assert one shape onto the other.
  if (kind === "user") {
    const { data, error } = await api.GET("/users", {
      params: {
        query: {
          limit: ROSTER_PAGE_SIZE,
          ...(cursor ? { cursor } : {}),
        },
      },
    });
    if (error) throwProblem(error);
    return { entries: data.data, next: data.page.next_cursor ?? null };
  }
  const { data, error } = await api.GET("/teams", {
    params: {
      query: { limit: ROSTER_PAGE_SIZE, ...(cursor ? { cursor } : {}) },
    },
  });
  if (error) throwProblem(error);
  return { entries: data.data, next: data.page.next_cursor ?? null };
}

async function walkRoster(kind: RosterKind): Promise<Roster> {
  const entries: RosterEntry[] = [];
  let cursor = FIRST_PAGE;
  for (let page = 0; page < ROSTER_WALK_PAGES; page += 1) {
    // A page that fails throws out of the whole walk (through `throwProblem`
    // inside `readRosterPage`), so react-query holds the read as an error.
    // Caught and dropped here it would come back as a SHORT list that reads as
    // complete — the one shape a caller cannot tell from a small workspace.
    const answered = await readRosterPage(kind, cursor);
    entries.push(...answered.entries);
    // The CURSOR is what the walk can continue with, and `has_more` without one
    // is a list nothing can read the rest of — so both ends of the walk stop
    // here rather than looping on a cursor that will not move.
    cursor = answered.next;
    if (!cursor) {
      return { entries, partial: false };
    }
  }
  return { entries, partial: true };
}

// One cache entry per roster kind, shared by every consumer — the Share
// picker, each owner column, every EntityRef on the page — so the walk runs
// once per minute however many surfaces read it.
function rosterQueryOptions(kind: RosterKind, enabled: boolean) {
  return {
    queryKey: [kind === "user" ? "users" : "teams"],
    queryFn: () => walkRoster(kind),
    enabled,
    staleTime: 60_000,
  };
}

// Both facts off the one entry, for the two callers below that need the
// entries AND whether they are all of them; one observer rather than two.
export function useRosterWalk(kind: RosterKind, enabled: boolean) {
  return useQuery(rosterQueryOptions(kind, enabled));
}

/**
 * The roster's entries — everyone a picker may offer.
 *
 * Exported so the Share subject picker, the owner pickers and a team's own
 * name resolution all build off the exact same cache entry — one walk, one
 * cache key, every consumer. Invited and deactivated seats never reach this
 * list: the server leaves them out because nothing here asks for them, which
 * is what keeps the walk's budget spent on colleagues who can actually be
 * chosen.
 *
 * A consumer that OFFERS these entries as a list of who exists owes its reader
 * `useRosterPartial` beside it: this result cannot say whether the walk reached
 * the end, and a picker missing colleagues looks exactly like a small workspace.
 */
export function useRoster(kind: RosterKind, enabled: boolean) {
  return useQuery({
    ...rosterQueryOptions(kind, enabled),
    select: (roster: Roster) => roster.entries,
  });
}

/**
 * Whether the roster this surface is showing stopped short of the workspace.
 *
 * A second hook rather than a field on `useRoster`'s result, because it reads
 * the SAME cache entry — same key, no extra request — and the many consumers
 * that only resolve names never have to thread a flag they do not render.
 *
 * False while the walk is still running and false once it finished: a caveat
 * about a list nobody has read yet is noise, and one about a complete list is
 * simply untrue.
 */
export function useRosterPartial(kind: RosterKind, enabled: boolean): boolean {
  const query = useQuery({
    ...rosterQueryOptions(kind, enabled),
    select: (roster: Roster) => roster.partial,
  });
  return query.data === true;
}

/**
 * The words a picker owes its reader when the roster behind it is not the whole
 * workspace: the list they are looking at is part of one, and the colleague
 * they cannot find may be somebody this surface never read.
 *
 * The sentence AND the decision that there is one to say, in one place, so a
 * picker cannot invent a gentler wording for the same gap. Exported as words
 * rather than only as markup for the callers whose slot takes a string: a
 * `Field` hint owns its own paragraph and wires it into the control's
 * `aria-describedby`, and a second paragraph nested inside that one would be
 * invalid markup describing nothing. `RosterPartialNote` is the element form of
 * the same fact, so both stay one authority.
 */
export function useRosterPartialHint(partial: boolean): string | undefined {
  const t = useT();
  return partial ? t("state.partial") : undefined;
}

/**
 * The caveat as its own line, for a picker with room for one beside it.
 *
 * Takes the FACT rather than reading the roster itself — `useRosterPartial` at
 * the call site is the read the surface already made, and a note with a query of
 * its own could arm a fetch the surface deferred. Renders nothing when the walk
 * reached the end, so it can sit unconditionally beside the control it is about.
 *
 * `id` is for the control that names this line in its `aria-describedby`, which
 * is how a caveat that cannot sit next to its control stays attached to it.
 * `aria-live="off"` is the same concern from the other side: a caveat that lands
 * inside somebody else's live region — the bulk bar is one — would be announced
 * as news when the walk finishes, interrupting whatever the reader was doing to
 * report a fact about a list they were not asking about.
 */
export function RosterPartialNote({
  partial,
  id,
}: Readonly<{ partial: boolean; id?: string }>) {
  const hint = useRosterPartialHint(partial);
  if (!hint) {
    return null;
  }
  return (
    <p className="t-caption" id={id} aria-live="off">
      {hint}
    </p>
  );
}
