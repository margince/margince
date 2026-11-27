// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch, requireVersion } from "../../api/version";
import { ArchiveAction } from "../../components/archive";
import { MergeAction } from "../../components/merge";
import type { RecordPickerCandidate } from "../../design-system/recordpicker";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { fileListKeys } from "../../lib/attachmentkeys";
import { unwrap } from "../../lib/problem";
import { restoreLatestArchive } from "../../lib/recordrestore";
import {
  invalidateRecord,
  invalidateRetiredRecord,
} from "../../lib/recordwritekeys";
import { navigate } from "../../lib/router";
import { useArchiveRefusal } from "../../lib/writerefusal";

// The contact header's merge and archive commands.

type Contact = components["schemas"]["Contact"];

// The detail line that tells two contacts of one name apart, for the picked
// target and the contact merged away alike. A field the reader may not see
// arrives empty, and it is left out with its separator.
export function contactMergeDetail(contact: Contact): string | undefined {
  return (
    [contact.employer?.company_name, contact.title, contact.primary_email]
      .filter(Boolean)
      .join(" · ") || undefined
  );
}

// Merge-target search (P-2) over the list read.
export async function searchContactsTargets(
  q: string,
): Promise<RecordPickerCandidate[]> {
  const data = unwrap(
    await api.GET("/contacts", {
      params: { query: { q, limit: 10 } },
    }),
  );
  return data.data.map((candidate) => ({
    id: candidate.id,
    name: candidate.full_name,
    writable: candidate.writable,
    detail: contactMergeDetail(candidate),
  }));
}

export function ContactRecordActions({
  contact,
  disabledReasonId,
  beforeArchive,
}: Readonly<{
  contact: Contact;
  // The page's shared explanation for refused record actions.
  disabledReasonId?: string;
  // The caller's own menu rows, drawn between Merge and Archive. The position
  // is the whole prop: Archive is destructive and goes last in any menu, so a
  // caller appending its rows after this fragment would seat them past the one
  // item that has to stay at the end.
  beforeArchive?: ReactNode;
}>) {
  const t = useT();
  const archiveRefusal = useArchiveRefusal("contact");
  const id = contact.id;
  // MergeAction/ArchiveAction's own invalidate/recordKey props
  // only reach the LIST cache and this record's own `["contact", id]` read.
  // A contact's fields are ALSO served under `["contact360", id]` (the page
  // shell ContactPageV2 reads) and `["contactBrief", id]`, three siblings
  // under no common prefix (recordwritekeys.ts) — without this, a save on
  // ContactPageV2 closes the form over a page still showing what the reader
  // just corrected.
  const queryClient = useQueryClient();
  const toast = useToast();
  return (
    <>
      <MergeAction
        requires={{ object: "contact", action: "update" }}
        disabledReasonId={disabledReasonId}
        label={t("merge.contact")}
        sourceId={contact.id}
        sourceName={contact.full_name}
        sourceDetail={contactMergeDetail(contact)}
        searchTargets={searchContactsTargets}
        merge={async (targetId) => {
          const data = unwrap(
            await api.POST("/contacts/{id}/merge", {
              params: {
                path: { id: contact.id },
                ...ifMatch(requireVersion(contact.version)),
              },
              body: { target_id: targetId },
            }),
            t,
          );
          // Both ends of the merge: the source is gone and the survivor
          // may now carry fields the source contributed — a reader landing
          // on either via survivorRoute must not see pre-merge state.
          await invalidateRetiredRecord(queryClient, "contact", contact.id);
          await invalidateRecord(queryClient, "contact", targetId);
          // The source's files and message links move to the survivor.
          await Promise.all(
            [contact.id, targetId]
              .flatMap((id) =>
                fileListKeys({ entityType: "contact", entityId: id }, "moved"),
              )
              .map((queryKey) => queryClient.invalidateQueries({ queryKey })),
          );
          return data;
        }}
        invalidate="contacts"
        recordKey="contact"
        survivorRoute={(targetId) => ({
          screen: "contacts",
          id: targetId,
        })}
      />
      {beforeArchive}
      <ArchiveAction
        requires={{ object: "contact", action: "delete" }}
        archivable={contact.archivable ?? false}
        refusedReason={archiveRefusal}
        disabledReasonId={disabledReasonId}
        label={t("record.archive")}
        confirmText={t("record.archiveConfirm")}
        archivedMessage={t("record.archiveDone", {
          name: contact.full_name,
        })}
        archive={async () => {
          const data = unwrap(
            await api.DELETE("/contacts/{id}", {
              params: {
                path: { id },
                ...ifMatch(requireVersion(contact.version)),
              },
            }),
          );
          await invalidateRetiredRecord(queryClient, "contact", id);
          return data;
        }}
        invalidate="contacts"
        recordKey="contact"
        onArchived={(from) => from.closeOnto({ screen: "contacts" })}
        undo={async (archived) => {
          const restored = await restoreLatestArchive(
            "contact",
            archived.id,
            requireVersion(archived.version),
          );
          await invalidateRecord(queryClient, "contact", archived.id);
          await queryClient.invalidateQueries({ queryKey: ["contacts"] });
          if (restored) {
            navigate({ screen: "contacts", id: archived.id });
          } else {
            toast.show(t("contact.undoArchiveNothing"), { tone: "danger" });
          }
        }}
      />
    </>
  );
}
