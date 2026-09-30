// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { ENTITY, type EntityKind } from "../app/entity";
import { routeHash } from "../app/router";
import { useT } from "../i18n";
import {
  ENTITY_NAME_KEY,
  fetchEntityName,
  type NameReading,
  readingOf,
  usableName,
} from "./entityref.queries";
import { useMemberName } from "./membernames";
import { type RosterKind, useRosterWalk } from "./roster";

// A cross-record reference rendered as the target's display name plus a
// backlink to its 360, resolved by id. Records point at each other by id
// across the contract (owner, counterparty, partner company, deal); showing the
// raw UUID is honest but unreadable, so this hydrates the name off the record
// read and links through. A reference that cannot be named renders the id
// (as text, no link) rather than blank or a dead link — on an audit row or a
// history entry that id is the one traceable fact left. A reference whose read
// has not answered YET, or whose read came back refused, says so instead:
// a name that is coming, a name that is never coming, and a name nobody could
// read at all are three different facts.
//
// `user`/`team` are the one exception to the "resolved name is a link" rule:
// there is no 360 to send them to, so a resolved name always renders as plain
// text and never touches the ENTITY registry, which has no `user`/`team`
// entry. A user is named by id (`/users/names`); a team is still named off
// the roster walk — see `RosterRef`.

export {
  ENTITY_NAME_KEY,
  fetchEntityName,
  useEntityName,
} from "./entityref.queries";
export type { RosterKind } from "./roster";
export {
  RosterPartialNote,
  useRoster,
  useRosterPartial,
  useRosterPartialHint,
} from "./roster";
export type EntityRefKind = EntityKind | RosterKind;

/**
 * What a roster that could not name an id is allowed to say.
 *
 * A roster walked to the end has ANSWERED about this id: whoever holds it is
 * not somebody this reader may list — deactivated, or gone — and the id is what
 * is left to trace, which is the settled `unnamed`. A roster that stopped at
 * its page budget has answered nothing about it: the name may sit on a page
 * nobody read. That reading borrows `failed` — nothing further is coming
 * without a new read, so it is not `pending`, and printing the id would state
 * as settled fact a question this walk never reached.
 *
 * Exported because every surface that names a roster id has to make the same
 * three-way call, and the ones that made it separately made it differently: a
 * picker went on reporting a colleague as departed while the walk beside it had
 * not finished reading. One classification, and each caller decides only what to
 * SAY about it.
 */
export function rosterReading(
  query: Readonly<{ isPending: boolean; isError: boolean }>,
  partial: boolean,
): NameReading {
  const reading = readingOf(query);
  return reading === "unnamed" && partial ? "failed" : reading;
}

/**
 * What a reading is allowed to say once there is no name to show:
 * `unlisted` is the CALLER's sentence, because only the caller knows what the
 * id was for — an account's owner, a request's assignee — and it is the one
 * reading that claims the read answered about them. The other two readings
 * say the same thing wherever they happen: a read still in flight has said
 * nothing yet, and one that failed has said nothing about THIS id.
 */
function missLabel(
  reading: NameReading,
  t: ReturnType<typeof useT>,
  unlisted: string,
): string {
  if (reading === "pending") {
    return t("common.loading");
  }
  return reading === "failed" ? t("ref.nameLoadFailed") : unlisted;
}

/**
 * What to call a roster id the WALK could not name.
 *
 * A team is named this way: teams carry no invited seats, so the walk's page
 * budget was never a naming question for them. A colleague is named by id
 * through `useMemberName`, which answers about that id alone and has no walk to
 * stop short.
 *
 * A picker whose current value matches no option renders blank — indistinguish-
 * able from unset — so a value the roster cannot name still needs a label, and
 * this is the one that is honest about why it has none.
 */
export function rosterMissLabel(
  roster: Readonly<{ isPending: boolean; isError: boolean }>,
  partial: boolean,
  t: ReturnType<typeof useT>,
  unlisted: string,
): string {
  return missLabel(rosterReading(roster, partial), t, unlisted);
}

function UnnamedRef({
  id,
  reading,
}: Readonly<{ id: string; reading: NameReading }>) {
  const t = useT();
  if (reading === "pending") {
    return <span>{t("common.loading")}</span>;
  }
  if (reading === "failed") {
    // The id stays reachable through the title rather than printed as the
    // value: on the line it reads as what the read came back with, and the
    // read came back with nothing.
    return <span title={id}>{t("ref.nameLoadFailed")}</span>;
  }
  return <span title={id}>{id}</span>;
}

/**
 * A cross-record reference, as the target's display name and a way to it.
 *
 * Records point at each other by id across the contract, and a raw uuid on a
 * line is honest and unreadable. This is the one place that turns one into a
 * name — so a reference that cannot be named degrades the same way wherever it
 * appears, rather than each surface inventing its own fallback.
 */
export function EntityRef({
  kind,
  id,
  name,
  asText = false,
  newTab = false,
}: Readonly<{
  kind: EntityRefKind;
  id: string | null | undefined;
  /**
   * Name the record without linking to it, for a caller that is already a link
   * to the same place — a list row's identity cell. A control nested inside a
   * link is invalid markup, and the second route would go where the first one
   * already goes.
   */
  asText?: boolean;
  /**
   * Open the record in a NEW tab, for a caller the reader must not be
   * navigated away from: a reference inside a dialog. Following it in place
   * closes the dialog and loses whatever was open in it — the message being
   * read, the draft being written — to reach a page the reader could have
   * opened from behind it anyway.
   *
   * A `user`/`team` reference ignores it, having no page to open at all.
   */
  newTab?: boolean;
  // The display name, when the CALLER already has it. A composite read that
  // returns its own labels — the company view's connection graph — would
  // otherwise pay one record fetch per reference and show the raw id until each
  // one lands. Passing it skips the lookup entirely; the link and the id
  // fallback are unchanged.
  name?: string | null;
}>) {
  if (!id) {
    return <span>—</span>;
  }
  // Dispatch on the kind rather than running both resolutions and discarding
  // one. Each branch then owns exactly the read it needs — no query has to be
  // told to stay switched off, and none can report itself as loading when it
  // was never going to run — and `kind` narrows here instead of being asserted
  // inside a body that serves both.
  if (kind === "user" || kind === "team") {
    return <RosterRef kind={kind} id={id} name={name} />;
  }
  return (
    <RecordRef
      kind={kind}
      id={id}
      name={name}
      asText={asText}
      newTab={newTab}
    />
  );
}

// A workspace user or team: no 360 exists to send the reader to, so a resolved
// name renders as plain text and the reference never becomes a link.
//
// A user is named by id and a team is still named off the walk. The asymmetry
// is deliberate: teams carry no invited seats, so the walk's budget was never a
// naming question for them. A `/teams/names` read would make both arms one.
function RosterRef({
  kind,
  id,
  name,
}: Readonly<{ kind: RosterKind; id: string; name?: string | null }>) {
  if (kind === "user") {
    return <UserRef id={id} name={name} />;
  }
  return <TeamRef id={id} name={name} />;
}

// A resolved name as plain text, or the reading `UnnamedRef` owes a reader who
// gets none — shared by both roster arms so neither draws its fallback
// differently from the other.
function resolvedOrFallback(
  id: string,
  resolved: string | null,
  reading: NameReading,
) {
  if (resolved == null) {
    return <UnnamedRef id={id} reading={reading} />;
  }
  return <span title={id}>{resolved}</span>;
}

function UserRef({ id, name }: Readonly<{ id: string; name?: string | null }>) {
  // A caller-supplied name wins here exactly as it does for a record: the
  // connection graph returns its own labels rather than a raw uuid until the
  // by-id read resolves.
  const supplied = usableName(name);
  const query = useMemberName(supplied == null ? id : null);
  return resolvedOrFallback(
    id,
    supplied ?? usableName(query.data),
    readingOf(query),
  );
}

function TeamRef({ id, name }: Readonly<{ id: string; name?: string | null }>) {
  const supplied = usableName(name);
  const roster = useRosterWalk("team", supplied == null);
  const match = roster.data?.entries.find((entry) => entry.id === id);
  // `match` is `User | Team`; only a Team has `name`, so the property check
  // narrows it without a cast.
  const resolved =
    supplied ?? (match && "name" in match ? usableName(match.name) : null);
  return resolvedOrFallback(
    id,
    resolved,
    rosterReading(roster, roster.data?.partial === true),
  );
}

/** A record with a 360 behind it: a resolved name is also the backlink. */
function RecordRef({
  kind,
  id,
  name,
  asText,
  newTab,
}: Readonly<{
  kind: EntityKind;
  id: string;
  name?: string | null;
  asText: boolean;
  newTab: boolean;
}>) {
  // A caller-supplied name skips the lookup; a blank one does not, because a
  // blank is the caller saying it has nothing rather than saying the record is
  // nameless. `usableName` is what decides that, once, so the value that
  // switches the read off is the same value that gets rendered.
  const supplied = usableName(name);
  const query = useQuery({
    queryKey: [kind, ENTITY_NAME_KEY, id],
    queryFn: () => fetchEntityName(kind, id),
    enabled: supplied == null,
    // References change rarely relative to the pages that render them; a short
    // cache keeps a 360 from re-fetching the same name on every hover/refetch.
    staleTime: 60_000,
  });
  // Only a resolved name is a safe link target; a reference with no name —
  // still loading, refused, or a record that carries none — never becomes one.
  const resolved = supplied ?? usableName(query.data);
  if (resolved == null) {
    return <UnnamedRef id={id} reading={readingOf(query)} />;
  }
  if (asText) {
    return <span title={id}>{resolved}</span>;
  }
  // A real anchor, so the record can be opened the ways a link can: a new tab,
  // a new window, a bookmark, the keyboard. A button carried the same route and
  // offered none of them — a reader who middle-clicked a company in a list got
  // nothing, and one who wanted it beside the page they were on had to lose
  // that page to get there.
  //
  // Only the default click is stopped from reaching an enclosing row's own
  // handler. `navigate` is NOT called beside it: the anchor already performs
  // the navigation, so doing both would dispatch the same route twice, and
  // preventing the anchor instead would navigate the current page while the
  // new tab opens too. This is the identity cell's own arrangement
  // (design-system/listtable.tsx), which is where a row and a link inside it
  // were first made to agree.
  //
  // `newTab` carries `rel` with it, never alone: `target="_blank"` without
  // `noopener` hands the opened page a live `window.opener` handle back into
  // this tab. `OffsiteLink` spells the same pair for the same reason.
  return (
    <a
      className="entity-link"
      href={routeHash(ENTITY[kind].route(id))}
      onClick={(event) => event.stopPropagation()}
      title={id}
      {...(newTab ? { target: "_blank", rel: "noopener noreferrer" } : {})}
    >
      {resolved}
    </a>
  );
}

/**
 * rosterOwnerName names a record's owner by id — the same by-id read every
 * other reference in this file resolves through, never a walk. An owner the
 * read has not named yet gets that read's own reading of why, never a bare id.
 */
export function rosterOwnerName(
  ownerId: string | null | undefined,
  name: ReturnType<typeof useMemberName>,
  t: ReturnType<typeof useT>,
  unowned: string,
): string {
  if (!ownerId) {
    return unowned;
  }
  return (
    usableName(name.data) ?? missLabel(readingOf(name), t, t("ref.notInRoster"))
  );
}

/**
 * How a caller that maps records onto cards names their owners.
 *
 * A function rather than the map itself, for the same reason the company
 * mapping takes `CompanyNaming`: the mapper is pure and `useMemberNames`'s
 * result is a hook's, and a mapper that reads one directly is one that every
 * story and test of it has to assemble a hook result for. Null is BOTH
 * "unowned" and "the read has not named this id" — on a card the two draw
 * the same nothing, and the table's owner column is where they are told
 * apart.
 */
export type OwnerNaming = (ownerId: string | null | undefined) => string | null;

export function rosterOwnerNaming(
  names: ReadonlyMap<string, string>,
): OwnerNaming {
  return (ownerId) => (ownerId ? (names.get(ownerId) ?? null) : null);
}

/**
 * The owner of a record, by name, for a list column.
 *
 * Reads `useMemberName`, batching every id asked for within one tick into a
 * single request, so a list of 50 rows costs one request rather than fifty.
 * An owner the read cannot name still renders rather than going blank — a
 * blank column reads as unowned, a different fact with its own filter — but
 * as the same unnamed reference every other cross-record reference gets, not
 * a truncated id, which is a non-answer that has also lost the ability to be
 * looked up.
 */
export function OwnerName({
  ownerId,
  unowned,
}: Readonly<{ ownerId?: string | null; unowned: string }>) {
  const query = useMemberName(ownerId);
  if (!ownerId) {
    return <span>{unowned}</span>;
  }
  const resolved = usableName(query.data);
  if (resolved != null) {
    return <span>{resolved}</span>;
  }
  return <UnnamedRef id={ownerId} reading={readingOf(query)} />;
}
