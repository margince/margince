// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import type { RecordType } from "../../components/share.parts";
import { useCanWrite } from "../../lib/capability";
import { problemStatus, SHOWN_REFUSALS } from "../../lib/problem";
import { recordRead } from "../../lib/recordread";
import {
  useActionRefusal,
  useKindWriteRefusal,
  useMeSettled,
} from "../../lib/writerefusal";

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
  // Settled, not answered: a /me that failed or carried no snapshot refuses
  // every grant (useActionRefusal), which the screen shows rather than waits.
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
