// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Who was on an exchange, named.
//
// Its own module because more than one surface names who was on an exchange.
// Those are every row of a record's chronology (activitytimeline.tsx), the mail thread,
// the contact's memory. Spelled twice, one copy would come to name three
// contacts where the other named two.
//
// An activity carries ids, not names. Resolving them is the caller's job: the
// names live in the sections around the row and this module reads nothing.

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
 * A party on an exchange: what it is called, and the key its face is drawn
 * on — a contact's record id, or the name itself where no record stands
 * behind it.
 */
export type ContactOn = Readonly<{ key: string; name: string }>;

/**
 * The contacts an activity is filed against, named and in link order.
 *
 * The id rides beside the name because a face is keyed on the record: a chip
 * keyed on the name draws one contact in two colours on a page whose header
 * keys it on the id. An id nobody can put a name to is dropped rather than
 * printed: a reader cannot recognise a uuid, and a row that shows one has spent
 * its line saying nothing.
 */
export function contactsOn(
  links: readonly ActivityLinkRef[] | undefined,
  nameOf?: NameOf,
): ContactOn[] {
  const contacts: ContactOn[] = [];
  for (const link of links ?? []) {
    if (link.entity_type !== "contact") {
      continue;
    }
    const name = nameOf?.("contact", link.entity_id);
    if (name && !contacts.some((seen) => seen.key === link.entity_id)) {
      contacts.push({ key: link.entity_id, name });
    }
  }
  return contacts;
}

/**
 * Their names, each once. Two records can carry one name, and a line that
 * printed it twice would read as a slip rather than as two contacts; a face
 * keeps the two apart by key.
 */
export function namesOf(contacts: readonly ContactOn[]): string[] {
  return [...new Set(contacts.map((contact) => contact.name))];
}

/** A contact record as a page holds it: enough to know it by name. */
export type RecordContact = Readonly<{ id: string; full_name: string }>;

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
