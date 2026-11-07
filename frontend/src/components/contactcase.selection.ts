// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { ContactCase } from "./contactcase.queries";

type Edge = components["schemas"]["ContactCaseEdge"];
type Owner = components["schemas"]["ContactCaseOwner"];

function otherEnd(edge: Edge, id: string): string | undefined {
  if (edge.left_id === id) return edge.right_id;
  if (edge.right_id === id) return edge.left_id;
  return undefined;
}

/** The members someone marked as a different person from `id`. */
export function dismissedWith(
  contactCase: ContactCase,
  id: string,
): readonly string[] {
  return contactCase.edges.flatMap((edge) => {
    const other = edge.status === "not_a_duplicate" && otherEnd(edge, id);
    return other ? [other] : [];
  });
}

/**
 * Whether a `pair`-mode review may merge this pair: the server offers it, and
 * what survives can keep a live owner. The case route names an owner; the
 * pair route does not, so `can_merge` alone says nothing about an owner.
 */
function caseMergeable(contactCase: ContactCase, edge: Edge): boolean {
  if (edge.status !== "open" || !edge.can_merge) return false;
  const owning = edge.keep_id ? [edge.keep_id] : [edge.left_id, edge.right_id];
  return contactCase.owners.some((owner) =>
    owner.member_ids.some((id) => owning.includes(id)),
  );
}

function offeredEdges(contactCase: ContactCase): readonly Edge[] {
  if (contactCase.merge_mode !== "pair") return [];
  return contactCase.edges.filter((edge) => caseMergeable(contactCase, edge));
}

/** The offered pair joining the two ticked members, when only two are ticked. */
function offeredJoining(
  contactCase: ContactCase,
  selected: readonly string[],
): Edge | undefined {
  const [one, two, ...rest] = selected;
  if (!one || !two || rest.length > 0) return undefined;
  return offeredEdges(contactCase).find((edge) => otherEnd(edge, one) === two);
}

/**
 * The two members a `pair`-mode review starts on: the ends of an offered
 * pair, touching `start` when one does. The server merges no more than two.
 */
function offeredPair(
  contactCase: ContactCase,
  start: string,
): readonly string[] | undefined {
  const offered = offeredEdges(contactCase);
  const touching = offered.find((edge) => otherEnd(edge, start));
  const other = touching && otherEnd(touching, start);
  if (other) return [start, other];
  return offered[0] && [offered[0].left_id, offered[0].right_id];
}

/**
 * The member that must survive when the ticked members are one offered pair
 * the reader may merge only one way. The server refuses any other survivor.
 */
export function pairKeep(
  contactCase: ContactCase,
  selected: readonly string[],
): string | undefined {
  return offeredJoining(contactCase, selected)?.keep_id ?? undefined;
}

/** Why a `pair`-mode selection cannot merge: it is not one offered pair. */
export function pairProblem(
  contactCase: ContactCase,
  selected: readonly string[],
): "contactCase.pairOnly" | null {
  if (contactCase.merge_mode !== "pair") return null;
  return offeredJoining(contactCase, selected) ? null : "contactCase.pairOnly";
}

/**
 * Whether a `pair`-mode review keeps `id` from being ticked: only the other
 * end of an offered pair may join one ticked member, and nothing joins two.
 */
export function pairLocked(
  contactCase: ContactCase,
  selected: readonly string[],
  id: string,
): boolean {
  if (contactCase.merge_mode !== "pair" || selected.includes(id)) return false;
  const partners = offeredEdges(contactCase).flatMap((edge) => {
    const other = otherEnd(edge, id);
    return other ? [other] : [];
  });
  if (selected.length === 0) return partners.length === 0;
  return selected.length > 1 || !partners.includes(selected[0] ?? "");
}

/**
 * The members ticked when the review opens: one pair, never a chain. In `pair`
 * mode it is the offered pair. Otherwise it is `startId` and its strongest open
 * pair, because A matching B and B matching C says nothing about A and C.
 */
export function defaultSelection(
  contactCase: ContactCase,
  startId: string,
): readonly string[] {
  const ids = new Set(contactCase.members.map((member) => member.id));
  const start = ids.has(startId) ? startId : contactCase.anchor_id;
  return offeredPair(contactCase, start) ?? strongestPair(contactCase, start);
}

function strongestPair(
  contactCase: ContactCase,
  start: string,
): readonly string[] {
  const ids = new Set(contactCase.members.map((member) => member.id));
  let best: Edge | undefined;
  for (const edge of contactCase.edges) {
    const other = edge.status === "open" ? otherEnd(edge, start) : undefined;
    if (!other || !ids.has(other)) continue;
    if (!best || edge.confidence > best.confidence) best = edge;
  }
  const other = best && otherEnd(best, start);
  return other ? [start, other] : [start];
}

/** Whether the ticked members are joined by open pairs among themselves. */
export function isConnected(
  contactCase: ContactCase,
  selected: readonly string[],
): boolean {
  if (selected.length === 0) return false;
  const reached = new Set([selected[0]]);
  const queue = [selected[0]];
  for (let at = queue.shift(); at !== undefined; at = queue.shift()) {
    for (const edge of contactCase.edges) {
      const next = edge.status === "open" ? otherEnd(edge, at) : undefined;
      if (next && selected.includes(next) && !reached.has(next)) {
        reached.add(next);
        queue.push(next);
      }
    }
  }
  return reached.size === selected.length;
}

/**
 * The live owners of the ticked members: the only owners a merge may pick. A
 * one-way pair keeps its survivor's owner, so that owner is the only choice.
 */
export function ownerChoices(
  contactCase: ContactCase,
  selected: readonly string[],
): readonly Owner[] {
  const keep = pairKeep(contactCase, selected);
  const owning = keep ? [keep] : selected;
  return contactCase.owners.filter((owner) =>
    owner.member_ids.some((id) => owning.includes(id)),
  );
}

/**
 * The owner the review starts on: the kept record's own owner when that owner
 * may own the result. Otherwise no one, so the reviewer picks.
 */
export function defaultOwner(
  contactCase: ContactCase,
  selected: readonly string[],
  survivorId: string,
): string {
  const survivor = contactCase.members.find(
    (member) => member.id === survivorId,
  );
  const owner = survivor?.owner_id;
  return owner &&
    ownerChoices(contactCase, selected).some((one) => one.user_id === owner)
    ? owner
    : "";
}

/**
 * How a member is named where the reviewer must tell members apart. Duplicates
 * often share a name, since that is why they look alike, so a shared name
 * carries the email too.
 */
export function memberLabel(contactCase: ContactCase, id: string): string {
  const member = contactCase.members.find((one) => one.id === id);
  if (!member) return "";
  const shared = contactCase.members.some(
    (one) => one.id !== id && one.name === member.name,
  );
  return shared && member.email
    ? `${member.name} (${member.email})`
    : member.name;
}
