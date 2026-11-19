// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { RecordType } from "../../components/share.parts";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  useCanWrite,
  useRecordWriteRefusal,
  type WritableRecord,
} from "../../lib/capability";
import { valueFor } from "../../lib/labelfor";
import { problemStatus, SHOWN_REFUSALS, unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";

// The sentences each record's own page gives for a write it refuses, so the
// share screen says what the record's Share verb already said.
const REFUSALS = {
  contact: {
    archived: "contact.rail.archivedReadOnly",
    notYours: "contact.notYoursToChange",
  },
  company: {
    archived: "record.archivedReadOnly",
    notYours: "record.notYoursToChange",
  },
  deal: {
    archived: "deal.archivedReadOnly",
    notYours: "deal.notYoursToChange",
  },
  lead: {
    archived: "lead.terminalReadOnly",
    notYours: "lead.notYoursToChange",
  },
  project: {
    archived: "project.archivedReadOnly",
    notYours: "project.notYoursToChange",
  },
} as const satisfies Record<
  RecordType,
  Record<"archived" | "notYours", MessageKey>
>;

async function readRecord(
  kind: RecordType,
  id: string,
): Promise<WritableRecord> {
  const params = { params: { path: { id } } };
  switch (kind) {
    case "contact":
      return unwrap(await api.GET("/contacts/{id}", params));
    case "company":
      return unwrap(await api.GET("/companies/{id}", params));
    case "deal":
      return unwrap(await api.GET("/deals/{id}", params));
    case "lead":
      return unwrap(await api.GET("/leads/{id}", params));
    case "project":
      return unwrap(await api.GET("/projects/{id}", params));
    default:
      // The route admits only the kinds above (share.tsx), so this is a kind a
      // newer contract added and this build cannot read.
      throw new Error(`no record read for kind ${kind}`);
  }
}

// The generic sentences answer a kind this build does not word on its own.
const GENERIC_REFUSAL = REFUSALS.company;

export type ShareStanding =
  | Readonly<{ state: "pending" }>
  | Readonly<{ state: "missing" }>
  | Readonly<{ state: "shown"; refusal: string | undefined }>;

/**
 * Whether the reader may see the record a share screen is about, and why they
 * may not share it. Sharing asks the record's write authority (identity/
 * grants.go `EnsureCanGrant`), so the screen reads the server's `writable`.
 * A 404 is a record the reader cannot see, which is not offered for sharing.
 *
 * `contactsOnly` shares a company's contacts, which asks the contact grant
 * rather than the company's write authority: each contact is judged on its own.
 */
export function useShareStanding(
  kind: RecordType,
  id: string,
  contactsOnly: boolean,
): ShareStanding {
  const t = useT();
  const record = useQuery({
    queryKey: ["share-record", kind, id],
    queryFn: () => readRecord(kind, id),
    meta: { [SHOWN_REFUSALS]: [404] },
  });
  const words = valueFor(REFUSALS, kind, GENERIC_REFUSAL);
  const refusal = useRecordWriteRefusal(kind, record.data, {
    archived: t(words.archived),
    notYours: t(words.notYours),
  });
  const mayShareContacts = useCanWrite("contact", "update");
  const answered = useMe().data?.authorization !== undefined;
  if (record.isError && problemStatus(record.error) === 404) {
    return { state: "missing" };
  }
  if (record.isPending || !answered) {
    return { state: "pending" };
  }
  // Another failure says nothing about the reader. The screen claims no
  // refusal and leaves the write to the server.
  if (record.isError) {
    return { state: "shown", refusal: undefined };
  }
  if (contactsOnly) {
    return {
      state: "shown",
      refusal: mayShareContacts ? undefined : t("record.actionRefused"),
    };
  }
  return { state: "shown", refusal };
}
