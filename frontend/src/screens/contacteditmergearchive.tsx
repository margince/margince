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
import { contactEditFields, mapContactUpdate } from "./contactformfields";
import type { ObjectCustomFields } from "./customfields.form";
import { EditAction } from "./edit";
import { MergeAction } from "./merge";
import { invalidateRecord } from "./recordwritekeys";

// Edit, merge and archive — the record's own core write verbs, shared
// between ContactScreen's header (contacts.tsx) and ContactPageV2's
// (contactactions.tsx's sibling, wired from contactpage.tsx). One writer of
// the PATCH/merge/archive config rather than two, so a change to what a
// contact edit form carries cannot fix one page's form and silently leave
// the other stale.
//
// Every verb here is WORDED: both callers draw them as rows of an
// OverflowMenu, where a bare pencil would be the one row of the list that
// names nothing.

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

function stringField(value: unknown): string {
  return typeof value === "string" ? value : "";
}

export function ContactEditMergeArchive({
  contact,
  cf,
  disabledReasonId,
  overlay,
  beforeArchive,
}: Readonly<{
  contact: Contact;
  // Read at screen level and handed down, so the schema request runs BESIDE the
  // contact's rather than after it. Started here, it would begin only once the
  // record had landed and the strip first rendered — and an edit opened in that
  // gap would offer a form with no custom fields on it.
  cf: ObjectCustomFields;
  // The one sentence the caller points every refused verb at. What refuses
  // varies by caller — ContactScreen asks only whether the record is archived,
  // ContactPageV2's useRecordWriteRefusal also answers "not yours to change" —
  // so the reason is the caller's to decide, not this component's.
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
  // EditAction/MergeAction/ArchiveAction's own invalidate/recordKey props
  // only reach the LIST cache and this record's own `["contact", id]` read.
  // A contact's fields are ALSO served under `["contact360", id]` (the page
  // shell ContactPageV2 reads) and `["contactBrief", id]`, three siblings
  // under no common prefix (recordwritekeys.ts) — without this, a save on
  // ContactPageV2 closes the form over a page still showing what the reader
  // just corrected.
  const queryClient = useQueryClient();
  return (
    <>
      <EditAction<Contact>
        labelled
        disabledReasonId={disabledReasonId}
        label={t("record.edit")}
        savedMessage={(saved) =>
          t("record.saveDone", { name: saved.full_name })
        }
        notice={overlay ? t("overlay.partialWriteBack") : undefined}
        // An address is unique among LIVE rows across the workspace
        // (uq_contact_email_dedupe), so a correction can now collide with the
        // contact that already holds it — 409 `duplicate_email`, naming that
        // record. Create has always offered the way there; edit could not
        // collide until it carried the address field, and a conflict the
        // reader cannot follow is a dead end on the one screen that fixes
        // addresses.
        resolveExisting={(_code, existingId) => ({
          screen: "contacts",
          id: existingId,
        })}
        fields={[...contactEditFields(t), ...cf.formFields]}
        record={{
          id: contact.id,
          version: contact.version,
          full_name: contact.full_name,
          first_name: contact.first_name ?? "",
          last_name: contact.last_name ?? "",
          title: contact.title ?? "",
          "social.linkedin": stringField(contact.social?.linkedin),
          // The rows the form prefills from. `is_primary` is stringified by
          // prefillRows like every other cell, and read back as `=== "true"`,
          // so the primary marker survives a save that did not touch it.
          emails: contact.emails ?? [],
          phones: contact.phones ?? [],
          ...cf.recordSlice(contact),
        }}
        update={async (values, rows, opened) => {
          const { data, error } = await api.PATCH("/contacts/{id}", {
            params: {
              path: { id },
              ...ifMatch(requireVersion(opened?.version)),
            },
            body: {
              ...mapContactUpdate(values, rows),
              // A diff against what the form prefilled from: a
              // snapshot sends `null` for every empty custom field,
              // and the API reads that as clearing a column nobody
              // touched.
              ...cf.toPatch(values, opened ?? {}),
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
      />
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
            params: { path: { id } },
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
