// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { navigate } from "../app/router";
import { useT } from "../i18n";
import { ArchiveAction } from "./archive";
import { throwProblem } from "./common";
import { MergeAction } from "./merge";
import { invalidateRecord } from "./recordwritekeys";

// The contact header's merge and archive commands.

type Contact = components["schemas"]["Contact"];

// Merge-target search (P-2): reuses the list read, mapped down to the
// {id, name} shape MergeAction renders — the caller filters out the source
// row since this fetch has no notion of "the record being merged away".
async function searchContactsTargets(
  q: string,
): Promise<{ id: string; name: string }[]> {
  const { data, error } = await api.GET("/contacts", {
    params: { query: { q, limit: 10 } },
  });
  if (error) {
    throwProblem(error);
  }
  return data.data.map((candidate) => ({
    id: candidate.id,
    name: candidate.full_name,
  }));
}

export function ContactRecordActions({
  contact,
  disabledReasonId,
  overlay,
  beforeArchive,
}: Readonly<{
  contact: Contact;
  // The page's shared explanation for refused record actions.
  disabledReasonId?: string;
  overlay: boolean;
  // The caller's own menu rows, drawn between Merge and Archive. The position
  // is the whole prop: Archive is destructive and goes last in any menu, so a
  // caller appending its rows after this fragment would seat them past the one
  // item that has to stay at the end.
  beforeArchive?: ReactNode;
}>) {
  const t = useT();
  const id = contact.id;
  // MergeAction/ArchiveAction's own invalidate/recordKey props
  // only reach the LIST cache and this record's own `["contact", id]` read.
  // A contact's fields are ALSO served under `["contact360", id]` (the page
  // shell ContactPageV2 reads) and `["contactBrief", id]`, three siblings
  // under no common prefix (recordwritekeys.ts) — without this, a save on
  // ContactPageV2 closes the form over a page still showing what the reader
  // just corrected.
  const queryClient = useQueryClient();
  return (
    <>
      {/* Merge has no incumbent-first projection — the seam
          refuses it outright (overlay/provider_writes.go
          Merge) — unlike edit/archive below, which it
          serves, so it stays hidden here. */}
      {!overlay && (
        <MergeAction
          disabledReasonId={disabledReasonId}
          label={t("merge.contact")}
          sourceId={contact.id}
          sourceName={contact.full_name}
          searchTargets={searchContactsTargets}
          merge={async (targetId) => {
            const { data, error } = await api.POST("/contacts/{id}/merge", {
              params: {
                path: { id: contact.id },
                ...ifMatch(requireVersion(contact.version)),
              },
              body: { target_id: targetId },
            });
            if (error) {
              throwProblem(error, t);
            }
            // Both ends of the merge: the source is gone and the survivor
            // may now carry fields the source contributed — a reader landing
            // on either via survivorRoute must not see pre-merge state.
            await invalidateRecord(queryClient, "contact", contact.id);
            await invalidateRecord(queryClient, "contact", targetId);
            return data;
          }}
          invalidate="contacts"
          recordKey="contact"
          survivorRoute={(targetId) => ({
            screen: "contacts",
            id: targetId,
          })}
        />
      )}
      {beforeArchive}
      <ArchiveAction
        disabledReasonId={disabledReasonId}
        label={t("record.archive")}
        confirmText={t("record.archiveConfirm")}
        archivedMessage={t("record.archiveDone", {
          name: contact.full_name,
        })}
        archive={async () => {
          const { data, error } = await api.DELETE("/contacts/{id}", {
            params: {
              path: { id },
              ...ifMatch(requireVersion(contact.version)),
            },
          });
          if (error) {
            throwProblem(error);
          }
          await invalidateRecord(queryClient, "contact", id);
          return data;
        }}
        invalidate="contacts"
        recordKey="contact"
        onArchived={() => navigate({ screen: "contacts" })}
      />
    </>
  );
}
