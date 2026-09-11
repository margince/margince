// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Who was on an exchange, named.
//
// Its own module because two surfaces answer the same question and neither
// owns it: the thread across the top of a record (screens/record360/spine.tsx)
// and every row of its chronology (activitytimeline.tsx). Spelled twice, one
// copy would come to name three contacts where the other named two, and a reader
// comparing the thread with the list underneath it would have to decide which
// of the two was lying.
//
// An activity carries ids, not names — resolving them is the CALLER's job,
// because the names live in the sections around the row and this module holds
// no read of its own.

import { formatNumber } from "../format/format";
import type { Locale, useT } from "../i18n";

/** What a link on an activity points at. The shape the contract sends. */
export type ActivityLinkRef = { entity_type: string; entity_id: string };

/** Resolves a record id to what it is called, or nothing when it cannot. */
export type NameOf = (
  entityType: string,
  entityId: string,
) => string | undefined;

/**
 * The contacts an activity is filed against, named and in link order.
 *
 * An id nobody can put a name to is dropped rather than printed: a reader
 * cannot recognise a uuid, and a row that shows one has spent its line saying
 * nothing.
 */
export function contactsOn(
  links: readonly ActivityLinkRef[] | undefined,
  nameOf?: NameOf,
): string[] {
  const names: string[] = [];
  for (const link of links ?? []) {
    if (link.entity_type !== "contact") {
      continue;
    }
    const name = nameOf?.("contact", link.entity_id);
    if (name && !names.includes(name)) {
      names.push(name);
    }
  }
  return names;
}

/**
 * Two lists of contacts as one, first-seen order kept.
 *
 * A conversation is folded newest message first, so the contacts on its latest
 * exchange lead — those are the ones a reader is going back to.
 */
export function mergeContacts(
  held: readonly string[],
  arriving: readonly string[],
): readonly string[] {
  const out = [...held];
  for (const name of arriving) {
    if (!out.includes(name)) {
      out.push(name);
    }
  }
  return out;
}

// How many contacts one line names before it counts the rest. Two is what fits
// beside a subject at this size; a third name pushes the line onto a wrap that
// costs the row below it its own room.
const NAMED_CONTACTS = 2;

/**
 * Who was on it, as one phrase: the names while there are few, the first ones
 * and a count once there are many.
 */
export function withWhom(
  contacts: readonly string[],
  t: ReturnType<typeof useT>,
  locale: Locale,
): string | undefined {
  if (contacts.length === 0) {
    return undefined;
  }
  if (contacts.length <= NAMED_CONTACTS) {
    return contacts.join(", ");
  }
  return t("co.spine.andOthers", {
    names: contacts.slice(0, NAMED_CONTACTS).join(", "),
    count: formatNumber(contacts.length - NAMED_CONTACTS, locale),
  });
}
