// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type QueryKey, useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { RecordType } from "../../components/share.parts";
import { useCanWrite, type WritableRecord } from "../../lib/capability";
import { contactQuery } from "../../lib/contact.queries";
import { leadKey } from "../../lib/leadkeys";
import { problemStatus, SHOWN_REFUSALS, unwrap } from "../../lib/problem";
import {
  useActionRefusal,
  useKindWriteRefusal,
  useMeSettled,
} from "../../lib/writerefusal";

type RecordRead = Readonly<{
  queryKey: QueryKey;
  queryFn: () => Promise<WritableRecord>;
}>;

// Each record under the key its own page reads it by. Every write to it
// invalidates that key (lib/recordwritekeys.ts), so the screen never holds a
// `writable` the record has since lost.
function recordRead(kind: RecordType, id: string): RecordRead {
  const params = { params: { path: { id } } };
  switch (kind) {
    case "contact":
      return {
        queryKey: contactQuery(id).queryKey,
        queryFn: async () => unwrap(await api.GET("/contacts/{id}", params)),
      };
    case "company":
      return {
        queryKey: ["company", id],
        queryFn: async () => unwrap(await api.GET("/companies/{id}", params)),
      };
    case "deal":
      return {
        queryKey: ["deal", id],
        queryFn: async () => unwrap(await api.GET("/deals/{id}", params)),
      };
    case "lead":
      return {
        queryKey: leadKey(id),
        queryFn: async () => unwrap(await api.GET("/leads/{id}", params)),
      };
    case "project":
      return {
        queryKey: ["project", id],
        queryFn: async () => unwrap(await api.GET("/projects/{id}", params)),
      };
    default:
      // The route admits only the kinds above (share.tsx), so this is a kind a
      // newer contract added and this build cannot read.
      throw new Error(`no record read for kind ${kind}`);
  }
}

export type ShareStanding =
  | Readonly<{ state: "pending" }>
  | Readonly<{ state: "missing" }>
  | Readonly<{ state: "refused"; refusal: string }>
  | Readonly<{
      state: "shown";
      refusal: string | undefined;
      mayRetract: boolean;
    }>;

/**
 * Whether the reader may see the record a share screen is about, and why they
 * may not share it. Sharing asks the record's write authority (`EnsureCanGrant`),
 * which the server answers as `writable`. A 404 is a record the reader cannot
 * see; a 403, a kind the role cannot read.
 *
 * Revoking asks that authority on a row that need not be live (`mayRevoke`).
 * `writable` is false on an archived row, so there the grant and seat decide.
 *
 * `contactsOnly` shares a company's contacts, which asks the contact grant
 * rather than the company's write authority: each contact is judged on its own.
 */
export function useShareStanding(
  kind: RecordType,
  id: string,
  contactsOnly: boolean,
): ShareStanding {
  const read = recordRead(kind, id);
  const record = useQuery({
    queryKey: read.queryKey,
    queryFn: read.queryFn,
    meta: { [SHOWN_REFUSALS]: [403, 404] },
  });
  const recordRefusal = useKindWriteRefusal(kind, record.data);
  const grantRefusal = useActionRefusal({
    object: contactsOnly ? "contact" : kind,
    action: "update",
  });
  const mayUpdate = useCanWrite(kind, "update");
  // Settled, not answered: a /me that failed or carried no snapshot leaves
  // every grant denied, which the screen shows rather than waiting forever.
  const meSettled = useMeSettled();
  const status = record.isError ? problemStatus(record.error) : 0;
  if (status === 404) {
    return { state: "missing" };
  }
  if (status === 403) {
    return { state: "refused", refusal: grantRefusal ?? recordRefusal ?? "" };
  }
  if ((record.isPending && !record.isError) || !meSettled) {
    return { state: "pending" };
  }
  // Another failure says nothing about the row, so only the grant decides.
  if (record.isError || contactsOnly) {
    return {
      state: "shown",
      refusal: grantRefusal,
      mayRetract: grantRefusal === undefined,
    };
  }
  const archived = Boolean(record.data?.archived_at);
  return {
    state: "shown",
    refusal: recordRefusal,
    mayRetract: recordRefusal === undefined || (archived && mayUpdate),
  };
}
