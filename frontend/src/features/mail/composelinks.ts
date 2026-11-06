// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Grounding } from "./composedraftcall";
import type { RelinkKind } from "./composerelink";

export type Link = { entity_type: RelinkKind; entity_id: string };

/**
 * What a sent message files under: the page it was written from, and every
 * record the rep named while writing it.
 *
 * The grounding choices are the attribution, so they travel with the send. A
 * link is matched on kind and id, the way the server identifies it, and a
 * repeat is dropped because the link table refuses one.
 *
 * `chosen` is null on a reply, whose links belong to its thread.
 */
export function composedLinks(
  anchor: { entityType: RelinkKind; entityId: string; dealId?: string },
  chosen: Grounding | null,
  // The derived filing when the rep chose nothing. Empty means no project.
  derivedProjectId = "",
): Link[] {
  const links: Link[] = [
    { entity_type: anchor.entityType, entity_id: anchor.entityId },
  ];
  const add = (kind: RelinkKind, id: string) => {
    const already = links.some(
      (l) => l.entity_type === kind && l.entity_id === id,
    );
    if (!id || already) {
      return;
    }
    links.push({ entity_type: kind, entity_id: id });
  };
  add("deal", anchor.dealId ?? "");
  if (!chosen) {
    // An anchored send: the record it started from, plus the filing it states.
    add("project", derivedProjectId);
    return links;
  }
  add("contact", chosen.recipientId);
  add("deal", chosen.dealId);
  add("project", chosen.projectId);
  return links;
}

/**
 * What a new message is about, from whichever page it was opened on. That is
 * the deal and project the rep chose. On the account path it is also the
 * contact the draft is for, which only that path picks. A reply has none of
 * its own, because its thread already files it.
 *
 * A chosen deal travels only from a composer that shows it: the account's
 * filing pick and the contact's deal pick. Elsewhere a deal kept by a draft
 * would file the message under a record nobody on screen named.
 */
export function groundingOf(where: {
  entityType: RelinkKind;
  replying: boolean;
  recipientId: string;
  dealId: string;
  projectId: string;
}): Grounding | null {
  if (where.replying) {
    return null;
  }
  const account = where.entityType === "company";
  const showsDeal = account || where.entityType === "contact";
  return {
    recipientId: account ? where.recipientId : "",
    dealId: showsDeal ? where.dealId : "",
    projectId: where.projectId,
  };
}
