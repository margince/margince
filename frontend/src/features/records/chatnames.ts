// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueries } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { entityNameObserverQuery } from "../../components/entityref.queries";
import {
  contactsOn,
  namesOf,
  withWhom,
} from "../../design-system/participants";
import type { TimelineEntry } from "../../design-system/timelineentry";
import type { Locale, useT } from "../../i18n";

type Activity = components["schemas"]["Activity"];

// A chat names no sender. It is filed on the contact it was with, and that
// contact wrote the other side. A mail may come from somebody else than its
// contact. So a mail keeps the server's own reading and is never named here.
function chatContactIds(activities: readonly Activity[]): string[] {
  const ids = new Set<string>();
  for (const activity of activities) {
    if (activity.kind !== "message") {
      continue;
    }
    for (const link of activity.links ?? []) {
      if (link.entity_type === "contact") {
        ids.add(link.entity_id);
      }
    }
  }
  return [...ids];
}

/**
 * useChatNames names the other side of each chat message by the contact it is
 * filed on, for an entry the page could not name otherwise. It answers the
 * entry unchanged for every other kind, and while a name is still loading.
 */
export function useChatNames(
  activities: readonly Activity[],
  t: ReturnType<typeof useT>,
  locale: Locale,
): (entry: TimelineEntry, activity: Activity | undefined) => TimelineEntry {
  const ids = chatContactIds(activities);
  const reads = useQueries({
    queries: ids.map((id) => entityNameObserverQuery("contact", id)),
  });
  const names = new Map<string, string>();
  ids.forEach((id, index) => {
    const name = reads[index]?.data;
    if (name) {
      names.set(id, name);
    }
  });
  return (entry, activity) => {
    if (activity?.kind !== "message" || entry.counterparts || entry.withheld) {
      return entry;
    }
    const contacts = contactsOn(activity.links, (type, id) =>
      type === "contact" ? names.get(id) : undefined,
    );
    if (contacts.length === 0) {
      return entry;
    }
    return {
      ...entry,
      counterparts: withWhom(namesOf(contacts), t, locale),
      counterpartContacts: contacts,
    };
  };
}
