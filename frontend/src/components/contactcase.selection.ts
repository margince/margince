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
 * The two members a `pair`-mode review starts on: the ends of a pair the
 * server offers to merge, touching `start` when one does. The server merges
 * no more than those two in this mode.
 */
function offeredPair(
  contactCase: ContactCase,
  start: string,
): readonly string[] | undefined {
  if (contactCase.merge_mode !== "pair") return undefined;
  const offered = contactCase.edges.filter(
    (edge) => edge.status === "open" && edge.can_merge,
  );
  const touching = offered.find((edge) => otherEnd(edge, start));
  const other = touching && otherEnd(touching, start);
  if (other) return [start, other];
  return offered[0] && [offered[0].left_id, offered[0].right_id];
}

/**
 * The member that must survive when the ticked members are one pair the
 * reader may merge only one way. The server refuses any other survivor.
 */
export function pairKeep(
  contactCase: ContactCase,
  selected: readonly string[],
): string | undefined {
  const [one, two, ...rest] = selected;
  if (contactCase.merge_mode !== "pair" || !one || !two || rest.length > 0) {
    return undefined;
  }
  const joining = contactCase.edges.find(
    (edge) => edge.status === "open" && otherEnd(edge, one) === two,
  );
  return joining?.keep_id ?? undefined;
}

/**
 * The members ticked when the review opens. In `pair` mode it is the offered
 * pair. Otherwise it is a walk over open pairs from `startId`.
 */
export function defaultSelection(
  contactCase: ContactCase,
  startId: string,
): readonly string[] {
  const ids = new Set(contactCase.members.map((member) => member.id));
  const start = ids.has(startId) ? startId : contactCase.anchor_id;
  return offeredPair(contactCase, start) ?? walkOpenPairs(contactCase, start);
}

/**
 * A walk over open pairs from `start`. It skips a member that someone marked
 * as not the same as a member already taken, so it never crosses a dismissed
 * pair. It is always connected, which the server requires.
 */
function walkOpenPairs(
  contactCase: ContactCase,
  start: string,
): readonly string[] {
  const ids = new Set(contactCase.members.map((member) => member.id));
  const taken: string[] = [start];
  const queue = [start];
  for (let at = queue.shift(); at !== undefined; at = queue.shift()) {
    for (const edge of contactCase.edges) {
      const next = edge.status === "open" ? otherEnd(edge, at) : undefined;
      if (!next || !ids.has(next) || taken.includes(next)) continue;
      const refused = dismissedWith(contactCase, next);
      if (taken.some((id) => refused.includes(id))) continue;
      taken.push(next);
      queue.push(next);
    }
  }
  return taken;
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
