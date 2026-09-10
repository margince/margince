// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { navigate } from "../app/router";
import { useT } from "../i18n";
import { ArchiveAction } from "./archive";
import { throwProblem } from "./common";
import type { ObjectCustomFields } from "./customfields.form";
import { EditAction } from "./edit";
import { MergeAction } from "./merge";
import { mapPersonUpdate, personEditFields } from "./personformfields";
import { invalidateRecord } from "./recordwritekeys";

// Edit, merge and archive — the record's own core write verbs, shared
// between PersonScreen's header (contacts.tsx) and PersonPageV2's
// (personactions.tsx's sibling, wired from personpage.tsx). One writer of
// the PATCH/merge/archive config rather than two, so a change to what a
// person edit form carries cannot fix one page's form and silently leave
// the other stale.

type Person = components["schemas"]["Person"];

// Merge-target search (P-2): reuses the list read, mapped down to the
// {id, name} shape MergeAction renders — the caller filters out the source
// row since this fetch has no notion of "the record being merged away".
async function searchPeopleTargets(
  q: string,
): Promise<{ id: string; name: string }[]> {
  const { data, error } = await api.GET("/people", {
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

export function PersonEditMergeArchive({
  person,
  cf,
  disabledReasonId,
  overlay,
}: Readonly<{
  person: Person;
  // Read at screen level and handed down, so the schema request runs BESIDE the
  // person's rather than after it. Started here, it would begin only once the
  // record had landed and the strip first rendered — and an edit opened in that
  // gap would offer a form with no custom fields on it.
  cf: ObjectCustomFields;
  // The one sentence the caller points every refused verb at. What refuses
  // varies by caller — PersonScreen asks only whether the record is archived,
  // PersonPageV2's useRecordWriteRefusal also answers "not yours to change" —
  // so the reason is the caller's to decide, not this component's.
  disabledReasonId?: string;
  overlay: boolean;
}>) {
  const t = useT();
  const id = person.id;
  // EditAction/MergeAction/ArchiveAction's own invalidate/recordKey props
  // only reach the LIST cache and this record's own `["person", id]` read.
  // A person's fields are ALSO served under `["person360", id]` (the page
  // shell PersonPageV2 reads) and `["personBrief", id]`, three siblings
  // under no common prefix (recordwritekeys.ts) — without this, a save on
  // PersonPageV2 closes the form over a page still showing what the reader
  // just corrected.
  const queryClient = useQueryClient();
  return (
    <>
      <EditAction<Person>
        disabledReasonId={disabledReasonId}
        label={t("record.edit")}
        savedMessage={(saved) =>
          t("record.saveDone", { name: saved.full_name })
        }
        notice={overlay ? t("overlay.partialWriteBack") : undefined}
        // An address is unique among LIVE rows across the workspace
        // (uq_person_email_dedupe), so a correction can now collide with the
        // contact that already holds it — 409 `duplicate_email`, naming that
        // record. Create has always offered the way there; edit could not
        // collide until it carried the address field, and a conflict the
        // reader cannot follow is a dead end on the one screen that fixes
        // addresses.
        resolveExisting={(_code, existingId) => ({
          screen: "contacts",
          id: existingId,
        })}
        fields={[...personEditFields(t), ...cf.formFields]}
        record={{
          id: person.id,
          version: person.version,
          full_name: person.full_name,
          first_name: person.first_name ?? "",
          last_name: person.last_name ?? "",
          title: person.title ?? "",
          "social.linkedin": stringField(person.social?.linkedin),
          // The rows the form prefills from. `is_primary` is stringified by
          // prefillRows like every other cell, and read back as `=== "true"`,
          // so the primary marker survives a save that did not touch it.
          emails: person.emails ?? [],
          phones: person.phones ?? [],
          ...cf.recordSlice(person),
        }}
        update={async (values, rows, opened) => {
          const { data, error } = await api.PATCH("/people/{id}", {
            params: {
              path: { id },
              ...ifMatch(requireVersion(opened?.version)),
            },
            body: {
              ...mapPersonUpdate(values, rows),
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
          await invalidateRecord(queryClient, "person", id);
          return data;
        }}
        invalidate="people"
        recordKey="person"
      />
      {/* Merge has no incumbent-first projection — the seam
          refuses it outright (overlay/provider_writes.go
          Merge) — unlike edit/archive below, which it
          serves, so it stays hidden here. */}
      {!overlay && (
        <MergeAction
          disabledReasonId={disabledReasonId}
          label={t("merge.contact")}
          sourceId={person.id}
          sourceName={person.full_name}
          searchTargets={searchPeopleTargets}
          merge={async (targetId) => {
            const { data, error } = await api.POST("/people/{id}/merge", {
              params: {
                path: { id: person.id },
                ...ifMatch(requireVersion(person.version)),
              },
              body: { target_id: targetId },
            });
            if (error) {
              throwProblem(error, t);
            }
            // Both ends of the merge: the source is gone and the survivor
            // may now carry fields the source contributed — a reader landing
            // on either via survivorRoute must not see pre-merge state.
            await invalidateRecord(queryClient, "person", person.id);
            await invalidateRecord(queryClient, "person", targetId);
            return data;
          }}
          invalidate="people"
          recordKey="person"
          survivorRoute={(targetId) => ({
            screen: "contacts",
            id: targetId,
          })}
        />
      )}
      <ArchiveAction
        disabledReasonId={disabledReasonId}
        label={t("record.archive")}
        confirmText={t("record.archiveConfirm")}
        archivedMessage={t("record.archiveDone", {
          name: person.full_name,
        })}
        archive={async () => {
          const { data, error } = await api.DELETE("/people/{id}", {
            params: { path: { id } },
          });
          if (error) {
            throwProblem(error);
          }
          await invalidateRecord(queryClient, "person", id);
          return data;
        }}
        invalidate="people"
        recordKey="person"
        onArchived={() => navigate({ screen: "contacts" })}
      />
    </>
  );
}
