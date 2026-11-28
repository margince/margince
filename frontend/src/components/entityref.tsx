// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { AppLink } from "../design-system/applink";
import { Badge } from "../design-system/atoms";
import { useT } from "../i18n";
import { recordEnabled, useHiddenCoreFeatures } from "../lib/corefeatures";
import { ENTITY, type EntityKind } from "../lib/entity";
import { useMemberName } from "../lib/membernames";
import { routeHash } from "../lib/router";
import {
  entityReferenceObserverQuery,
  type NameReading,
  readingOf,
  usableName,
} from "./entityref.queries";
import { type RosterKind, useRosterWalk } from "./roster";
import { SeatAvatar } from "./seatavatar";

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
// the roster walk (see `RosterRef`).

export {
  ENTITY_NAME_KEY,
  useEntityName,
} from "./entityref.queries";
export type { RosterKind, RosterRead } from "./roster";
export {
  RosterFailedLine,
  RosterPartialNote,
  useRoster,
  useRosterHint,
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
 * id was for (an account's owner, a request's assignee), and it is the one
 * reading that claims the read answered about them. The other two readings
 * say the same thing wherever they happen: a read still in flight has said
 * nothing yet, and one that failed has said nothing about this id.
 */
export function missLabel(
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
 * A picker whose current value matches no option renders blank, which reads
 * as unset, so a value the roster cannot name still needs a label, and this is
 * the one that says truthfully why it has none.
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
  archived,
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
  // Supply both facts when the caller already has them. A name alone keeps
  // its spelling while the shared batched read resolves the archive state.
  name?: string | null;
  archived?: boolean;
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
      archived={archived}
      asText={asText}
      newTab={newTab}
    />
  );
}

// A workspace user or team: no 360 exists to send the reader to, so a resolved
// name renders as plain text and the reference never becomes a link.
//
// A user is named by id and a team is still named off the walk. The asymmetry
// is intended: teams carry no invited seats, so the walk's budget was never a
// naming question for them. A `/teams/names` read would make both arms one.
function RosterRef({
  kind,
  id,
  name,
}: Readonly<{ kind: RosterKind; id: string; name?: string | null }>) {
  const { name: resolved, reading } = useRosterName(kind, id, name);
  if (resolved == null) {
    return <UnnamedRef id={id} reading={reading} />;
  }
  return <span>{resolved}</span>;
}

/**
 * The name a colleague or team reference shows, and the reading that stands in
 * when there is none. Exported so a label that names the same id (an
 * accessible name, a question) says what the reference on screen says.
 *
 * A user is named by id, so a departed or invited seat still has its name. A
 * team is named off the roster walk. A caller-supplied name wins, as it does
 * for a record: the connection graph returns its own labels.
 */
export function useRosterName(
  kind: RosterKind,
  id: string,
  supplied?: string | null,
): { name: string | null; reading: NameReading } {
  const given = usableName(supplied);
  const member = useMemberName(kind === "user" && given == null ? id : null);
  const teams = useRosterWalk("team", kind === "team" && given == null);
  if (given != null) {
    return { name: given, reading: "unnamed" };
  }
  if (kind === "user") {
    return { name: usableName(member.data), reading: readingOf(member) };
  }
  const match = teams.data?.entries.find((entry) => entry.id === id);
  // `match` is `User | Team`; only a Team has `name`, so the property check
  // narrows it without a cast.
  return {
    name: match && "name" in match ? usableName(match.name) : null,
    reading: rosterReading(teams, teams.data?.partial === true),
  };
}

/** A record with a 360 behind it: a resolved name is also the backlink. */
function RecordRef({
  kind,
  id,
  name,
  archived,
  asText,
  newTab,
}: Readonly<{
  kind: EntityKind;
  id: string;
  name?: string | null;
  archived?: boolean;
  asText: boolean;
  newTab: boolean;
}>) {
  const t = useT();
  const available = recordEnabled(kind, useHiddenCoreFeatures());
  const supplied = usableName(name);
  const query = useQuery({
    ...entityReferenceObserverQuery(kind, id),
    enabled: available && (supplied == null || archived === undefined),
  });
  // Only a resolved name is a safe link target; a reference with no name —
  // still loading, refused, or a record that carries none — never becomes one.
  const resolved = supplied ?? usableName(query.data?.name);
  if (!available) {
    return supplied ? <span>{supplied}</span> : null;
  }
  const label =
    resolved == null ? (
      <UnnamedRef id={id} reading={readingOf(query)} />
    ) : asText ? (
      <span>{resolved}</span>
    ) : (
      <AppLink
        className="entity-link"
        href={routeHash(ENTITY[kind].route(id))}
        onClick={(event) => event.stopPropagation()}
        {...(newTab ? { target: "_blank", rel: "noopener noreferrer" } : {})}
      >
        {resolved}
      </AppLink>
    );
  return (
    <>
      {label}
      {(archived ?? query.data?.archived) && (
        <>
          {" "}
          <Badge tone="warning">{t("record.archived")}</Badge>
        </>
      )}
    </>
  );
}

/**
 * rosterOwnerName names a record's owner by id, the same by-id read every
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
 * story and test of it has to assemble a hook result for. Null is both
 * "unowned" and "the read has not named this id": on a card the two draw
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
 * The owner of a record, by name, for a list column. `useMemberName` batches
 * the ids of one tick, so a page of rows costs one request.
 *
 * An unnamed owner renders as an unnamed reference, never blank: a blank cell
 * reads as unowned. `face` adds the avatar, keyed by user id.
 */
export function OwnerName({
  ownerId,
  unowned,
  face = false,
}: Readonly<{ ownerId?: string | null; unowned: string; face?: boolean }>) {
  const query = useMemberName(ownerId);
  if (!ownerId) {
    return <span>{unowned}</span>;
  }
  const resolved = usableName(query.data);
  if (resolved != null && face) {
    return (
      <span className="avatar-row">
        <SeatAvatar id={ownerId} name={resolved} />
        <span>{resolved}</span>
      </span>
    );
  }
  if (resolved != null) {
    return <span>{resolved}</span>;
  }
  return <UnnamedRef id={ownerId} reading={readingOf(query)} />;
}
